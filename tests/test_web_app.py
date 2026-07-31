from fastapi.testclient import TestClient
import pandas as pd

import backend.controllers.web_controller as web_controller
from backend.app import app


def test_strategy_pages_render():
    client = TestClient(app)

    response = client.get("/")
    assert response.status_code == 200
    assert "Backtests" in response.text
    assert "Custom Multi-Leg Strategy" in response.text
    assert "New Backtest" in response.text

    response = client.get("/strategies/custom_multi_leg")
    assert response.status_code == 200
    assert "Backtest Configuration" in response.text
    assert "Basic Settings" in response.text
    assert "Legs" in response.text
    assert "Strike Selection" in response.text
    assert "Offset" in response.text
    assert "Delta" in response.text
    assert "Premium" in response.text
    assert "Fixed Strike" in response.text
    assert "ATM" in response.text
    assert "<th>value</th>" in response.text
    assert "Risk-free Rate" not in response.text
    assert "Delta Scan Range" not in response.text
    assert "Total Stop Loss Multiplier" not in response.text
    assert "Calculate 5-Min MTM" in response.text
    assert "Filters" in response.text
    assert "Run Backtest" in response.text


def test_home_page_renders_rr_and_expectancy(monkeypatch):
    monkeypatch.setattr(
        web_controller,
        "list_runs",
        lambda: [
            {
                "run_id": "run-1",
                "strategy_name": "Custom Multi-Leg Strategy",
                "start_date": "2026-01-01",
                "end_date": "2026-02-28",
                "created_at": "2026-07-30T12:00:00",
                "total_pnl": 100.0,
                "win_rate": 50.0,
                "risk_reward_ratio": 2.0,
                "expectancy": 10.0,
                "traded_expiries": 2,
                "skipped_expiries": 0,
            }
        ],
    )

    client = TestClient(app)
    response = client.get("/")

    assert response.status_code == 200
    assert "<th>RR</th>" in response.text
    assert "<th>expectancy</th>" in response.text
    assert "<td>2.0</td>" in response.text
    assert "<td>10.0</td>" in response.text


def test_strategy_run_form_renders_metrics_link(monkeypatch):
    trades = pd.DataFrame(
        [
            {
                "trade_id": "trade-1",
                "expiry_date": "2026-01-27",
                "entry_date": "2025-12-12",
                "exit_date": "2026-01-27",
                "leg_role": "short_call",
            },
            {
                "trade_id": "trade-1",
                "expiry_date": "2026-01-27",
                "entry_date": "2025-12-12",
                "exit_date": "2026-01-27",
                "leg_role": "short_put",
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
                "risk_reward_ratio": 2.0,
                "expectancy": 50.0,
                "max_drawdown": 0.0,
                "best_expiry_pnl": 100.0,
                "worst_expiry_pnl": 100.0,
                "average_pnl_per_expiry": 100.0,
                "traded_expiries": 1,
                "skipped_expiries": 0,
            },
            "equity_curve": [{"date": "2026-01-27", "equity": 100.0}],
            "expiry_pnl_curve": [{"date": "2026-01-27", "pnl": 100.0}],
            "average_mtm_by_expiry": [{"date": "2026-01-27", "average_mtm_pct_of_premium": 25.0}],
            "trade_metrics": [
                {
                    "trade_id": "trade-1",
                    "expiry_date": "2026-01-27",
                    "exit_date": "2026-01-20",
                    "exit_reason": "scheduled_exit",
                    "premium_received": 100.0,
                    "maxMtm": 50.0,
                    "minMtm": -10.0,
                    "mtmVolatilityPctOfPremium": 7.35,
                }
            ],
            "vix_curve": [{"date": "2026-01-27", "vix": 14.2}],
        }

    monkeypatch.setattr(web_controller, "run_backtest_for_strategy", fake_run_backtest_for_strategy)
    monkeypatch.setattr(web_controller, "load_run", fake_load_run)

    client = TestClient(app)
    response = client.post(
        "/strategies/custom_multi_leg/run",
        data={
            "start_date": "2026-01-01",
            "end_date": "2026-02-28",
            "leg_role": ["short_call", "short_put"],
            "option_type": ["call", "put"],
            "side": ["sell", "sell"],
            "quantity": ["", ""],
            "strike_selection": ["offset", "offset"],
            "strike_value": ["300", "300"],
            "entry_dte": ["45", "45"],
            "entry_time": ["09:30", "09:30"],
            "exit_dte": ["0", "0"],
            "exit_time": ["15:30", "15:30"],
        },
    )

    assert response.status_code == 200
    assert "Backtest Results" in response.text
    assert "Summary" in response.text
    assert "Regime Analysis" in response.text
    assert "Trades" in response.text
    assert "Analytics" not in response.text
    assert "Drawdown chart placeholder" not in response.text
    assert "averageMtmByExpiry" in response.text
    assert "Average MTM (% of Premium) by Expiry" in response.text
    assert "maxMtm" in response.text
    assert "minMtm" in response.text
    assert "India VIX" in response.text
    assert "vixCurve" in response.text
    assert "calendarDateAxis" in response.text
    assert "expiryPnl" in response.text
    assert '<canvas id="expiry-pnl-chart"' in response.text
    assert "Expiry P&amp;L" in response.text or "Expiry P&L" in response.text
    assert "chart.umd.min.js" in response.text
    assert '<canvas id="regime-equity-chart"' in response.text
    assert '<canvas id="regime-average-mtm-chart"' in response.text
    assert '<canvas id="regime-vix-chart"' in response.text
    assert "Total PnL" in response.text
    assert "RR" in response.text
    assert "Expectancy" in response.text
    assert "/backtests/run-1/trades/trade-1/mtm" in response.text
    assert 'rowspan="2" class="merged-trade-id"' in response.text
    assert response.text.index('href="#summary"') < response.text.index('href="#regime-analysis"')
    assert response.text.index('href="#regime-analysis"') < response.text.index('href="#trades"')

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
    assert "Trade Information" in response.text
    assert "MTM Curve" in response.text
    assert "mtm-chart" in response.text
    assert "chart.umd.min.js" in response.text
    assert '<canvas id="mtm-chart"' in response.text
    assert "5-Min MTM Rows" in response.text


def test_placeholder_pages_render():
    client = TestClient(app)

    assert client.get("/compare").status_code == 200
    assert "Compare Backtests" in client.get("/compare").text
    assert client.get("/settings").status_code == 200
    assert "Settings" in client.get("/settings").text
