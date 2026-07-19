"""Current strategy: monthly Nifty short strangle."""

from __future__ import annotations

from datetime import date

import pandas as pd

from backend.common.nse_calendar import entry_date_for_expiry
from backend.common.strike_selector import select_strikes
from backend.services import market_data_service


class ShortStrangleStrategy:
    name = "nifty_short_strangle"

    def run(self, config, expiry: date) -> dict[str, pd.DataFrame]:
        entry_date = entry_date_for_expiry(expiry, config.entry_dte)
        exit_date = expiry
        base = _base_row(config, expiry, entry_date, exit_date)

        if entry_date >= expiry:
            return _skipped(base, "entry_on_or_after_expiry")

        spot_candle = market_data_service.get_underlying_candle(
            symbol="NIFTY",
            exchange="NSE",
            candle_date=entry_date,
            candle_time=config.entry_time,
        )
        spot = _close(spot_candle)
        if spot is None:
            return _skipped(base, "spot_no_price")

        selection = select_strikes(spot, config.strike_offset)
        base.update(
            {
                "spot_at_entry": round(spot, 2),
                "atm_strike": selection.atm_strike,
            }
        )

        ce_entry = _close(
            market_data_service.get_option_candle(
                symbol="NIFTY",
                exchange="NFO",
                expiry=expiry,
                strike=selection.ce_strike,
                right="call",
                candle_date=entry_date,
                candle_time=config.entry_time,
            )
        )
        pe_entry = _close(
            market_data_service.get_option_candle(
                symbol="NIFTY",
                exchange="NFO",
                expiry=expiry,
                strike=selection.pe_strike,
                right="put",
                candle_date=entry_date,
                candle_time=config.entry_time,
            )
        )
        ce_exit = _close(
            market_data_service.get_option_candle(
                symbol="NIFTY",
                exchange="NFO",
                expiry=expiry,
                strike=selection.ce_strike,
                right="call",
                candle_date=exit_date,
                candle_time=config.exit_time,
            )
        )
        pe_exit = _close(
            market_data_service.get_option_candle(
                symbol="NIFTY",
                exchange="NFO",
                expiry=expiry,
                strike=selection.pe_strike,
                right="put",
                candle_date=exit_date,
                candle_time=config.exit_time,
            )
        )

        if ce_entry is None and pe_entry is None:
            return _skipped(base, "both_entry_prices_missing")
        if ce_entry is None:
            return _skipped(base, "call_entry_price_missing")
        if pe_entry is None:
            return _skipped(base, "put_entry_price_missing")
        if ce_exit is None and pe_exit is None:
            return _skipped(base, "both_exit_prices_missing")
        if ce_exit is None:
            return _skipped(base, "call_exit_price_missing")
        if pe_exit is None:
            return _skipped(base, "put_exit_price_missing")

        lot = config.resolve_lot_size(entry_date)

        rows = [
            _leg_row(
                base,
                leg_role="short_call",
                option_type="call",
                strike=selection.ce_strike,
                entry_price=ce_entry,
                exit_price=ce_exit,
                lot_size=lot,
            ),
            _leg_row(
                base,
                leg_role="short_put",
                option_type="put",
                strike=selection.pe_strike,
                entry_price=pe_entry,
                exit_price=pe_exit,
                lot_size=lot,
            ),
        ]
        return {
            "trades": pd.DataFrame(rows, columns=_trade_columns()),
            "skipped_expiries": pd.DataFrame(columns=_skipped_columns()),
        }


def _close(candle: dict | None) -> float | None:
    if candle is None or candle.get("close") is None:
        return None
    close = float(candle["close"])
    return close if close > 0 else None


def _base_row(config, expiry: date, entry_date: date, exit_date: date) -> dict:
    return {
        "trade_id": f"{ShortStrangleStrategy.name}_{expiry.isoformat()}",
        "expiry_date": expiry.isoformat(),
        "entry_date": entry_date.isoformat(),
        "exit_date": exit_date.isoformat(),
        "spot_at_entry": None,
        "atm_strike": None,
        "strike_offset": config.strike_offset,
        "entry_time": config.entry_time.strftime("%H:%M"),
        "exit_time": config.exit_time.strftime("%H:%M"),
    }


def _leg_row(
    base: dict,
    *,
    leg_role: str,
    option_type: str,
    strike: int,
    entry_price: float,
    exit_price: float,
    lot_size: int,
) -> dict:
    row = base.copy()
    row.update(
        {
            "leg_role": leg_role,
            "option_type": option_type,
            "strike": strike,
            "entry_price": round(entry_price, 2),
            "exit_price": round(exit_price, 2),
            "lot_size": lot_size,
        }
    )
    return row


def _skipped(base: dict, reason: str) -> dict[str, pd.DataFrame]:
    row = {col: base.get(col) for col in _skipped_columns()}
    row["reason"] = reason
    return {
        "trades": pd.DataFrame(columns=_trade_columns()),
        "skipped_expiries": pd.DataFrame([row], columns=_skipped_columns()),
    }


def _trade_columns() -> list[str]:
    return [
        "trade_id",
        "expiry_date",
        "entry_date",
        "exit_date",
        "leg_role",
        "option_type",
        "strike",
        "entry_price",
        "exit_price",
        "lot_size",
        "spot_at_entry",
        "atm_strike",
        "strike_offset",
        "entry_time",
        "exit_time",
    ]


def _skipped_columns() -> list[str]:
    return [
        "trade_id",
        "expiry_date",
        "entry_date",
        "exit_date",
        "reason",
        "spot_at_entry",
        "atm_strike",
        "strike_offset",
        "entry_time",
        "exit_time",
    ]
