from datetime import date

import pandas as pd

from backtest.engine import BacktestEngine
from settings import StrategyConfig


class FakeStrategy:
    def __init__(self):
        self.calls = []

    def run(self, config, expiry, data):
        self.calls.append(expiry)
        return {
            "trades": pd.DataFrame([{"expiry_date": expiry.isoformat()}]),
            "skipped_expiries": pd.DataFrame(),
        }


def test_engine_runs_strategy_once_per_expiry():
    config = StrategyConfig(start_date=date(2026, 1, 1), end_date=date(2026, 2, 28))
    strategy = FakeStrategy()

    results = BacktestEngine(data=object()).run(strategy, config)

    assert len(results["trades"]) == 2
    assert results["trades"]["expiry_date"].tolist() == ["2026-01-27", "2026-02-24"]
    assert results["skipped_expiries"].empty
    assert strategy.calls == [date(2026, 1, 27), date(2026, 2, 24)]
