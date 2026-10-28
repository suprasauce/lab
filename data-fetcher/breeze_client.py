"""Existing Breeze authentication, rate pacing, and historical requests."""

import logging
import os
import time
from datetime import datetime
from zoneinfo import ZoneInfo


logger = logging.getLogger(__name__)


class RetryableBreezeError(RuntimeError):
    """A transient historical-data error that may be retried safely."""


MARKET_ZONE = ZoneInfo("Asia/Kolkata")
MAX_CANDLES = 1000
_client = None


def get_client():
    """Reuse authentication and the rate budget in this single-threaded service."""
    global _client
    if _client is None:
        try:
            _client = BreezeClient()
        except ValueError as exc:
            raise RuntimeError("Breeze credentials are missing; configure the service environment") from exc
    return _client


class BreezeClient:
    def __init__(self):
        credentials = {}
        for name in ("BREEZE_API_KEY", "BREEZE_API_SECRET", "BREEZE_SESSION_TOKEN"):
            value = os.environ.get(name)
            if not value:
                raise ValueError(f"{name} is required")
            credentials[name] = value

        # Import lazily: the SDK may download its security master during import.
        from breeze_connect import BreezeConnect

        self._sdk = BreezeConnect(api_key=credentials["BREEZE_API_KEY"])
        try:
            self._sdk.generate_session(
                api_secret=credentials["BREEZE_API_SECRET"],
                session_token=credentials["BREEZE_SESSION_TOKEN"],
            )
        except Exception as exc:
            raise RuntimeError("Breeze authentication failed; check API credentials and refresh the session token") from exc
        self._last_request = time.monotonic()
        self._request_day = datetime.now(MARKET_ZONE).date()
        self._daily_requests = 1  # Include session authentication in this process.

    def _historical_request(self, **parameters):
        today = datetime.now(MARKET_ZONE).date()
        if today != self._request_day:
            self._request_day = today
            self._daily_requests = 0
        if self._daily_requests >= 5000:
            raise RuntimeError("Breeze daily request budget reached; rerun on another day")
        # One request per second stays below 100/minute for this client process.
        time.sleep(max(0, 1.0 - (time.monotonic() - self._last_request)))
        self._last_request = time.monotonic()
        self._daily_requests += 1

        from requests import RequestException

        try:
            response = self._sdk.get_historical_data_v2(**parameters)
        except RequestException as exc:
            raise RetryableBreezeError("Breeze historical network request failed") from exc
        if not isinstance(response, dict):
            raise RetryableBreezeError("Breeze historical request returned an invalid response")
        if response.get("Status") != 200 or response.get("Error"):
            # Never include the request/credentials in the error message.
            status = str(response.get("Status"))
            error = str(response.get("Error"))
            retryable = status in {"429", "500", "502", "503", "504"} or "limit exceed" in error.lower() or "rate limit" in error.lower()
            exception = RetryableBreezeError if retryable else RuntimeError
            raise exception(f"Breeze historical request failed: status={status}, error={error}")
        candles = response.get("Success")
        if not isinstance(candles, list):
            raise RuntimeError("Breeze historical response did not contain a candle list")
        if len(candles) >= MAX_CANDLES:
            raise RuntimeError("Breeze candle limit reached; refusing potentially truncated data")
        return candles

    def historical_underlying(self, symbol, start, end):
        return self._historical_request(
            interval="5minute",
            from_date=start.astimezone(MARKET_ZONE).strftime("%Y-%m-%dT%H:%M:%S.000Z"),
            to_date=end.astimezone(MARKET_ZONE).strftime("%Y-%m-%dT%H:%M:%S.000Z"),
            stock_code=symbol, exchange_code="NSE", product_type="cash",
        )

    def historical_option(self, symbol, expiry, strike, option_type, start, end):
        """Read one 5-minute options window, retrying transient errors three times."""
        from decimal import Decimal

        parameters = dict(
            interval="5minute",
            from_date=start.astimezone(MARKET_ZONE).strftime("%Y-%m-%dT%H:%M:%S.000Z"),
            to_date=end.astimezone(MARKET_ZONE).strftime("%Y-%m-%dT%H:%M:%S.000Z"),
            stock_code=symbol, exchange_code="NFO", product_type="options",
            expiry_date=expiry.strftime("%Y-%m-%dT00:00:00.000Z"),
            right=option_type.lower(), strike_price=format(Decimal(strike) / 100, "f"),
        )
        for attempt in range(3):
            try:
                return self._historical_request(**parameters)
            except RetryableBreezeError:
                if attempt == 2:
                    raise
                delay = 2 ** (attempt + 1)
                logger.warning("Retrying Breeze options request stock=%s strike_paise=%d right=%s attempt=%d backoff_seconds=%d",
                               symbol, strike, option_type, attempt + 1, delay)
                time.sleep(delay)
