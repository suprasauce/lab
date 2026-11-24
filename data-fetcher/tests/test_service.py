"""Offline HTTP/SQL tests: no credentials, live database, or Breeze calls."""

from contextlib import contextmanager
from copy import deepcopy
from datetime import datetime, timedelta
from decimal import Decimal
import unittest
from unittest.mock import Mock, patch

import database
import ingestion
from breeze_client import MARKET_ZONE
from main import app


def candle(timestamp):
    return {"datetime": timestamp.isoformat(), "open": "22500.05", "high": "22510.10",
            "low": "22490.00", "close": "22505.25", "volume": "0"}


class Store:
    """Small transactional SQL double for the actual database.py functions."""

    def __init__(self):
        self.contracts = {}
        self.chain_fetched = False
        self.fail_flag = False
        self.options = {}
        self.underlyings = {}
        self.connections = 0
        self.in_transaction = False
        self.commits = self.rollbacks = self.writes = 0
        self.fail_batch = 0
        self.fail_commit = False
        self.has_underlying = self.has_expiry = True

    @contextmanager
    def connect(self):
        self.connections += 1
        try:
            yield Connection(self)
        finally:
            self.connections -= 1


class Connection:
    def __init__(self, store):
        self.store = store

    @contextmanager
    def transaction(self):
        s = self.store
        snapshot = deepcopy((s.contracts, s.options, s.underlyings, s.chain_fetched))
        s.in_transaction = True
        try:
            yield
            if s.fail_commit:
                raise OSError("commit failed")
        except BaseException:
            s.contracts, s.options, s.underlyings, s.chain_fetched = snapshot
            s.rollbacks += 1
            raise
        else:
            s.commits += 1
        finally:
            s.in_transaction = False

    def cursor(self):
        return Cursor(self.store)


class Cursor:
    def __init__(self, store):
        self.store = store
        self.row = None
        self.rowcount = 0

    def __enter__(self):
        return self

    def __exit__(self, *args):
        return False

    def execute(self, query, args=()):
        s = self.store
        if "FROM information_schema.columns" in query:
            self.row = (False,)
        elif "SELECT option_chain_fetched" in query:
            self.row = (s.chain_fetched,) if s.has_expiry else None
        elif "UPDATE lab.option_expiry SET option_chain_fetched" in query:
            assert s.in_transaction
            if s.fail_flag:
                raise OSError("flag update failed")
            s.chain_fetched = True
            self.rowcount = 1
        elif "SELECT start_time, timeframe, open, high, low, close, volume" in query:
            underlying_id, start, end = args
            self.rows = [row[1:] for row in s.underlyings.values()
                         if row[0] == underlying_id and start <= row[1] < end]
        elif "SELECT id FROM lab.underlyings" in query:
            self.row = (7,) if s.has_underlying else None
        elif "SELECT id FROM lab.option_expiry" in query:
            self.row = (9,) if s.has_expiry else None
        elif "SELECT id FROM lab.option_contracts" in query:
            contract = s.contracts.get(tuple(args))
            self.row = (contract,) if contract is not None else None
        elif "INSERT INTO lab.option_contracts" in query:
            assert s.in_transaction, "contract insert outside transaction"
            assert "ON CONFLICT (expiry_id, strike_price, option_type) DO NOTHING" in query
            if tuple(args) in s.contracts:
                self.row = None
            else:
                contract_id = max(s.contracts.values(), default=41) + 1
                s.contracts[tuple(args)] = contract_id
                self.row = (contract_id,)
        else:
            raise AssertionError(f"Unexpected query: {query}")

    def fetchall(self):
        return self.rows

    def fetchone(self):
        return self.row

    def executemany(self, query, rows):
        s = self.store
        assert s.in_transaction, "candle insert outside transaction"
        assert "ON CONFLICT" in query
        s.writes += 1
        if s.fail_batch and s.writes == s.fail_batch:
            raise OSError("candle insert failed")
        target = s.options if "lab.option_candles" in query else s.underlyings
        self.rowcount = 0
        for row in rows:
            if target is s.options:
                assert row[0] in s.contracts.values(), "missing contract ID"
            key = row[:3]
            if key not in target:
                target[key] = row
                self.rowcount += 1


class ServiceTests(unittest.TestCase):
    def setUp(self):
        self.store = Store()
        self.db_patch = patch("ingestion.database.connect", side_effect=self.store.connect)
        self.db = self.db_patch.start()
        self.addCleanup(self.db_patch.stop)
        self.breeze = Mock()
        self.client_patch = patch("ingestion.get_client", return_value=self.breeze)
        self.get_client = self.client_patch.start()
        self.addCleanup(self.client_patch.stop)
        self.calls = []
        self.breeze.historical_underlying.side_effect = self.underlying_window
        self.http = app.test_client()

    def check_fetch(self, start, end):
        self.assertEqual(self.store.connections, 0, "connection held while fetching")
        self.assertFalse(self.store.in_transaction, "transaction open while fetching")
        self.assertLess(end - start, timedelta(days=16))
        self.calls.append((start, end))
        return [candle(start + timedelta(minutes=15))]


    def underlying_window(self, symbol, start, end):
        self.assertEqual(symbol, "NIFTY")
        # Underlying reruns may follow a previously committed invocation.
        commits = self.store.commits
        self.store.commits = 0
        try:
            self.check_fetch(start, end)
            # A fixed candle series must not change when request boundaries move.
            rows = []
            day = start.replace(hour=0, minute=0, second=0, microsecond=0)
            while day <= end:
                for hour in (0, 12):
                    timestamp = day + timedelta(hours=hour, minutes=15)
                    if start <= timestamp <= end:
                        rows.append(candle(timestamp))
                day += timedelta(days=1)
            return rows
        finally:
            self.store.commits = commits


    def test_application_endpoints(self):
        self.assertEqual({rule.rule for rule in app.url_map.iter_rules()}, {"/underlyings/fetch", "/option-chain/fetch"})
        self.assertEqual(self.http.post("/options/fetch", json={}).status_code, 404)
        self.assertEqual(self.http.get("/underlyings/fetch").status_code, 405)


    def test_overlapping_underlying_ranges_are_idempotent(self):
        first = self.http.post("/underlyings/fetch", json={"underlying": "NIFTY", "start_date": "2025-01-01", "end_date": "2025-01-02"})
        second = self.http.post("/underlyings/fetch", json={"underlying": "NIFTY", "start_date": "2025-01-02", "end_date": "2025-01-03"})
        self.assertEqual(first.status_code, 200, first.json)
        self.assertEqual(second.status_code, 200, second.json)
        self.assertEqual(first.json["inserted"], 4)
        self.assertEqual(second.json["inserted"], 2)
        self.assertEqual(len(self.store.underlyings), 6)
        self.assertTrue(all(row[2] == "5m" for row in self.store.underlyings.values()))


    def test_request_validation(self):
        body = {"underlying": "NIFTY", "start_date": "2025-01-01", "end_date": "2025-01-02"}
        for change in ({"timeframe": "1minute"}, {"dte": 30}, {"underlying": "UNKNOWN"}, {"start_date": "bad"}, {"start_date": "2025-02-01"}):
            with self.subTest(change=change):
                self.assertEqual(self.http.post("/underlyings/fetch", json={**body, **change}).status_code, 400)
        self.assertEqual(self.http.post("/underlyings/fetch", data="{bad", content_type="application/json").status_code, 400)
        self.get_client.assert_not_called()

    def test_missing_underlying_does_not_fetch(self):
        self.store.has_underlying = False
        response = self.http.post("/underlyings/fetch", json={"underlying": "NIFTY", "start_date": "2025-01-01", "end_date": "2025-01-02"})
        self.assertEqual(response.status_code, 400)
        self.get_client.assert_not_called()


class ConversionTests(unittest.TestCase):
    def test_negative_breeze_volume_is_preserved(self):
        row = candle(datetime(2025, 6, 20, 14, 30, tzinfo=MARKET_ZONE))
        converted = ingestion.candle_row({**row, "volume": -679125})
        self.assertEqual(converted[-1], -679125)
        self.assertEqual(converted[2], 2250005)

    def test_decimal_paise_and_volume(self):
        self.assertEqual(ingestion.to_paise(Decimal("0.29")), 29)
        self.assertEqual(ingestion.to_paise("25000.05"), 2500005)
        for value in ("1.001", "NaN", "Infinity", "-1", str(2**63)):
            with self.subTest(value=value), self.assertRaises(ValueError):
                ingestion.to_paise(value)
        row = candle(datetime(2025, 1, 1, 9, 15, tzinfo=MARKET_ZONE))
        utc = {**row, "datetime": "2025-01-01T03:45:00+00:00"}
        naive = {**row, "datetime": "2025-01-01 09:15:00"}
        self.assertEqual(ingestion.candle_row(row), ingestion.candle_row(utc))
        self.assertEqual(ingestion.candle_row(row), ingestion.candle_row(naive))
        with self.assertRaises(ValueError):
            ingestion.candle_row({**row, "volume": "0.5"})

    def test_database_reads_use_autocommit(self):
        with patch.dict("os.environ", {"DATABASE_URL": "test-dsn"}), patch("database.psycopg.connect") as connect:
            database.connect()
        connect.assert_called_once_with("test-dsn", connect_timeout=10, autocommit=True)


if __name__ == "__main__":
    unittest.main()
