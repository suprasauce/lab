"""Backtest engine."""

from __future__ import annotations

import logging

import pandas as pd

from common.nse_calendar import iter_monthly_expiries

logger = logging.getLogger(__name__)


class BacktestEngine:
    def __init__(self, data):
        self.data = data

    def run(self, strategy, config) -> dict[str, pd.DataFrame]:
        trades: list[pd.DataFrame] = []
        skipped_expiries: list[pd.DataFrame] = []

        for expiry in iter_monthly_expiries(config.start_date, config.end_date):
            logger.info("Processing expiry %s", expiry)
            result = strategy.run(config, expiry, self.data)
            trades.append(result["trades"])
            skipped_expiries.append(result["skipped_expiries"])

        return {
            "trades": _concat(trades),
            "skipped_expiries": _concat(skipped_expiries),
        }


def _concat(frames: list[pd.DataFrame]) -> pd.DataFrame:
    non_empty = [frame for frame in frames if not frame.empty]
    return pd.concat(non_empty, ignore_index=True) if non_empty else pd.DataFrame()
