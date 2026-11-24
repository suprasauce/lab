"""Provider selection and HTTP delegation; no live services required."""

import unittest
from unittest.mock import Mock, patch

from main import app
from providers import create_provider
from providers.breeze import BreezeProvider
from providers.upstox import UpstoxProvider


class ProviderTests(unittest.TestCase):
    def test_selection(self):
        self.assertIsInstance(create_provider("breeze"), BreezeProvider)
        self.assertIsInstance(create_provider("upstox"), UpstoxProvider)
        with self.assertRaisesRegex(ValueError, "Unsupported MARKET_DATA_PROVIDER"):
            create_provider("unknown")

    def test_routes_reuse_selected_provider(self):
        provider = Mock()
        provider.fetch_underlying.return_value = {"status": "ingested", "fetched": 0, "inserted": 0}
        provider.fetch_option_chain.return_value = {"status": "already_exists", "expiry_id": 9}
        with patch.dict(app.config, {"DATA_PROVIDER": provider}):
            http = app.test_client()
            underlying = {"underlying": "NIFTY", "start_date": "2026-01-01", "end_date": "2026-01-31"}
            for _ in range(2):
                self.assertEqual(http.post("/underlyings/fetch", json=underlying).status_code, 200)
            chain = {"underlying": "NIFTY", "expiry": "2026-01-27"}
            self.assertEqual(http.post("/option-chain/fetch", json=chain).status_code, 200)
            self.assertIs(app.config["DATA_PROVIDER"], provider)
        self.assertEqual(provider.fetch_underlying.call_count, 2)
        provider.fetch_underlying.assert_called_with(**underlying)
        provider.fetch_option_chain.assert_called_once_with(**chain)
