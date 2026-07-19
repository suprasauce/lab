"""Market data access: DuckDB first, Breeze only for missing data."""

from __future__ import annotations

import logging
from datetime import date, time
from typing import TYPE_CHECKING

from backend.config.settings import load_credentials
from backend.dao.market_data_dao import MarketDataDao
from backend.common.utils import day_session_breeze_range, expiry_to_breeze_iso, normalize_candle_df

if TYPE_CHECKING:
    from backend.client.breeze_client import BreezeClient

logger = logging.getLogger(__name__)
_service: _MarketDataService | None = None


class _MarketDataService:
    def __init__(self, client: "BreezeClient", dao: MarketDataDao):
        self.client = client
        self.dao = dao

    def get_underlying_candle(
        self,
        *,
        symbol: str,
        exchange: str,
        candle_date: date,
        candle_time: time,
    ) -> dict | None:
        candle = self.dao.load_underlying_candle(
            symbol=symbol,
            exchange=exchange,
            candle_date=candle_date,
            candle_time=candle_time,
        )
        if candle is not None:
            return candle

        self._fetch_underlying_day(symbol=symbol, exchange=exchange, candle_date=candle_date)
        return self.dao.load_underlying_candle(
            symbol=symbol,
            exchange=exchange,
            candle_date=candle_date,
            candle_time=candle_time,
        )

    def get_option_candle(
        self,
        *,
        symbol: str,
        exchange: str,
        expiry: date,
        strike: int,
        right: str,
        candle_date: date,
        candle_time: time,
    ) -> dict | None:
        right = right.lower()
        candle = self.dao.load_derivative_candle(
            underlying_symbol=symbol,
            exchange=exchange,
            instrument_type="option",
            expiry=expiry,
            strike=strike,
            right=right,
            candle_date=candle_date,
            candle_time=candle_time,
        )
        if candle is not None:
            return candle

        self._fetch_option_day(
            symbol=symbol,
            exchange=exchange,
            expiry=expiry,
            strike=strike,
            right=right,
            candle_date=candle_date,
        )
        return self.dao.load_derivative_candle(
            underlying_symbol=symbol,
            exchange=exchange,
            instrument_type="option",
            expiry=expiry,
            strike=strike,
            right=right,
            candle_date=candle_date,
            candle_time=candle_time,
        )

    def _fetch_underlying_day(self, *, symbol: str, exchange: str, candle_date: date) -> None:
        logger.info("Fetching underlying %s %s on %s", exchange, symbol, candle_date)
        from_iso, to_iso = day_session_breeze_range(candle_date)
        raw = self.client.get_historical_5min(
            from_date=from_iso,
            to_date=to_iso,
            stock_code=symbol,
            exchange_code=exchange,
            product_type="cash",
        )
        df = normalize_candle_df(raw)
        if df.empty:
            logger.warning("No underlying data %s %s on %s", exchange, symbol, candle_date)
            return
        self.dao.save_underlying_5m(symbol=symbol, exchange=exchange, df=df)

    def _fetch_option_day(
        self,
        *,
        symbol: str,
        exchange: str,
        expiry: date,
        strike: int,
        right: str,
        candle_date: date,
    ) -> None:
        logger.info("Fetching option %s %s %s %s on %s", expiry, strike, right, symbol, candle_date)
        from_iso, to_iso = day_session_breeze_range(candle_date)
        raw = self.client.get_historical_5min(
            from_date=from_iso,
            to_date=to_iso,
            stock_code=symbol,
            exchange_code=exchange,
            product_type="options",
            expiry_date=expiry_to_breeze_iso(expiry),
            right=right,
            strike_price=str(strike),
        )
        df = normalize_candle_df(raw)
        if df.empty:
            logger.debug("No option data %s %s %s on %s", expiry, strike, right, candle_date)
            return
        self.dao.save_derivative_5m(
            underlying_symbol=symbol,
            exchange=exchange,
            instrument_type="option",
            expiry=expiry,
            strike=strike,
            right=right,
            df=df,
        )


def get_underlying_candle(
    *,
    symbol: str,
    exchange: str,
    candle_date: date,
    candle_time: time,
) -> dict | None:
    return _get_market_data_service().get_underlying_candle(
        symbol=symbol,
        exchange=exchange,
        candle_date=candle_date,
        candle_time=candle_time,
    )


def get_option_candle(
    *,
    symbol: str,
    exchange: str,
    expiry: date,
    strike: int,
    right: str,
    candle_date: date,
    candle_time: time,
) -> dict | None:
    return _get_market_data_service().get_option_candle(
        symbol=symbol,
        exchange=exchange,
        expiry=expiry,
        strike=strike,
        right=right,
        candle_date=candle_date,
        candle_time=candle_time,
    )


def _get_market_data_service() -> _MarketDataService:
    global _service
    if _service is None:
        _service = _create_market_data_service()
    return _service


def _create_market_data_service() -> _MarketDataService:
    from backend.client.breeze_client import BreezeClient

    return _MarketDataService(BreezeClient(load_credentials()), MarketDataDao())
