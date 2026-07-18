"""Thin wrapper around breeze-connect with retries."""

from __future__ import annotations

import logging
import time
from typing import Any

from breeze_connect import BreezeConnect

from common.settings import BreezeCredentials

logger = logging.getLogger(__name__)


class BreezeClient:
    def __init__(self, creds: BreezeCredentials, pause_sec: float = 0.4, max_retries: int = 3):
        self._pause = pause_sec
        self._max_retries = max_retries
        self._breeze = BreezeConnect(api_key=creds.api_key)
        self._breeze.generate_session(
            api_secret=creds.api_secret,
            session_token=creds.session_token,
        )

    def _call(self, fn_name: str, **kwargs: Any) -> dict:
        fn = getattr(self._breeze, fn_name)
        last_err: Exception | None = None
        for attempt in range(self._max_retries):
            try:
                time.sleep(self._pause)
                result = fn(**kwargs)
                if isinstance(result, dict) and result.get("Status") == 200:
                    return result
                if isinstance(result, dict) and result.get("Error"):
                    logger.warning("Breeze %s error: %s", fn_name, result.get("Error"))
                return result if isinstance(result, dict) else {"Success": result}
            except Exception as e:
                last_err = e
                logger.warning("Breeze %s attempt %d failed: %s", fn_name, attempt + 1, e)
                time.sleep(1.0 * (attempt + 1))
        raise RuntimeError(f"Breeze {fn_name} failed after retries") from last_err

    def get_historical_5min(
        self,
        *,
        from_date: str,
        to_date: str,
        stock_code: str,
        exchange_code: str,
        product_type: str,
        expiry_date: str = "",
        right: str = "",
        strike_price: str = "",
    ) -> list[dict]:
        kwargs: dict[str, Any] = {
            "interval": "5minute",
            "from_date": from_date,
            "to_date": to_date,
            "stock_code": stock_code,
            "exchange_code": exchange_code,
            "product_type": product_type,
        }
        if product_type == "options":
            kwargs.update(
                expiry_date=expiry_date,
                right=right,
                strike_price=strike_price,
            )
        elif product_type == "futures":
            kwargs.update(expiry_date=expiry_date, right="others", strike_price="0")
        result = self._call("get_historical_data_v2", **kwargs)
        success = result.get("Success") or []
        return success if isinstance(success, list) else []
