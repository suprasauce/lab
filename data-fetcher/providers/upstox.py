"""Upstox ingestion using shared persistence and candle conventions."""

import logging
from datetime import datetime, time, timedelta
from decimal import InvalidOperation

import database
from candles import BATCH_SIZE, MARKET_ZONE, calendar_date, candle_row, to_paise
from trading_symbol import generate_option_trading_symbol
from .upstox_client import UpstoxClient

logger = logging.getLogger(__name__)
MAX_DTE = 45


class UpstoxProvider:
    def __init__(self):
        self.client = None

    def fetch_underlying(self, underlying, start_date, end_date):
        start, end = calendar_date(start_date), calendar_date(end_date)
        if start > end:
            raise ValueError("start_date must not be after end_date")
        with database.connect() as connection:
            underlying_id = database.get_underlying_id(connection, underlying)
        if self.client is None:
            self.client = UpstoxClient()
        instrument_key = self.client.resolve_underlying(underlying)

        ordered = self._fetch_candles(start, end,
            lambda first, last: self.client.historical_underlying(instrument_key, first, last))
        inserted = 0
        if ordered:
            with database.connect() as connection:
                with connection.transaction():
                    for offset in range(0, len(ordered), BATCH_SIZE):
                        inserted += database.insert_underlying_candles(connection,
                            [(underlying_id, *row) for row in ordered[offset:offset + BATCH_SIZE]])
        logger.info("underlying=%s fetched=%d inserted=%d", underlying, len(ordered), inserted)
        return {"status": "ingested", "fetched": len(ordered), "inserted": inserted}

    def _fetch_candles(self, start, end, request):
        rows = {}
        current = start
        while current <= end:
            # Both boundaries are inclusive: at most 30 calendar dates per request.
            last = min(current + timedelta(days=29), end)
            candles = request(current, last)
            for candle in candles:
                try:
                    row = candle_row(dict(zip(
                        ("datetime", "open", "high", "low", "close", "volume"), candle,
                    )))
                    if not datetime.combine(current, time.min, MARKET_ZONE) <= row[0] < datetime.combine(last + timedelta(days=1), time.min, MARKET_ZONE):
                        raise ValueError("Candle outside requested dates")
                except (KeyError, TypeError, ValueError, InvalidOperation) as exc:
                    raise RuntimeError("Upstox returned invalid candle data") from exc
                rows[row[0]] = row
            current = last + timedelta(days=1)

        return [rows[timestamp] for timestamp in sorted(rows)]

    def fetch_option_chain(self, underlying, expiry):
        expiry_date = calendar_date(expiry)
        with database.connect() as connection:
            underlying_id = database.get_underlying_id(connection, underlying)
            expiry_id = database.get_option_expiry_id(connection, underlying_id, expiry_date)
            if database.is_option_chain_fetched(connection, expiry_id):
                return {"status": "already_exists", "expiry_id": expiry_id, "contracts": 0, "fetched": 0, "inserted": 0}
        if expiry_date >= datetime.now(MARKET_ZONE).date():
            raise ValueError("Option chain expiry must be before today so historical data can be complete")
        if self.client is None:
            self.client = UpstoxClient()
        instrument_key = self.client.resolve_underlying(underlying)
        discovered = self.client.expired_option_contracts(instrument_key, expiry_date)
        if not discovered:
            raise RuntimeError(f"Upstox option chain not available for {underlying} expiry {expiry}")

        start = expiry_date - timedelta(days=MAX_DTE)
        underlying_rows = self._fetch_candles(start, expiry_date,
            lambda first, last: self.client.historical_underlying(instrument_key, first, last))
        contracts = []
        for contract in discovered:
            try:
                if contract["underlying_key"] != instrument_key or contract["expiry"] != expiry_date.isoformat():
                    raise ValueError("Contract does not match underlying and expiry")
                kind = {"CE": "CALL", "PE": "PUT"}[contract["instrument_type"]]
                strike = to_paise(contract["strike_price"])
                key = contract["instrument_key"]
            except (KeyError, TypeError, ValueError) as exc:
                raise RuntimeError("Upstox returned invalid option contract metadata") from exc
            try:
                rows = self._fetch_candles(start, expiry_date,
                    lambda first, last: self.client.historical_option(key, first, last))
            except RuntimeError as exc:
                raise RuntimeError(f"Option chain {underlying} expiry={expiry} strike_paise={strike} option_type={kind}: {exc}") from exc
            contracts.append((strike, kind, rows))

        inserted = underlying_inserted = 0
        with database.connect() as connection:
            with connection.transaction():
                has_symbol = database.has_option_trading_symbol(connection)
                for offset in range(0, len(underlying_rows), BATCH_SIZE):
                    underlying_inserted += database.insert_underlying_candles(connection,
                        [(underlying_id, *row) for row in underlying_rows[offset:offset + BATCH_SIZE]])
                for strike, kind, rows in contracts:
                    symbol = generate_option_trading_symbol(underlying, expiry_date, strike, kind)
                    contract_id = database.insert_option_contract(connection, underlying_id, expiry_id, strike, kind,
                                                                  symbol if has_symbol else None)
                    if contract_id is None:
                        contract_id = database.get_option_contract_id(connection, underlying_id, expiry_id, strike, kind)
                    if contract_id is None:
                        raise RuntimeError("Conflicting contract has an inconsistent underlying relationship")
                    for offset in range(0, len(rows), BATCH_SIZE):
                        inserted += database.insert_option_candles(connection,
                            [(contract_id, *row) for row in rows[offset:offset + BATCH_SIZE]])
                database.mark_option_chain_fetched(connection, expiry_id)
        fetched = sum(len(rows) for _, _, rows in contracts)
        logger.info("option_chain underlying=%s expiry=%s contracts=%d fetched=%d inserted=%d", underlying, expiry, len(contracts), fetched, inserted)
        return {"status": "ingested", "expiry_id": expiry_id, "contracts": len(contracts),
                "fetched": fetched, "inserted": inserted, "underlying_inserted": underlying_inserted}
