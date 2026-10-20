from contextlib import ExitStack
from datetime import date, timedelta
from pathlib import Path
import tempfile
import unittest
from unittest.mock import MagicMock, patch

from pipelines.options import ingest_options, input_dates, strike_bounds


class OptionInputTests(unittest.TestCase):
    def test_calendar_month_validation(self):
        for start, end in (("2026-09-01", "2026-09-30"), ("2024-02-01", "2024-02-29"),
                           ("2026-09-15", "2026-10-14")):
            with self.subTest(start=start, end=end):
                expiry, first, stop = input_dates("2026-09-29", start, end)
                self.assertEqual(expiry, date(2026, 9, 29))
                self.assertEqual(first.date().isoformat(), start)
                self.assertEqual((stop - timedelta(days=1)).date().isoformat(), end)
        for start, end in (("2026-09-02", "2026-09-01"), ("2026-09-01", "2026-10-01")):
            with self.assertRaises(ValueError):
                input_dates("2026-09-29", start, end)

    def test_strike_bounds_floor_ceil_and_padding(self):
        self.assertEqual(strike_bounds(2250001, 2264999, 5000, 0), (2250000, 2265000))
        self.assertEqual(strike_bounds(2250001, 2264999, 5000, 2), (2240000, 2275000))
        self.assertEqual(strike_bounds(2250001, 2264999, 5000, 20), (2150000, 2365000))
        self.assertEqual(strike_bounds(2250000, 2265000, 5000, 20), (2150000, 2365000))
        for values in ((100, 200, 0, 20), (-1, 200, 5000, 20), (100, 200, 5000, 20), (10000, 20000, 5000, -1)):
            with self.assertRaises(ValueError):
                strike_bounds(*values)


class OptionPipelineTests(unittest.TestCase):
    def setUp(self):
        self.stack = ExitStack()
        self.addCleanup(self.stack.close)
        temporary = self.stack.enter_context(tempfile.TemporaryDirectory())
        self.stack.enter_context(patch("pipelines.options.PROGRESS_PATH", Path(temporary) / "progress.sqlite3"))
        self.connect = self.stack.enter_context(patch("pipelines.options.connect"))
        connection = self.connect.return_value.__enter__.return_value
        connection.info.host, connection.info.port, connection.info.dbname = "localhost", 5432, "test"
        self.stack.enter_context(patch("pipelines.options.get_underlying_id", return_value=7))
        self.expiry_lookup = self.stack.enter_context(patch("pipelines.options.get_option_expiry_id", return_value=9))
        self.range_lookup = self.stack.enter_context(patch("pipelines.options.get_underlying_range", return_value=(2250001, 2264999)))
        self.contracts = self.stack.enter_context(patch("pipelines.options.ensure_option_contracts", return_value=[(10, 2250000, "CALL"), (11, 2250000, "PUT")]))
        self.insert = self.stack.enter_context(patch("pipelines.options.insert_option_candles", side_effect=lambda conn, rows: len(rows)))
        self.client_class = self.stack.enter_context(patch("pipelines.options.BreezeClient"))
        self.client = self.client_class.return_value

        def response(symbol, expiry, strike, right, start, end):
            # Intentionally outside market hours: must be preserved unchanged.
            return [{"datetime": (start + timedelta(hours=1)).isoformat(),
                     "open": "100.05", "high": "101.00", "low": "99.00", "close": "100.50", "volume": 0}]
        self.client.historical_option.side_effect = response

    def run_pipeline(self):
        return ingest_options("NIFTY", "2024-09-24", "2024-09-01", "2024-09-01", 5000, 20)

    def test_contract_windows_and_completed_rerun(self):
        first = self.run_pipeline()
        self.assertEqual(first["successful_calls"], 4)
        self.assertEqual(first["inserted"], 4)
        self.range_lookup.assert_called_once()
        self.assertEqual(self.contracts.call_args.args[3:], (2150000, 2365000, 5000))
        calls = self.client.historical_option.call_args_list
        self.assertEqual([c.args[3] for c in calls], ["CALL", "CALL", "PUT", "PUT"])
        self.assertEqual(calls[0].args[4:], calls[2].args[4:])
        self.assertEqual(calls[1].args[4:], calls[3].args[4:])
        row = self.insert.call_args_list[0].args[1][0]
        self.assertEqual((row[0], row[1].hour, row[2], row[3]), (10, 1, "5m", 10005))
        self.client_class.reset_mock()
        second = self.run_pipeline()
        self.assertEqual(second["skipped_windows"], 4)
        self.assertEqual(second["successful_calls"], 0)
        self.client_class.assert_not_called()

    def test_partial_failure_refetches_only_unfinished_windows(self):
        self.client.historical_option.side_effect = [[], RuntimeError("API failure")]
        with self.assertRaisesRegex(RuntimeError, "contract=10"):
            self.run_pipeline()
        self.client.historical_option.side_effect = lambda *args: []
        result = self.run_pipeline()
        self.assertEqual(result["skipped_windows"], 1)
        self.assertEqual(result["successful_calls"], 3)

    def test_missing_expiry_stops_before_api(self):
        self.expiry_lookup.side_effect = ValueError("Expiry does not exist")
        with self.assertRaisesRegex(ValueError, "Expiry does not exist"):
            self.run_pipeline()
        self.client_class.assert_not_called()
        self.contracts.assert_not_called()

    def test_database_failure_does_not_checkpoint(self):
        self.insert.side_effect = RuntimeError("insert failed")
        with self.assertRaisesRegex(RuntimeError, "contract=10"):
            self.run_pipeline()
        self.insert.side_effect = lambda conn, rows: len(rows)
        result = self.run_pipeline()
        self.assertEqual(result["successful_calls"], 4)
        self.assertEqual(result["skipped_windows"], 0)


if __name__ == "__main__":
    unittest.main()
