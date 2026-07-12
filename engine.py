"""Backtest engine."""

from __future__ import annotations

import logging
from datetime import date

from nse_calendar import (
    calendar_dte_at_entry,
    entry_date_for_expiry,
    iter_monthly_expiries,
)
from settings import StrategyConfig
from data_fetcher import DataFetcher
from data_store import load_nifty_day, load_option_day
from strategy import SkippedTrade, Trade
from strike_selector import select_strikes
from utils import get_price_at_time

logger = logging.getLogger(__name__)


class BacktestEngine:
    def __init__(self, config: StrategyConfig, fetcher: DataFetcher):
        self.config = config
        self.fetcher = fetcher

    def run(self) -> tuple[list[Trade], list[SkippedTrade]]:
        trades: list[Trade] = []
        skipped: list[SkippedTrade] = []
        expiries = iter_monthly_expiries(self.config.start_date, self.config.end_date)

        for expiry in expiries:
            entry = entry_date_for_expiry(expiry, self.config.entry_dte)
            if entry >= expiry:
                skipped.append(
                    SkippedTrade(
                        expiry_date=expiry,
                        entry_date=entry,
                        actual_dte=None,
                        ce_strike=None,
                        pe_strike=None,
                        skip_reason="entry_on_or_after_expiry",
                    )
                )
                continue

            actual_dte = calendar_dte_at_entry(expiry, entry)
            logger.info("Processing expiry %s entry %s (DTE %d)", expiry, entry, actual_dte)

            nifty_entry_df = self.fetcher.ensure_nifty_day(entry)
            spot = get_price_at_time(nifty_entry_df, self.config.entry_time)
            if spot is None:
                skipped.append(
                    SkippedTrade(
                        expiry_date=expiry,
                        entry_date=entry,
                        actual_dte=actual_dte,
                        ce_strike=None,
                        pe_strike=None,
                        skip_reason="spot_no_price",
                    )
                )
                continue

            sel = select_strikes(spot, self.config.strike_offset)
            self.fetcher.fetch_hold_period(entry, expiry, sel.ce_strike, sel.pe_strike)

            ce_entry_df = load_option_day(entry, expiry, sel.ce_strike, "call")
            pe_entry_df = load_option_day(entry, expiry, sel.pe_strike, "put")
            ce_entry = get_price_at_time(ce_entry_df, self.config.entry_time)
            pe_entry = get_price_at_time(pe_entry_df, self.config.entry_time)

            if ce_entry is None and pe_entry is None:
                skip_reason = "both_no_price"
            elif ce_entry is None:
                skip_reason = "ce_no_price"
            elif pe_entry is None:
                skip_reason = "pe_no_price"
            else:
                skip_reason = ""

            if skip_reason:
                skipped.append(
                    SkippedTrade(
                        expiry_date=expiry,
                        entry_date=entry,
                        actual_dte=actual_dte,
                        ce_strike=sel.ce_strike,
                        pe_strike=sel.pe_strike,
                        skip_reason=skip_reason,
                    )
                )
                continue

            ce_exit_df = load_option_day(expiry, expiry, sel.ce_strike, "call")
            pe_exit_df = load_option_day(expiry, expiry, sel.pe_strike, "put")
            ce_exit = get_price_at_time(ce_exit_df, self.config.exit_time)
            pe_exit = get_price_at_time(pe_exit_df, self.config.exit_time)

            if ce_exit is None or pe_exit is None:
                skipped.append(
                    SkippedTrade(
                        expiry_date=expiry,
                        entry_date=entry,
                        actual_dte=actual_dte,
                        ce_strike=sel.ce_strike,
                        pe_strike=sel.pe_strike,
                        skip_reason="exit_no_price",
                    )
                )
                continue

            entry_premium = ce_entry + pe_entry
            exit_cost = ce_exit + pe_exit
            lot = self.config.resolve_lot_size(entry)
            slippage = self.config.slippage_per_leg * 4  # entry+exit × 2 legs
            pnl = (entry_premium - exit_cost) * lot - slippage - self.config.brokerage_per_trade

            trades.append(
                Trade(
                    expiry_date=expiry,
                    entry_date=entry,
                    exit_date=expiry,
                    actual_dte=actual_dte,
                    spot_at_entry=round(spot, 2),
                    atm_strike=sel.atm_strike,
                    ce_strike=sel.ce_strike,
                    pe_strike=sel.pe_strike,
                    ce_entry=round(ce_entry, 2),
                    pe_entry=round(pe_entry, 2),
                    entry_premium=round(entry_premium, 2),
                    ce_exit=round(ce_exit, 2),
                    pe_exit=round(pe_exit, 2),
                    exit_cost=round(exit_cost, 2),
                    pnl=round(pnl, 2),
                    lot_size=lot,
                    entry_dte=self.config.entry_dte,
                    strike_offset=self.config.strike_offset,
                    entry_time=self.config.entry_time.strftime("%H:%M"),
                    exit_time=self.config.exit_time.strftime("%H:%M"),
                )
            )

        return trades, skipped
