from fastapi.testclient import TestClient
import pandas as pd

import backend.controllers.web_controller as web_controller
from backend.app import app


def test_strategy_pages_render():
    client = TestClient(app)

    response = client.get("/")
    assert response.status_code == 200
    assert "Nifty Short Strangle" in response.text

    response = client.get("/strategies/nifty_short_strangle")
    assert response.status_code == 200
    assert "Run Backtest" in response.text


def test_strategy_run_form_renders_metrics_link(monkeypatch):
    trades = pd.DataFrame(
        [
            {
                "trade_id": "trade-1",
                "expiry_date": "2026-01-27",
                "entry_date": "2025-12-12",
                "exit_date": "2026-01-27",
            }
        ]
    )
    results = {"trades": trades, "skipped_expiries": pd.DataFrame(), "daily_mtm": pd.DataFrame()}

    def fake_run_backtest_for_strategy(**kwargs):
        return "run-1", results

    def fake_load_run(run_id):
        return {
            "metadata": {
                "run_id": run_id,
                "start_date": "2026-01-01",
                "end_date": "2026-02-28",
                "trade_rows": 1,
                "skipped_expiries": 0,
                "daily_mtm_rows": 0,
            },
            "trades": trades,
            "skipped_expiries": pd.DataFrame(),
            "daily_mtm": pd.DataFrame(),
            "metrics": {
                "total_pnl": 100.0,
                "win_rate": 100.0,
                "max_drawdown": 0.0,
                "best_expiry_pnl": 100.0,
                "worst_expiry_pnl": 100.0,
                "average_pnl_per_expiry": 100.0,
                "traded_expiries": 1,
                "skipped_expiries": 0,
            },
            "equity_curve": [{"date": "2026-01-27", "equity": 100.0}],
            "trade_metrics": [
                {
                    "trade_id": "trade-1",
                    "expiry_date": "2026-01-27",
                    "premium_received": 100.0,
                    "maxMtm": 50.0,
                    "minMtm": -10.0,
                    "mtmVolatilityPctOfPremium": 7.35,
                }
            ],
        }

    monkeypatch.setattr(web_controller, "run_backtest_for_strategy", fake_run_backtest_for_strategy)
    monkeypatch.setattr(web_controller, "load_run", fake_load_run)

    client = TestClient(app)
    response = client.post(
        "/strategies/nifty_short_strangle/run",
        data={
            "start_date": "2026-01-01",
            "end_date": "2026-02-28",
            "entry_dte": "45",
            "entry_time": "09:30",
            "exit_time": "15:30",
            "strike_offset": "6",
            "lot_size": "",
        },
    )

    assert response.status_code == 200
    assert "Metrics" in response.text
    assert "Equity Curve" in response.text
    assert "Trade MTM Volatility" in response.text
    assert "mtmVolatilityPctOfPremium" in response.text
    assert "maxMtm" in response.text
    assert "minMtm" in response.text
    assert "Total PnL" in response.text
    assert 'href="/backtests/run-1/trades" target="_blank"' in response.text
    assert 'href="/backtests/run-1/skipped-expiries" target="_blank"' in response.text
    assert "/backtests/run-1/trades/trade-1/mtm" in response.text

    response = client.get("/backtests/run-1/trades")
    assert response.status_code == 200
    assert "trade-1" in response.text
    assert "/backtests/run-1/trades/trade-1/mtm" in response.text


def test_trade_mtm_page_renders_curve(monkeypatch):
    def fake_load_trade_mtm(run_id, trade_id):
        return {
            "metadata": {
                "run_id": run_id,
                "start_date": "2026-01-01",
                "end_date": "2026-02-28",
            },
            "trades": pd.DataFrame([{"trade_id": trade_id, "leg_role": "short_call"}]),
            "daily_mtm": pd.DataFrame(
                [
                    {"trade_id": trade_id, "mtm_date": "2026-01-01", "mtm": 10.0},
                    {"trade_id": trade_id, "mtm_date": "2026-01-02", "mtm": -5.0},
                ]
            ),
        }

    monkeypatch.setattr(web_controller, "load_trade_mtm", fake_load_trade_mtm)

    client = TestClient(app)
    response = client.get("/backtests/run-1/trades/trade-1/mtm")

    assert response.status_code == 200
    assert "MTM Curve" in response.text
    assert "mtm-chart" in response.text
    assert "Daily MTM Rows" in response.text
