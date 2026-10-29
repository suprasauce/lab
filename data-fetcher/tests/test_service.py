"""Offline HTTP/SQL tests: no credentials, live database, or Breeze calls."""

from contextlib import contextmanager
from copy import deepcopy
from datetime import date, datetime, timedelta
from decimal import Decimal
import unittest
from unittest.mock import Mock, patch

import database
import ingestion
from breeze_client import MARKET_ZONE
from main import app


OPTION = {"underlying": "NIFTY", "expiry": "2025-12-30", "strike": 25000, "option_type": "CALL"}


def candle(timestamp):
    return {"datetime": timestamp.isoformat(), "open": "22500.05", "high": "22510.10",
            "low": "22490.00", "close": "22505.25", "volume": "0"}


class Store:
    """Small transactional SQL double for the actual database.py functions."""

    def __init__(self):
        self.contracts = {}
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
        snapshot = deepcopy((s.contracts, s.options, s.underlyings))
        s.in_transaction = True
        try:
            yield
            if s.fail_commit:
                raise OSError("commit failed")
        except BaseException:
            s.contracts, s.options, s.underlyings = snapshot
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

    def execute(self, query, args):
        s = self.store
        if "SELECT id FROM lab.underlyings" in query:
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
                s.contracts[tuple(args)] = 42
                self.row = (42,)
        else:
            raise AssertionError(f"Unexpected query: {query}")

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
        self.expected_strike = 2500000
        self.breeze.historical_option.side_effect = self.option_window
        self.breeze.historical_underlying.side_effect = self.underlying_window
        self.http = app.test_client()

    def check_fetch(self, start, end):
        self.assertEqual(self.store.connections, 0, "connection held while fetching")
        self.assertFalse(self.store.in_transaction, "transaction open while fetching")
        self.assertEqual(self.store.commits, 0)
        self.assertLess(end - start, timedelta(days=13))
        self.calls.append((start, end))
        return [candle(start + timedelta(minutes=15))]

    def option_window(self, symbol, expiry, strike, right, start, end):
        self.assertEqual((symbol, expiry, strike, right), ("NIFTY", date(2025, 12, 30), self.expected_strike, "CALL"))
        return self.check_fetch(start, end)

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

    def post_option(self):
        return self.http.post("/options/fetch", json=OPTION)

    def test_exactly_two_endpoints(self):
        self.assertEqual({rule.rule for rule in app.url_map.iter_rules()}, {"/options/fetch", "/underlyings/fetch"})
        self.assertEqual(self.http.get("/options/fetch").status_code, 405)

    def test_existing_contract_skips_breeze(self):
        self.store.contracts[(7, 9, 2500000, "CALL")] = 42
        response = self.post_option()
        self.assertEqual(response.status_code, 200)
        self.assertEqual(response.json, {"status": "already_exists", "contract_id": 42, "fetched": 0, "inserted": 0})
        self.get_client.assert_not_called()
        self.assertEqual(self.store.commits, 0)

    def test_new_contract_fetches_all_before_atomic_insert(self):
        response = self.post_option()
        self.assertEqual(response.status_code, 200, response.json)
        self.assertEqual(response.json, {"status": "ingested", "contract_id": 42, "fetched": 4, "inserted": 4})
        self.assertEqual(self.store.commits, 1)
        self.assertEqual(len(self.calls), 4)
        self.assertEqual(self.calls[0][0], datetime(2025, 11, 15, tzinfo=MARKET_ZONE))
        self.assertEqual(self.calls[-1][1], datetime(2025, 12, 30, 23, 59, 59, tzinfo=MARKET_ZONE))
        for previous, following in zip(self.calls, self.calls[1:]):
            self.assertEqual(previous[1] + timedelta(seconds=1), following[0])
        row = next(iter(self.store.options.values()))
        self.assertEqual(row[0], 42)
        self.assertEqual(row[2:], ("5m", 2250005, 2251010, 2249000, 2250525, 0))
        self.assertEqual(row[1].tzinfo, MARKET_ZONE)

    def test_failed_later_fetch_never_inserts_contract(self):
        self.breeze.historical_option.side_effect = [
            [candle(datetime(2025, 11, 15, 0, 15, tzinfo=MARKET_ZONE))], RuntimeError("Breeze unavailable"),
        ]
        self.assertEqual(self.post_option().status_code, 502)
        self.assertEqual((self.store.contracts, self.store.options), ({}, {}))
        self.assertEqual((self.store.commits, self.store.rollbacks), (0, 0))

    def test_json_decimal_strike_stays_exact(self):
        self.expected_strike = 2500005
        response = self.http.post(
            "/options/fetch", content_type="application/json",
            data='{"underlying":"NIFTY","expiry":"2025-12-30","strike":25000.05,"option_type":"CALL"}',
        )
        self.assertEqual(response.status_code, 200, response.json)
        self.assertIn((7, 9, 2500005, "CALL"), self.store.contracts)

    def test_later_candle_failure_rolls_back_contract_and_prior_batches(self):
        self.store.fail_batch = 2
        with patch("ingestion.BATCH_SIZE", 1):
            self.assertEqual(self.post_option().status_code, 500)
        self.assertEqual((self.store.contracts, self.store.options), ({}, {}))
        self.assertEqual((self.store.commits, self.store.rollbacks), (0, 1))
        self.store.fail_batch = 0
        self.assertEqual(self.post_option().status_code, 200)
        self.assertEqual(len(self.store.contracts), 1)

    def test_commit_failure_never_returns_success(self):
        self.store.fail_commit = True
        self.assertEqual(self.post_option().status_code, 500)
        self.assertEqual((self.store.contracts, self.store.options), ({}, {}))
        self.assertEqual(self.store.commits, 0)

    def test_repeated_requests_do_not_fetch_or_duplicate(self):
        self.assertEqual(self.post_option().status_code, 200)
        calls = self.breeze.historical_option.call_count
        response = self.post_option()
        self.assertEqual(response.json["status"], "already_exists")
        self.assertEqual(self.breeze.historical_option.call_count, calls)
        self.assertEqual((len(self.store.contracts), len(self.store.options)), (1, 4))

    def test_concurrent_winner_is_reused(self):
        def window(*args):
            result = self.option_window(*args)
            self.store.contracts[(7, 9, 2500000, "CALL")] = 88
            return result
        self.breeze.historical_option.side_effect = window
        response = self.post_option()
        self.assertEqual(response.status_code, 200, response.json)
        self.assertEqual(response.json["status"], "already_exists")
        self.assertEqual(response.json["contract_id"], 88)
        self.assertEqual(self.store.writes, 0)

    def test_overlapping_underlying_ranges_are_idempotent(self):
        first = self.http.post("/underlyings/fetch", json={"underlying": "NIFTY", "start_date": "2025-01-01", "end_date": "2025-01-02"})
        second = self.http.post("/underlyings/fetch", json={"underlying": "NIFTY", "start_date": "2025-01-02", "end_date": "2025-01-03"})
        self.assertEqual(first.status_code, 200, first.json)
        self.assertEqual(second.status_code, 200, second.json)
        self.assertEqual(first.json["inserted"], 4)
        self.assertEqual(second.json["inserted"], 2)
        self.assertEqual(len(self.store.underlyings), 6)
        self.assertTrue(all(row[2] == "5m" for row in self.store.underlyings.values()))

    def test_missing_catalog_records_do_not_fetch(self):
        for attribute in ("has_underlying", "has_expiry"):
            with self.subTest(attribute=attribute):
                setattr(self.store, attribute, False)
                self.assertEqual(self.post_option().status_code, 400)
                setattr(self.store, attribute, True)
        self.get_client.assert_not_called()

    def test_empty_options_do_not_create_a_completion_marker(self):
        self.breeze.historical_option.return_value = []
        self.breeze.historical_option.side_effect = None
        self.assertEqual(self.post_option().status_code, 502)
        self.assertEqual(self.store.contracts, {})

    def test_invalid_breeze_candle_fails_before_writing(self):
        self.breeze.historical_option.side_effect = None
        self.breeze.historical_option.return_value = [{"datetime": "bad"}]
        response = self.post_option()
        self.assertEqual(response.status_code, 502)
        self.assertEqual(self.store.contracts, {})
        self.assertEqual(self.store.commits, 0)

    def test_unexpired_contract_is_rejected_without_fetching(self):
        future = (datetime.now(MARKET_ZONE).date() + timedelta(days=1)).isoformat()
        response = self.http.post("/options/fetch", json={**OPTION, "expiry": future})
        self.assertEqual(response.status_code, 400)
        self.get_client.assert_not_called()

    def test_request_validation(self):
        for change in ({"timeframe": "1minute"}, {"dte": 30}, {"strike": -1}, {"strike": True}, {"option_type": "FUTURE"}):
            with self.subTest(change=change):
                self.assertEqual(self.http.post("/options/fetch", json={**OPTION, **change}).status_code, 400)
        self.assertEqual(self.http.post("/options/fetch", data="{bad", content_type="application/json").status_code, 400)
        self.assertEqual(self.http.post("/underlyings/fetch", json={"underlying": "NIFTY", "start_date": "2025-02-01", "end_date": "2025-01-01"}).status_code, 400)
        self.get_client.assert_not_called()


class ConversionTests(unittest.TestCase):
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
