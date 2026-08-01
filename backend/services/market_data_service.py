"""Market data access: DuckDB first, Breeze only for missing data."""

from __future__ import annotations

import logging
import threading
from datetime import date, time, timedelta
from typing import TYPE_CHECKING

from backend.config.settings import load_credentials
from backend.dao.market_data_dao import MarketDataDao
from backend.common.utils import (
    date_session_breeze_range,
    day_session_breeze_range,
    expiry_to_breeze_iso,
    normalize_candle_df,
)

if TYPE_CHECKING:
    from backend.client.breeze_client import BreezeClient

logger = logging.getLogger(__name__)
_service: _MarketDataService | None = None
_SERVICE_LOCK = threading.Lock()


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
        logger.debug(
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
            logger.debug(
                "DuckDB hit underlying symbol=%s exchange=%s date=%s time=%s",
                symbol,
                exchange,
                candle_date,
                candle_time,
            )
            return candle

        logger.debug(
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
            logger.debug(
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
            logger.debug(
                "Resolved underlying from Breeze cache symbol=%s exchange=%s date=%s time=%s",
                symbol,
                exchange,
                candle_date,
                candle_time,
            )
        else:
            logger.debug(
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
            logger.debug(
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
        logger.debug(
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
            logger.debug(
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

        logger.debug(
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
            data_date=candle_date,
            expiry=expiry,
            strike=strike,
            right=right,
        ):
            logger.debug(
                "Missing marker hit option symbol=%s exchange=%s expiry=%s strike=%s right=%s date=%s",
                symbol,
                exchange,
                expiry,
                strike,
                right,
                candle_date,
            )
            return None

        self._fetch_option_range(
            symbol=symbol,
            exchange=exchange,
            expiry=expiry,
            strike=strike,
            right=right,
            start=candle_date,
            end=candle_date,
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
            logger.debug(
                "Resolved option from Breeze cache symbol=%s exchange=%s expiry=%s strike=%s right=%s date=%s time=%s",
                symbol,
                exchange,
                expiry,
                strike,
                right,
                candle_date,
                candle_time,
            )
            return candle

        logger.debug(
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
            data_date=candle_date,
            reason="requested_candle_missing_after_fetch",
            expiry=expiry,
            strike=strike,
            right=right,
        )
        return None

    def get_option_5m_range(
        self,
        *,
        symbol: str,
        exchange: str,
        expiry: date,
        strike: int,
        right: str,
        start: date,
        end: date,
    ):
        right = right.lower()
        logger.debug(
            "Market data request option range symbol=%s exchange=%s expiry=%s strike=%s right=%s start=%s end=%s",
            symbol,
            exchange,
            expiry,
            strike,
            right,
            start,
            end,
        )
        self._ensure_option_contract_range(
            symbol=symbol,
            exchange=exchange,
            expiry=expiry,
            strike=strike,
            right=right,
            start=start,
        )

        return self.dao.load_derivative_5m(
            underlying_symbol=symbol,
            exchange=exchange,
            instrument_type="option",
            expiry=expiry,
            strike=strike,
            right=right,
            start=start,
            end=end,
        )

    def _fetch_underlying_day(self, *, symbol: str, exchange: str, candle_date: date) -> None:
        logger.debug(
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
        logger.debug(
            "Breeze returned underlying rows=%s symbol=%s exchange=%s date=%s",
            len(df),
            symbol,
            exchange,
            candle_date,
        )
        if df.empty:
            logger.debug("No underlying data %s %s on %s", exchange, symbol, candle_date)
            self.dao.mark_day_missing(
                symbol=symbol,
                exchange=exchange,
                instrument_type="underlying",
                data_date=candle_date,
                reason="provider_no_rows",
            )
            logger.debug(
                "Marked day missing underlying symbol=%s exchange=%s date=%s reason=provider_no_rows",
                symbol,
                exchange,
                candle_date,
            )
            return
        self.dao.save_underlying_5m(symbol=symbol, exchange=exchange, df=df)
        logger.debug(
            "DuckDB saved underlying rows=%s symbol=%s exchange=%s date=%s",
            len(df),
            symbol,
            exchange,
            candle_date,
        )

    def _ensure_option_contract_range(
        self,
        *,
        symbol: str,
        exchange: str,
        expiry: date,
        strike: int,
        right: str,
        start: date,
    ) -> None:
        fetched_from = self.dao.option_contract_fetched_from(
            symbol=symbol,
            exchange=exchange,
            expiry=expiry,
            strike=strike,
            right=right,
        )
        if fetched_from is not None and fetched_from <= start:
            logger.debug(
                "Option contract coverage hit symbol=%s exchange=%s expiry=%s strike=%s right=%s fetched_from=%s requested_start=%s",
                symbol,
                exchange,
                expiry,
                strike,
                right,
                fetched_from,
                start,
            )
            return

        fetch_end = expiry if fetched_from is None else fetched_from - timedelta(days=1)
        if fetch_end < start:
            self.dao.mark_option_contract_fetched_from(
                symbol=symbol,
                exchange=exchange,
                expiry=expiry,
                strike=strike,
                right=right,
                fetched_from_date=start,
            )
            return

        for chunk_start, chunk_end in _date_chunks(start, fetch_end, days=14):
            self._fetch_option_range(
                symbol=symbol,
                exchange=exchange,
                expiry=expiry,
                strike=strike,
                right=right,
                start=chunk_start,
                end=chunk_end,
            )
        self.dao.mark_option_contract_fetched_from(
            symbol=symbol,
            exchange=exchange,
            expiry=expiry,
            strike=strike,
            right=right,
            fetched_from_date=start,
        )
        logger.debug(
            "Marked option contract coverage symbol=%s exchange=%s expiry=%s strike=%s right=%s fetched_from=%s",
            symbol,
            exchange,
            expiry,
            strike,
            right,
            start,
        )

    def _fetch_option_range(
        self,
        *,
        symbol: str,
        exchange: str,
        expiry: date,
        strike: int,
        right: str,
        start: date,
        end: date,
    ) -> None:
        logger.debug(
            "Breeze fetch option range start symbol=%s exchange=%s expiry=%s strike=%s right=%s start=%s end=%s",
            symbol,
            exchange,
            expiry,
            strike,
            right,
            start,
            end,
        )
        from_iso, to_iso = date_session_breeze_range(start, end)
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
        logger.debug(
            "Breeze returned option rows=%s symbol=%s exchange=%s expiry=%s strike=%s right=%s start=%s end=%s",
            len(df),
            symbol,
            exchange,
            expiry,
            strike,
            right,
            start,
            end,
        )
        if df.empty:
            logger.debug("No option data %s %s %s from %s to %s", expiry, strike, right, start, end)
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
        logger.debug(
            "DuckDB saved option rows=%s symbol=%s exchange=%s expiry=%s strike=%s right=%s start=%s end=%s",
            len(df),
            symbol,
            exchange,
            expiry,
            strike,
            right,
            start,
            end,
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


def get_option_5m_range(
    *,
    symbol: str,
    exchange: str,
    expiry: date,
    strike: int,
    right: str,
    start: date,
    end: date,
):
    return _get_market_data_service().get_option_5m_range(
        symbol=symbol,
        exchange=exchange,
        expiry=expiry,
        strike=strike,
        right=right,
        start=start,
        end=end,
    )


def _date_chunks(start: date, end: date, *, days: int) -> list[tuple[date, date]]:
    chunks = []
    cur = start
    while cur <= end:
        chunk_end = min(cur + timedelta(days=days - 1), end)
        chunks.append((cur, chunk_end))
        cur = chunk_end + timedelta(days=1)
    return chunks


def _get_market_data_service() -> _MarketDataService:
    global _service
    if _service is None:
        with _SERVICE_LOCK:
            if _service is None:
                _service = _create_market_data_service()
    return _service


def _create_market_data_service() -> _MarketDataService:
    from backend.client.breeze_client import BreezeClient

    logger.debug("Market data init start")
    dao = MarketDataDao()
    logger.debug("DuckDB ready")
    client = BreezeClient(load_credentials())
    logger.debug("Breeze session ready")
    logger.debug("Market data init complete")
    return _MarketDataService(client, dao)
