"""Offline Upstox search, candle, and persistence tests."""

import os
import unittest
from datetime import date
from decimal import Decimal
from unittest.mock import Mock, patch

import requests
from main import app
from providers.upstox import UpstoxProvider
from providers.upstox_client import UpstoxClient
from test_service import Store


class UpstoxTests(unittest.TestCase):
    def setUp(self):
        self.store = Store()
        self.client = Mock()
        self.client.resolve_underlying.return_value = "NSE_INDEX|returned-by-search"
        self.client.historical_underlying.side_effect = self.candles
        self.client.expired_option_contracts.return_value = [
            {"underlying_key": "NSE_INDEX|returned-by-search", "expiry": "2026-01-27",
             "instrument_key": "NSE_FO|101|27-01-2026", "instrument_type": "CE", "strike_price": 25000},
            {"underlying_key": "NSE_INDEX|returned-by-search", "expiry": "2026-01-27",
             "instrument_key": "NSE_FO|102|27-01-2026", "instrument_type": "PE", "strike_price": 25000},
        ]
        self.client.historical_option.side_effect = lambda key, start, end: self.candles("NSE_INDEX|returned-by-search", start, end)
        for context in (
            patch("database.connect", side_effect=lambda: self.store.connect()),
            patch("providers.upstox.UpstoxClient", return_value=self.client),
            patch.dict(app.config, {"DATA_PROVIDER": UpstoxProvider()}),
        ):
            context.start()
            self.addCleanup(context.stop)
        self.http = app.test_client()

    def candles(self, key, start, end):
        self.assertEqual(key, "NSE_INDEX|returned-by-search")
        self.assertEqual(self.store.connections, 0)
        self.assertFalse(self.store.in_transaction)
        row = [start.isoformat() + "T09:15:00+05:30", Decimal("100.05"), 110, 90, Decimal("101.29"), 12, 0]
        return [row, row]  # Deduplicate identical timestamps before inserting.

    def post(self):
        return self.http.post("/underlyings/fetch", json={
            "underlying": "NIFTY", "start_date": "2026-01-31", "end_date": "2026-03-02",
        })

    def test_thirty_day_windows_conversion_and_idempotency(self):
        response = self.post()
        self.assertEqual(response.status_code, 200, response.json)
        self.assertEqual(response.json, {"status": "ingested", "fetched": 2, "inserted": 2})
        calls = self.client.historical_underlying.call_args_list
        self.assertEqual(calls[0].args[1:], (date(2026, 1, 31), date(2026, 3, 1)))
        self.assertEqual(calls[1].args[1:], (date(2026, 3, 2), date(2026, 3, 2)))
        self.assertTrue(all((call.args[2] - call.args[1]).days + 1 <= 30 for call in calls))
        row = next(iter(self.store.underlyings.values()))
        self.assertEqual(row[2:], ("1m", 10005, 11000, 9000, 10129, 12))
        self.assertEqual(row[1].utcoffset().total_seconds(), 19800)
        self.assertEqual(self.post().json["inserted"], 0)
        self.assertEqual(self.store.commits, 2)

    def test_failure_before_writes_and_rollback(self):
        self.client.historical_underlying.side_effect = [self.candles("NSE_INDEX|returned-by-search", date(2026, 1, 31), date(2026, 1, 31)), RuntimeError("upstream failed")]
        self.assertEqual(self.post().status_code, 502)
        self.assertFalse(self.store.underlyings)
        self.assertEqual(self.store.commits, 0)
        self.client.historical_underlying.side_effect = self.candles
        self.store.fail_commit = True
        self.assertEqual(self.post().status_code, 500)
        self.assertFalse(self.store.underlyings)
        self.assertEqual(self.store.rollbacks, 1)

    def test_invalid_candle_and_empty_history(self):
        self.client.historical_underlying.side_effect = None
        self.client.historical_underlying.return_value = [["bad"]]
        self.assertEqual(self.post().status_code, 502)
        self.assertFalse(self.store.underlyings)
        self.client.historical_underlying.return_value = []
        self.assertEqual(self.post().json, {"status": "ingested", "fetched": 0, "inserted": 0})

    def test_missing_underlying_prevents_api_calls(self):
        self.store.has_underlying = False
        self.assertEqual(self.post().status_code, 400)
        self.client.resolve_underlying.assert_not_called()

    def chain(self):
        return self.http.post("/option-chain/fetch", json={"underlying": "NIFTY", "expiry": "2026-01-27"})

    def test_chain_discovery_windows_commit_and_rerun(self):
        response = self.chain()
        self.assertEqual(response.status_code, 200, response.json)
        self.assertEqual(response.json, {"status": "ingested", "expiry_id": 9, "contracts": 2,
                                        "fetched": 4, "inserted": 4, "underlying_inserted": 2})
        self.assertEqual(set(self.store.contracts), {(7, 9, 2500000, "CALL"), (7, 9, 2500000, "PUT")})
        self.assertTrue(all(row[2] == "1m" for row in self.store.options.values()))
        self.assertTrue(self.store.chain_fetched)
        self.assertEqual(self.store.commits, 1)
        self.client.expired_option_contracts.assert_called_once_with("NSE_INDEX|returned-by-search", date(2026, 1, 27))
        calls = self.client.historical_option.call_args_list
        self.assertEqual(calls[0].args, ("NSE_FO|101|27-01-2026", date(2025, 12, 13), date(2026, 1, 11)))
        self.assertEqual(calls[1].args[1:], (date(2026, 1, 12), date(2026, 1, 27)))
        self.assertEqual(calls[2].args[0], "NSE_FO|102|27-01-2026")
        self.assertEqual(self.chain().json["status"], "already_exists")
        self.assertEqual(self.client.historical_option.call_count, 4)

    def test_chain_reuses_contracts_and_skips_existing_candles(self):
        self.assertEqual(self.chain().status_code, 200)
        self.store.chain_fetched = False
        response = self.chain()
        self.assertEqual(response.status_code, 200, response.json)
        self.assertEqual(response.json["inserted"], 0)
        self.assertEqual(len(self.store.contracts), 2)
        self.assertEqual(len(self.store.options), 4)

    def test_chain_failure_commits_nothing(self):
        def request(key, start, end):
            if key == "NSE_FO|102|27-01-2026":
                raise RuntimeError("option request failed")
            return self.candles("NSE_INDEX|returned-by-search", start, end)
        self.client.historical_option.side_effect = request
        response = self.chain()
        self.assertEqual(response.status_code, 502)
        self.assertIn("option_type=PUT", response.json["error"])
        self.assertFalse(self.store.contracts or self.store.options or self.store.underlyings or self.store.chain_fetched)
        self.assertEqual(self.store.commits, 0)

    def test_chain_insert_flag_and_commit_failures_roll_back(self):
        for failure in ("candle", "flag", "commit"):
            with self.subTest(failure=failure):
                self.store = Store()
                if failure == "candle": self.store.fail_batch = 2
                elif failure == "flag": self.store.fail_flag = True
                else: self.store.fail_commit = True
                self.assertEqual(self.chain().status_code, 500)
                self.assertFalse(self.store.contracts or self.store.options or self.store.underlyings or self.store.chain_fetched)
                self.assertEqual(self.store.rollbacks, 1)

    def test_chain_empty_discovery_is_not_marked_complete(self):
        self.client.expired_option_contracts.return_value = []
        response = self.chain()
        self.assertEqual(response.status_code, 502)
        self.assertIn("not available", response.json["error"])
        self.client.historical_underlying.assert_not_called()
        self.assertFalse(self.store.chain_fetched)

    def test_chain_empty_candle_history_and_invalid_metadata(self):
        self.client.historical_option.side_effect = None
        self.client.historical_option.return_value = []
        self.assertEqual(self.chain().status_code, 200)
        self.assertTrue(self.store.chain_fetched)
        self.assertEqual(len(self.store.contracts), 2)
        self.assertFalse(self.store.options)
        self.store = Store()
        self.client.expired_option_contracts.return_value = [{"expiry": "wrong"}]
        self.assertEqual(self.chain().status_code, 502)
        self.assertFalse(self.store.chain_fetched or self.store.contracts)


class UpstoxClientTests(unittest.TestCase):
    @patch.dict(os.environ, {"UPSTOX_ACCESS_TOKEN": "test-token"})
    def test_expired_endpoints(self):
        client = UpstoxClient()
        with patch.object(client, "_get") as get:
            get.return_value = {"data": [{"instrument_key": "expired-key"}]}
            self.assertEqual(len(client.expired_option_contracts("index-key", date(2026, 1, 27))), 1)
            get.assert_called_once_with("/v2/expired-instruments/option/contract", {"instrument_key": "index-key", "expiry_date": "2026-01-27"})
            get.return_value = {"data": {"candles": []}}
            self.assertEqual(client.historical_option("NSE_FO|101|27-01-2026", date(2025, 12, 13), date(2026, 1, 11)), [])
            get.assert_called_with("/v2/expired-instruments/historical-candle/NSE_FO%7C101%7C27-01-2026/1minute/2026-01-11/2025-12-13")

    @patch.dict(os.environ, {"UPSTOX_ACCESS_TOKEN": "test-token"})
    @patch("providers.upstox_client.time.sleep")
    def test_requests_and_exact_search_with_pagination(self, sleep):
        client = UpstoxClient()
        with patch.object(client, "_get") as get:
            get.side_effect = [
                {"data": [{"segment": "NSE_INDEX", "trading_symbol": "NIFTY BANK", "instrument_key": "wrong"}], "meta_data": {"page": {"total_pages": 2}}},
                {"data": [{"segment": "NSE_INDEX", "trading_symbol": "NIFTY", "instrument_key": "NSE_INDEX|from-search"}], "meta_data": {"page": {"total_pages": 2}}},
            ]
            self.assertEqual(client.resolve_underlying("NIFTY"), "NSE_INDEX|from-search")
            self.assertEqual(get.call_args.args[1]["page_number"], 2)
            get.side_effect = None
            get.return_value = {"data": []}
            with self.assertRaisesRegex(ValueError, "found 0"):
                client.resolve_underlying("NIFTY")
        response = Mock(status_code=200)
        response.json.return_value = {"status": "success", "data": {"candles": []}}
        with patch.object(client.session, "get", return_value=response) as get:
            self.assertEqual(client.historical_underlying("NSE_INDEX|from-search", date(2026, 1, 1), date(2026, 1, 31)), [])
            self.assertIn("NSE_INDEX%7Cfrom-search/minutes/1/2026-01-31/2026-01-01", get.call_args.args[0])
            self.assertEqual(get.call_args.kwargs["timeout"], 30)
        response.close.assert_called_once()

    @patch.dict(os.environ, {"UPSTOX_ACCESS_TOKEN": "test-token"})
    @patch("providers.upstox_client.time.sleep")
    def test_http_and_network_errors(self, sleep):
        client = UpstoxClient()
        for code in (401, 429, 500):
            with patch.object(client.session, "get", return_value=Mock(status_code=code)):
                with self.assertRaisesRegex(RuntimeError, str(code)):
                    client._get("/test")
        with patch.object(client.session, "get", side_effect=requests.Timeout):
            with self.assertRaisesRegex(RuntimeError, "network"):
                client._get("/test")
