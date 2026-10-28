import os
import sys
import unittest
from datetime import datetime
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

from breeze_client import BreezeClient, MARKET_ZONE


class BreezeTests(unittest.TestCase):
    def setUp(self):
        self.sdk = MagicMock()
        self.module_patch = patch.dict(sys.modules, {"breeze_connect": SimpleNamespace(BreezeConnect=MagicMock(return_value=self.sdk))})
        self.env_patch = patch.dict(os.environ, {
            "BREEZE_API_KEY": "test-key", "BREEZE_API_SECRET": "test-secret", "BREEZE_SESSION_TOKEN": "test-token",
        })
        self.module_patch.start()
        self.env_patch.start()
        self.addCleanup(self.module_patch.stop)
        self.addCleanup(self.env_patch.stop)
        self.client = BreezeClient()
        self.start = datetime(2026, 9, 1, 9, 15, tzinfo=MARKET_ZONE)
        self.end = datetime(2026, 9, 1, 11, 59, 59, tzinfo=MARKET_ZONE)

    @patch("breeze_client.time.sleep")
    def test_nifty_cash_parameters_and_rate_spacing(self, sleep):
        self.sdk.get_historical_data_v2.return_value = {"Status": 200, "Error": None, "Success": []}
        self.assertEqual(self.client.historical_underlying("NIFTY", self.start, self.end), [])
        self.sdk.generate_session.assert_called_once_with(api_secret="test-secret", session_token="test-token")
        self.sdk.get_historical_data_v2.assert_called_once_with(
            interval="5minute", from_date="2026-09-01T09:15:00.000Z", to_date="2026-09-01T11:59:59.000Z",
            stock_code="NIFTY", exchange_code="NSE", product_type="cash",
        )
        sleep.assert_called_once()
        self.assertGreater(sleep.call_args.args[0], 0)

    @patch("breeze_client.time.sleep")
    def test_errors_and_truncation(self, sleep):
        for response in (None, {"Status": 500, "Error": "Historical Data Fail"},
                         {"Status": 200, "Success": None}, {"Status": 200, "Success": [{}] * 1000}):
            with self.subTest(response_type=type(response).__name__):
                self.sdk.get_historical_data_v2.return_value = response
                with self.assertRaises(RuntimeError):
                    self.client.historical_underlying("NIFTY", self.start, self.end)

    def test_daily_budget(self):
        self.client._daily_requests = 5000
        with self.assertRaisesRegex(RuntimeError, "daily request budget"):
            self.client.historical_underlying("NIFTY", self.start, self.end)
        self.sdk.get_historical_data_v2.assert_not_called()

    @patch("breeze_client.time.sleep")
    def test_options_parameters_and_transient_retry(self, sleep):
        self.sdk.get_historical_data_v2.side_effect = [
            {"Status": 429, "Error": "Rate limit exceeded"},
            {"Status": 200, "Error": None, "Success": []},
        ]
        rows = self.client.historical_option("NIFTY", self.start.date(), 2250050, "PUT", self.start, self.end)
        self.assertEqual(rows, [])
        self.assertEqual(self.sdk.get_historical_data_v2.call_count, 2)
        params = self.sdk.get_historical_data_v2.call_args.kwargs
        self.assertEqual(params["exchange_code"], "NFO")
        self.assertEqual(params["product_type"], "options")
        self.assertEqual(params["interval"], "5minute")
        self.assertEqual(params["right"], "put")
        self.assertEqual(params["strike_price"], "22500.5")
        self.assertEqual(params["expiry_date"], "2026-09-01T00:00:00.000Z")
        self.assertTrue(any(call.args == (2,) for call in sleep.call_args_list))

    @patch("breeze_client.time.sleep")
    def test_options_retry_exhaustion_and_permanent_error(self, sleep):
        for status, expected_calls in ((503, 3), (401, 1)):
            self.sdk.get_historical_data_v2.reset_mock()
            self.sdk.get_historical_data_v2.return_value = {"Status": status, "Error": "Request failed"}
            with self.assertRaises(RuntimeError):
                self.client.historical_option("NIFTY", self.start.date(), 2250000, "CALL", self.start, self.end)
            self.assertEqual(self.sdk.get_historical_data_v2.call_count, expected_calls)


if __name__ == "__main__":
    unittest.main()
