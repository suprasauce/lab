"""Read-only Upstox instrument search and historical candle requests."""

import os
import time
from decimal import Decimal
from urllib.parse import quote

import requests


class UpstoxClient:
    def __init__(self):
        token = os.environ.get("UPSTOX_ACCESS_TOKEN")
        if not token:
            raise RuntimeError("UPSTOX_ACCESS_TOKEN is required")
        self.session = requests.Session()
        self.session.headers.update({"Authorization": "Bearer " + token, "Accept": "application/json"})
        self.last_request = 0.0

    def _get(self, path, params=None):
        # One request/second stays below the documented short and 30-minute limits.
        time.sleep(max(0, 1.0 - (time.monotonic() - self.last_request)))
        self.last_request = time.monotonic()
        try:
            response = self.session.get("https://api.upstox.com" + path, params=params, timeout=30, allow_redirects=False)
        except requests.RequestException as exc:
            raise RuntimeError("Upstox request failed; check network connectivity") from exc
        try:
            if not 200 <= response.status_code < 300:
                raise RuntimeError(f"Upstox API returned HTTP {response.status_code}")
            try:
                payload = response.json(parse_float=Decimal)
            except ValueError as exc:
                raise RuntimeError("Upstox returned invalid JSON") from exc
            if not isinstance(payload, dict) or payload.get("status") != "success" or "data" not in payload:
                raise RuntimeError("Upstox response did not confirm success")
            return payload
        finally:
            response.close()

    def resolve_underlying(self, symbol):
        matches = set()
        page = 1
        while True:
            result = self._get("/v2/instruments/search", {
                "query": symbol, "exchanges": "NSE", "segments": "INDEX",
                "page_number": page, "records": 30,
            })
            if not isinstance(result["data"], list):
                raise RuntimeError("Upstox search returned invalid instruments")
            for instrument in result["data"]:
                if instrument.get("segment") == "NSE_INDEX" and any(
                    str(instrument.get(field, "")).upper() == symbol.upper()
                    for field in ("trading_symbol", "name")
                ):
                    key = instrument.get("instrument_key")
                    if key:
                        matches.add(key)
            if page >= result.get("meta_data", {}).get("page", {}).get("total_pages", page):
                break
            page += 1
        if len(matches) != 1:
            raise ValueError(f"Expected one exact NSE index match for {symbol!r}; found {len(matches)}")
        return matches.pop()

    def historical_underlying(self, instrument_key, start, end):
        path = f"/v3/historical-candle/{quote(instrument_key, safe='')}/minutes/1/{end.isoformat()}/{start.isoformat()}"
        return self._historical_candles(path)

    def expired_option_contracts(self, instrument_key, expiry):
        data = self._get("/v2/expired-instruments/option/contract", {
            "instrument_key": instrument_key, "expiry_date": expiry.isoformat(),
        })["data"]
        if not isinstance(data, list):
            raise RuntimeError("Upstox returned an invalid option contract list")
        return data

    def historical_option(self, instrument_key, start, end):
        path = f"/v2/expired-instruments/historical-candle/{quote(instrument_key, safe='')}/1minute/{end.isoformat()}/{start.isoformat()}"
        return self._historical_candles(path)

    def _historical_candles(self, path):
        data = self._get(path)["data"]
        if not isinstance(data, dict) or not isinstance(data.get("candles"), list):
            raise RuntimeError("Upstox historical response did not contain candles")
        return data["candles"]
