import unittest
from datetime import datetime, timedelta
from decimal import Decimal
from unittest.mock import MagicMock, patch

from pipelines.underlying import MARKET_ZONE, candle_row, fetch_underlying, to_paise
from database import get_underlying_id, insert_candles


def candle(timestamp):
    return {"datetime": timestamp, "open": "22500.05", "high": 22510.1,
            "low": "22490.00", "close": "22505.25", "volume": "0"}


class ConversionTests(unittest.TestCase):
    def test_paise_uses_decimal(self):
        for value, expected in [("0.29", 29), (22500.05, 2250005), (Decimal("1.20"), 120)]:
            with self.subTest(value=value):
                self.assertEqual(to_paise(value), expected)
        for value in ("1.001", "NaN", "Infinity", "-1", str(2**63)):
            with self.subTest(value=value), self.assertRaises(ValueError):
                to_paise(value)

    def test_timestamp_and_schema_mapping(self):
        row = candle_row(candle("2026-09-01 09:15:00"), 7, "5m")
        self.assertEqual(row, (7, datetime(2026, 9, 1, 9, 15, tzinfo=MARKET_ZONE),
                               "5m", 2250005, 2251010, 2249000, 2250525, 0))
        utc_row = candle_row(candle("2026-09-01T03:45:00+00:00"), 7, "5m")
        self.assertEqual(utc_row, row)

    def test_invalid_volume(self):
        data = candle("2026-09-01 09:15:00")
        data["volume"] = "0.5"
        with self.assertRaises(ValueError):
            candle_row(data, 7, "5m")


class PipelineTests(unittest.TestCase):
    @patch("pipelines.underlying.BreezeClient")
    @patch("pipelines.underlying.insert_candles")
    @patch("pipelines.underlying.get_underlying_id", return_value=7)
    @patch("pipelines.underlying.connect")
    def test_chunking_and_rerun(self, connect, resolve, insert, client):
        connection = connect.return_value.__enter__.return_value
        requests = []
        stored = set()

        def historical(symbol, start, end, timeframe):
            requests.append((start, end))
            self.assertEqual((symbol, timeframe), ("NIFTY", "5minute"))
            self.assertLessEqual(end - start, timedelta(hours=12))
            return [candle((start + timedelta(minutes=15)).strftime("%Y-%m-%d %H:%M:%S"))]

        def persist(conn, rows):
            self.assertIs(conn, connection)
            added = 0
            for row in rows:
                self.assertEqual(row[2], "5m")
                key = (row[0], row[1], row[2])
                if key not in stored:
                    stored.add(key)
                    added += 1
            return added

        client.return_value.historical_underlying.side_effect = historical
        insert.side_effect = persist
        first = fetch_underlying(start_date="2026-09-01", end_date="2026-09-02")
        second = fetch_underlying(start_date="2026-09-01", end_date="2026-09-02")
        self.assertEqual(first, {"fetched": 4, "inserted": 4})
        self.assertEqual(second, {"fetched": 4, "inserted": 0})
        for previous, following in zip(requests[:3], requests[1:4]):
            self.assertEqual(previous[1] + timedelta(seconds=1), following[0])
        self.assertEqual(requests[3][1].isoformat(), "2026-09-02T23:59:59+05:30")

    @patch("pipelines.underlying.connect")
    def test_invalid_range_fails_before_connecting(self, connect):
        with self.assertRaises(ValueError):
            fetch_underlying(start_date="2026-10-01", end_date="2026-09-01")
        with self.assertRaises(ValueError):
            fetch_underlying(timeframe="15minute")
        connect.assert_not_called()

    @patch("pipelines.underlying.BreezeClient")
    @patch("pipelines.underlying.insert_candles")
    @patch("pipelines.underlying.get_underlying_id", return_value=7)
    @patch("pipelines.underlying.connect")
    def test_api_failure_is_propagated(self, connect, resolve, insert, client):
        client.return_value.historical_underlying.side_effect = RuntimeError("Breeze failed")
        with self.assertRaisesRegex(RuntimeError, "Breeze failed"):
            fetch_underlying()
        insert.assert_not_called()


class PersistenceTests(unittest.TestCase):
    def test_parameterized_lookup_and_duplicate_handling(self):
        connection = MagicMock()
        cursor = connection.cursor.return_value.__enter__.return_value
        cursor.fetchone.return_value = (7,)
        self.assertEqual(get_underlying_id(connection, "NIFTY"), 7)
        cursor.execute.assert_called_once_with(
            "SELECT id FROM lab.underlyings WHERE symbol = %s", ("NIFTY",))
        rows = [candle_row(candle("2026-09-01 09:15:00"), 7, "5m")]
        cursor.rowcount = 0
        self.assertEqual(insert_candles(connection, rows), 0)
        sql, params = cursor.executemany.call_args.args
        self.assertIn("ON CONFLICT (underlying_id, timeframe, start_time) DO NOTHING", sql)
        self.assertEqual(params, rows)
        cursor.fetchone.return_value = None
        with self.assertRaisesRegex(ValueError, "does not exist"):
            get_underlying_id(connection, "UNKNOWN")


if __name__ == "__main__":
    unittest.main()
