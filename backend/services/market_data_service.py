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
logger.setLevel(logging.INFO)
if not logger.handlers:
    handler = logging.StreamHandler()
    handler.setFormatter(
        logging.Formatter("%(asctime)s %(levelname)s %(name)s - %(message)s")
    )
    logger.addHandler(handler)
logger.propagate = False
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
        logger.info(
            "Market data request underlying symbol=%s exchange=%s date=%s time=%s",
            symbol,
            exchange,
            candle_date,
            candle_time,
        )
        candle = self.dao.load_underlying_candle(
            symbol=symbol,
            exchange=exchange,
            candle_date=candle_date,
            candle_time=candle_time,
        )
        if candle is not None:
            logger.info(
                "DuckDB hit underlying symbol=%s exchange=%s date=%s time=%s",
                symbol,
                exchange,
                candle_date,
                candle_time,
            )
            return candle

        logger.info(
            "DuckDB miss underlying symbol=%s exchange=%s date=%s time=%s",
            symbol,
            exchange,
            candle_date,
            candle_time,
        )
        if self.dao.is_day_marked_missing(
            symbol=symbol,
            exchange=exchange,
            instrument_type="underlying",
            data_date=candle_date,
        ):
            logger.warning(
                "Missing marker hit underlying symbol=%s exchange=%s date=%s",
                symbol,
                exchange,
                candle_date,
            )
            return None

        self._fetch_underlying_day(symbol=symbol, exchange=exchange, candle_date=candle_date)
        candle = self.dao.load_underlying_candle(
            symbol=symbol,
            exchange=exchange,
            candle_date=candle_date,
            candle_time=candle_time,
        )
        if candle is not None:
            logger.info(
                "Resolved underlying from Breeze cache symbol=%s exchange=%s date=%s time=%s",
                symbol,
                exchange,
                candle_date,
                candle_time,
            )
        else:
            logger.warning(
                "Still missing underlying after Breeze fetch symbol=%s exchange=%s date=%s time=%s",
                symbol,
                exchange,
                candle_date,
                candle_time,
            )
            self.dao.mark_day_missing(
                symbol=symbol,
                exchange=exchange,
                instrument_type="underlying",
                data_date=candle_date,
                reason="requested_candle_missing_after_fetch",
            )
            logger.info(
                "Marked day missing underlying symbol=%s exchange=%s date=%s reason=requested_candle_missing_after_fetch",
                symbol,
                exchange,
                candle_date,
            )
        return candle

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
        logger.info(
            "Market data request option symbol=%s exchange=%s expiry=%s strike=%s right=%s date=%s time=%s",
            symbol,
            exchange,
            expiry,
            strike,
            right,
            candle_date,
            candle_time,
        )
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
            logger.info(
                "DuckDB hit option symbol=%s exchange=%s expiry=%s strike=%s right=%s date=%s time=%s",
                symbol,
                exchange,
                expiry,
                strike,
                right,
                candle_date,
                candle_time,
            )
            return candle

        logger.info(
            "DuckDB miss option symbol=%s exchange=%s expiry=%s strike=%s right=%s date=%s time=%s",
            symbol,
            exchange,
            expiry,
            strike,
            right,
            candle_date,
            candle_time,
        )
        if self.dao.is_day_marked_missing(
            symbol=symbol,
            exchange=exchange,
            instrument_type="option",
            expiry=expiry,
            strike=strike,
            right=right,
            data_date=candle_date,
        ):
            logger.warning(
                "Missing marker hit option symbol=%s exchange=%s expiry=%s strike=%s right=%s date=%s",
                symbol,
                exchange,
                expiry,
                strike,
                right,
                candle_date,
            )
            return None

        self._fetch_option_day(
            symbol=symbol,
            exchange=exchange,
            expiry=expiry,
            strike=strike,
            right=right,
            candle_date=candle_date,
        )
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
            logger.info(
                "Resolved option from Breeze cache symbol=%s exchange=%s expiry=%s strike=%s right=%s date=%s time=%s",
                symbol,
                exchange,
                expiry,
                strike,
                right,
                candle_date,
                candle_time,
            )
        else:
            logger.warning(
                "Still missing option after Breeze fetch symbol=%s exchange=%s expiry=%s strike=%s right=%s date=%s time=%s",
                symbol,
                exchange,
                expiry,
                strike,
                right,
                candle_date,
                candle_time,
            )
            self.dao.mark_day_missing(
                symbol=symbol,
                exchange=exchange,
                instrument_type="option",
                expiry=expiry,
                strike=strike,
                right=right,
                data_date=candle_date,
                reason="requested_candle_missing_after_fetch",
            )
            logger.info(
                "Marked day missing option symbol=%s exchange=%s expiry=%s strike=%s right=%s date=%s reason=requested_candle_missing_after_fetch",
                symbol,
                exchange,
                expiry,
                strike,
                right,
                candle_date,
            )
        return candle

    def _fetch_underlying_day(self, *, symbol: str, exchange: str, candle_date: date) -> None:
        logger.info(
            "Breeze fetch underlying day start symbol=%s exchange=%s date=%s",
            symbol,
            exchange,
            candle_date,
        )
        from_iso, to_iso = day_session_breeze_range(candle_date)
        raw = self.client.get_historical_5min(
            from_date=from_iso,
            to_date=to_iso,
            stock_code=symbol,
            exchange_code=exchange,
            product_type="cash",
        )
        df = normalize_candle_df(raw)
        logger.info(
            "Breeze returned underlying rows=%s symbol=%s exchange=%s date=%s",
            len(df),
            symbol,
            exchange,
            candle_date,
        )
        if df.empty:
            logger.warning("No underlying data %s %s on %s", exchange, symbol, candle_date)
            self.dao.mark_day_missing(
                symbol=symbol,
                exchange=exchange,
                instrument_type="underlying",
                data_date=candle_date,
                reason="provider_no_rows",
            )
            logger.info(
                "Marked day missing underlying symbol=%s exchange=%s date=%s reason=provider_no_rows",
                symbol,
                exchange,
                candle_date,
            )
            return
        self.dao.save_underlying_5m(symbol=symbol, exchange=exchange, df=df)
        logger.info(
            "DuckDB saved underlying rows=%s symbol=%s exchange=%s date=%s",
            len(df),
            symbol,
            exchange,
            candle_date,
        )

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
        logger.info(
            "Breeze fetch option day start symbol=%s exchange=%s expiry=%s strike=%s right=%s date=%s",
            symbol,
            exchange,
            expiry,
            strike,
            right,
            candle_date,
        )
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
        logger.info(
            "Breeze returned option rows=%s symbol=%s exchange=%s expiry=%s strike=%s right=%s date=%s",
            len(df),
            symbol,
            exchange,
            expiry,
            strike,
            right,
            candle_date,
        )
        if df.empty:
            logger.warning("No option data %s %s %s on %s", expiry, strike, right, candle_date)
            self.dao.mark_day_missing(
                symbol=symbol,
                exchange=exchange,
                instrument_type="option",
                expiry=expiry,
                strike=strike,
                right=right,
                data_date=candle_date,
                reason="provider_no_rows",
            )
            logger.info(
                "Marked day missing option symbol=%s exchange=%s expiry=%s strike=%s right=%s date=%s reason=provider_no_rows",
                symbol,
                exchange,
                expiry,
                strike,
                right,
                candle_date,
            )
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
        logger.info(
            "DuckDB saved option rows=%s symbol=%s exchange=%s expiry=%s strike=%s right=%s date=%s",
            len(df),
            symbol,
            exchange,
            expiry,
            strike,
            right,
            candle_date,
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

    logger.info("Market data init start")
    dao = MarketDataDao()
    logger.info("DuckDB ready")
    client = BreezeClient(load_credentials())
    logger.info("Breeze session ready")
    logger.info("Market data init complete")
    return _MarketDataService(client, dao)
