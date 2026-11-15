"""Offline chain ingestion tests using the existing transactional SQL double."""
from copy import deepcopy
from datetime import date, datetime, timedelta
import unittest
from unittest.mock import Mock, patch

import database
from breeze_client import BreezeClient, MARKET_ZONE
from main import app
from test_service import Store, candle

REQUEST = {"underlying": "NIFTY", "expiry": "2025-12-30"}

class OptionChainTests(unittest.TestCase):
    def setUp(self):
        self.store = Store()
        p = patch("database.connect", side_effect=lambda: self.store.connect())
        p.start()
        self.addCleanup(p.stop)
        self.client = Mock()
        self.client.historical_underlying.side_effect = self.underlying
        self.client.historical_option.side_effect = self.option
        p = patch("option_chain.get_client", return_value=self.client)
        self.get_client = p.start()
        self.addCleanup(p.stop)
        self.http = app.test_client()

    def fetch_phase(self):
        self.assertEqual(self.store.connections, 0)
        self.assertFalse(self.store.in_transaction)
        self.assertEqual(self.store.commits, 0)
        self.assertFalse(self.store.chain_fetched)

    def underlying(self, symbol, start, end):
        self.fetch_phase()
        self.assertEqual(symbol, "NIFTY")
        return [candle(start + timedelta(minutes=15))]

    def option(self, symbol, expiry, strike, kind, start, end):
        self.fetch_phase()
        self.assertEqual((symbol, expiry), ("NIFTY", date(2025, 12, 30)))
        self.assertIn(kind, ("CALL", "PUT"))
        self.assertLess(end - start, timedelta(days=16))
        return [candle(start + timedelta(minutes=15))]

    def post(self):
        return self.http.post("/option-chain/fetch", json=REQUEST)

    def test_already_fetched(self):
        self.store.chain_fetched = True
        response = self.post()
        self.assertEqual(response.status_code, 200)
        self.assertEqual(response.json["status"], "already_exists")
        self.get_client.assert_not_called()
        self.assertEqual(self.store.commits, 0)

    def test_complete_chain_rounding_both_types_and_atomic_commit(self):
        response = self.post()
        self.assertEqual(response.status_code, 200, response.json)
        # Low 22490 rounds to 22450; high 22510.10 rounds to 22550; pad 2000.
        strikes = list(range(2045000, 2455001, 5000))
        self.assertEqual(set(self.store.contracts), {(7, 9, strike, kind) for strike in strikes for kind in ("CALL", "PUT")})
        self.assertEqual(response.json["fetched"], len(strikes) * 2 * 3)
        self.assertEqual(response.json["inserted"], len(self.store.options))
        self.assertEqual(response.json["underlying_inserted"], 3)
        self.assertTrue(self.store.chain_fetched)
        self.assertEqual(self.store.commits, 1)
        calls = self.client.historical_option.call_count
        self.assertEqual(self.post().json["status"], "already_exists")
        self.assertEqual(self.client.historical_option.call_count, calls)

    def test_45_dte_and_16_day_windows(self):
        self.assertEqual(self.post().status_code, 200)
        calls = self.client.historical_option.call_args_list[:3]
        start = datetime(2025, 11, 15, tzinfo=MARKET_ZONE)
        self.assertEqual(calls[-1].args[-1], datetime(2025, 12, 30, 23, 59, 59, tzinfo=MARKET_ZONE))
        for i, call in enumerate(calls):
            self.assertEqual(call.args[-2], start + timedelta(days=i * 16))
        for first, second in zip(calls, calls[1:]):
            self.assertEqual(first.args[-1] + timedelta(seconds=1), second.args[-2])
        self.assertEqual(self.client.historical_underlying.call_count, 3)

    def test_empty_options_are_successful(self):
        self.client.historical_option.side_effect = None
        self.client.historical_option.return_value = []
        response = self.post()
        self.assertEqual(response.status_code, 200, response.json)
        self.assertEqual(response.json["fetched"], 0)
        self.assertTrue(self.store.chain_fetched)
        self.assertEqual(len(self.store.contracts), 166)

    def test_stored_underlying_prices_are_reused(self):
        row = (7, datetime(2025, 11, 16, 9, 15, tzinfo=MARKET_ZONE), "5m", 2200000, 2250000, 2200000, 2200000, 0)
        self.store.underlyings[row[:3]] = row
        self.assertEqual(self.post().status_code, 200)
        self.assertEqual(min(key[2] for key in self.store.contracts), 2000000)
        self.assertEqual(self.store.underlyings[row[:3]], row)

    def test_no_underlying_aborts(self):
        self.client.historical_underlying.side_effect = None
        self.client.historical_underlying.return_value = []
        self.assertEqual(self.post().status_code, 502)
        self.client.historical_option.assert_not_called()
        self.assertEqual(self.store.commits, 0)

    def test_failed_later_contract_commits_nothing(self):
        count = 0
        def request(*args):
            nonlocal count
            count += 1
            if count == 4:
                raise RuntimeError("terminal Breeze failure")
            return self.option(*args)
        self.client.historical_option.side_effect = request
        response = self.post()
        self.assertEqual(response.status_code, 502)
        for text in ("strike_paise=2045000 option_type=PUT", "2025-11-15", "terminal Breeze failure"):
            self.assertIn(text, response.json["error"])
        self.assertEqual(count, 4)
        self.assertFalse(self.store.contracts or self.store.options or self.store.underlyings or self.store.chain_fetched)
        self.assertEqual(self.store.commits, 0)

    def test_database_failures_preserve_previously_committed_data(self):
        for failure in ("candle", "flag", "commit"):
            with self.subTest(failure=failure):
                self.store = Store()
                self.store.contracts[(7, 9, 2045000, "CALL")] = 90
                row = (90, datetime(2025, 11, 15, 0, 15, tzinfo=MARKET_ZONE), "5m", 1, 2, 1, 2, 3)
                self.store.options[row[:3]] = row
                before = deepcopy((self.store.contracts, self.store.options, self.store.underlyings))
                if failure == "candle": self.store.fail_batch = 3
                elif failure == "flag": self.store.fail_flag = True
                else: self.store.fail_commit = True
                response = self.post()
                self.assertEqual(response.status_code, 500, response.json)
                self.assertEqual((self.store.contracts, self.store.options, self.store.underlyings), before)
                self.assertEqual(self.store.rollbacks, 1)
                self.assertEqual(self.store.commits, 0)
                self.assertFalse(self.store.chain_fetched)

    def test_contract_insert_failure_rolls_back_underlying_and_contracts(self):
        original = database.insert_option_contract
        calls = 0
        def insert(*args):
            nonlocal calls
            calls += 1
            if calls == 2:
                raise OSError("contract insert failed")
            return original(*args)
        with patch("database.insert_option_contract", side_effect=insert):
            self.assertEqual(self.post().status_code, 500)
        self.assertEqual(self.store.rollbacks, 1)
        self.assertFalse(self.store.contracts or self.store.options or self.store.underlyings or self.store.chain_fetched)

    def test_invalid_candle_aborts_before_persistence(self):
        self.client.historical_option.side_effect = None
        self.client.historical_option.return_value = [{"datetime": "bad"}]
        response = self.post()
        self.assertEqual(response.status_code, 502)
        self.assertIn("strike_paise=2045000", response.json["error"])
        self.assertIn("invalid candle data", response.json["error"])
        self.assertEqual(self.client.historical_option.call_count, 1)
        self.assertFalse(self.store.contracts or self.store.underlyings or self.store.chain_fetched)

    def test_invalid_requests_and_missing_catalog(self):
        for flag in ("has_underlying", "has_expiry"):
            setattr(self.store, flag, False)
            self.assertEqual(self.post().status_code, 400)
            setattr(self.store, flag, True)
        for body in ({}, {"underlying": "NIFTY", "expiry": "bad"}, {**REQUEST, "timeframe": "1m"}):
            self.assertEqual(self.http.post("/option-chain/fetch", json=body).status_code, 400)
        self.get_client.assert_not_called()

    def test_real_client_retry_limits_and_abort(self):
        for failure, status, attempts in (("transient", 503, 3), ("permanent", 400, 1), ("recover", 429, 2)):
            with self.subTest(failure=failure), patch("breeze_client.time.sleep"):
                self.store = Store()
                client = BreezeClient.__new__(BreezeClient)
                client._sdk = Mock()
                client._last_request = 0
                client._request_day = datetime.now(MARKET_ZONE).date()
                client._daily_requests = 0
                option_attempts = 0
                def request(**params):
                    nonlocal option_attempts
                    self.fetch_phase()
                    if params["product_type"] == "options":
                        option_attempts += 1
                        if failure != "recover" or option_attempts == 1:
                            return {"Status": status, "Error": "temporary" if status != 400 else "invalid contract"}
                    at = datetime.strptime(params["from_date"], "%Y-%m-%dT%H:%M:%S.000Z").replace(tzinfo=MARKET_ZONE)
                    return {"Status": 200, "Error": None, "Success": [candle(at + timedelta(minutes=15))]}
                client._sdk.get_historical_data_v2.side_effect = request
                self.get_client.return_value = client
                response = self.post()
                if failure == "recover":
                    self.assertEqual(response.status_code, 200, response.json)
                    self.assertTrue(self.store.chain_fetched)
                    self.assertEqual(option_attempts, 83 * 2 * 3 + 1)
                else:
                    self.assertEqual(response.status_code, 502, response.json)
                    self.assertEqual(option_attempts, attempts)
                    self.assertEqual(self.store.commits, 0)
                    self.assertFalse(self.store.contracts or self.store.underlyings)
