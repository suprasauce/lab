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


def test_strategy_run_form_renders_trade_rows(monkeypatch):
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
    results = {"trades": trades, "skipped_expiries": pd.DataFrame()}

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
            }
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
    assert "trade-1" in response.text
    assert "/backtests/run-1/trades/trade-1/mtm" in response.text
