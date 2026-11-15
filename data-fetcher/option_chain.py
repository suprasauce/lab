"""Synchronous, atomic historical option-chain ingestion."""

import logging
from datetime import datetime, time, timedelta

import database
from trading_symbol import generate_option_trading_symbol
from breeze_client import MARKET_ZONE, get_client
from ingestion import BATCH_SIZE, MAX_DTE, calendar_date, fetch_candles, stock_code

logger = logging.getLogger(__name__)
# Fixed exchange strike conventions in paise; no database metadata is available.
STRIKE_INTERVALS = {"NIFTY": 50 * 100, "BANKNIFTY": 100 * 100}
STRIKE_PADDING = 40


def fetch_option_chain(underlying, expiry):
    symbol = stock_code(underlying)
    expiry_date = calendar_date(expiry)
    with database.connect() as connection:
        underlying_id = database.get_underlying_id(connection, underlying)
        expiry_id = database.get_option_expiry_id(connection, underlying_id, expiry_date)
        if database.is_option_chain_fetched(connection, expiry_id):
            return {"status": "already_exists", "expiry_id": expiry_id, "contracts": 0, "fetched": 0, "inserted": 0}
    if expiry_date >= datetime.now(MARKET_ZONE).date():
        raise ValueError("Option chain expiry must be before today so historical data can be complete")

    start_date = expiry_date - timedelta(days=MAX_DTE)
    start = datetime.combine(start_date, time.min, MARKET_ZONE)
    end = datetime.combine(expiry_date + timedelta(days=1), time.min, MARKET_ZONE)
    with database.connect() as connection:
        stored = database.get_underlying_candles(connection, underlying_id, start, end)

    client = get_client()
    # There is no underlying coverage flag. Re-request the full range to avoid
    # mistaking partial stored data for a complete historical high/low range.
    underlying_rows = fetch_candles(start_date, expiry_date,
        lambda first, last: client.historical_underlying(symbol, first, last))
    by_time = {row[0]: row for row in underlying_rows}
    by_time.update({row[0]: row for row in stored})  # Preserve existing DB values.
    if not by_time:
        raise RuntimeError("No underlying candles available for the complete 45-DTE window")
    low = min(row[4] for row in by_time.values())
    high = max(row[3] for row in by_time.values())
    interval = STRIKE_INTERVALS[underlying]
    lower = max(interval, low // interval * interval - STRIKE_PADDING * interval)
    upper = (high + interval - 1) // interval * interval + STRIKE_PADDING * interval

    contracts = []
    for strike in range(lower, upper + 1, interval):
        for option_type in ("CALL", "PUT"):
            try:
                rows = fetch_candles(start_date, expiry_date,
                    lambda first, last: client.historical_option(symbol, expiry_date, strike, option_type, first, last))
            except RuntimeError as exc:
                raise RuntimeError(
                    f"Option chain {underlying} expiry={expiry_date} strike_paise={strike} "
                    f"option_type={option_type}: {exc}"
                ) from exc
            # Successful empty responses are valid. The full history was checked.
            contracts.append((strike, option_type, rows))

    inserted = underlying_inserted = 0
    with database.connect() as connection:
        with connection.transaction():
            has_symbol = database.has_option_trading_symbol(connection)
            for offset in range(0, len(underlying_rows), BATCH_SIZE):
                underlying_inserted += database.insert_underlying_candles(connection,
                    [(underlying_id, *row) for row in underlying_rows[offset:offset + BATCH_SIZE]])
            for strike, option_type, rows in contracts:
                trading_symbol = generate_option_trading_symbol(underlying, expiry_date, strike, option_type)
                contract_id = database.insert_option_contract(connection, underlying_id, expiry_id, strike, option_type,
                                                               trading_symbol if has_symbol else None)
                if contract_id is None:
                    contract_id = database.get_option_contract_id(connection, underlying_id, expiry_id, strike, option_type)
                if contract_id is None:
                    raise RuntimeError("Conflicting contract has an inconsistent underlying relationship")
                for offset in range(0, len(rows), BATCH_SIZE):
                    inserted += database.insert_option_candles(connection,
                        [(contract_id, *row) for row in rows[offset:offset + BATCH_SIZE]])
            database.mark_option_chain_fetched(connection, expiry_id)
    fetched = sum(len(rows) for _, _, rows in contracts)
    logger.info("option_chain underlying=%s expiry=%s contracts=%d fetched=%d inserted=%d underlying_inserted=%d",
                underlying, expiry, len(contracts), fetched, inserted, underlying_inserted)
    return {"status": "ingested", "expiry_id": expiry_id, "contracts": len(contracts),
            "fetched": fetched, "inserted": inserted, "underlying_inserted": underlying_inserted}
