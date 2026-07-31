"""File-backed result persistence."""

from __future__ import annotations

import json
from pathlib import Path

import pandas as pd

from backend.config.settings import RESULTS_DIR
from backend.services.metrics_service import build_average_mtm_by_expiry


def save_run(run_id: str, metadata: dict, results: dict) -> Path:
    run_dir = RESULTS_DIR / run_id
    run_dir.mkdir(parents=True, exist_ok=True)
    for name in ("trades", "skipped_expiries", "daily_mtm"):
        df = results.get(name, pd.DataFrame())
        if not df.empty:
            df.to_csv(run_dir / f"{name}.csv", index=False)
    (run_dir / "metrics.json").write_text(
        json.dumps(results.get("metrics", {}), indent=2),
        encoding="utf-8",
    )
    (run_dir / "equity_curve.json").write_text(
        json.dumps(results.get("equity_curve", []), indent=2),
        encoding="utf-8",
    )
    (run_dir / "expiry_pnl_curve.json").write_text(
        json.dumps(results.get("expiry_pnl_curve", []), indent=2),
        encoding="utf-8",
    )
    (run_dir / "average_mtm_by_expiry.json").write_text(
        json.dumps(results.get("average_mtm_by_expiry", []), indent=2),
        encoding="utf-8",
    )
    (run_dir / "trade_metrics.json").write_text(
        json.dumps(results.get("trade_metrics", []), indent=2),
        encoding="utf-8",
    )
    (run_dir / "vix_curve.json").write_text(
        json.dumps(results.get("vix_curve", []), indent=2),
        encoding="utf-8",
    )
    (run_dir / "metadata.json").write_text(json.dumps(metadata, indent=2), encoding="utf-8")
    return run_dir


def load_run(run_id: str) -> dict:
    run_dir = _run_dir(run_id)
    metadata = _read_metadata(run_dir)
    trades = _read_csv(run_dir / "trades.csv")
    daily_mtm = _read_csv(run_dir / "daily_mtm.csv")
    average_mtm_by_expiry = _read_json(run_dir / "average_mtm_by_expiry.json", default=[])
    if _average_mtm_needs_rebuild(average_mtm_by_expiry):
        average_mtm_by_expiry = build_average_mtm_by_expiry(trades=trades, daily_mtm=daily_mtm)
    return {
        "metadata": metadata,
        "trades": trades,
        "skipped_expiries": _read_csv(run_dir / "skipped_expiries.csv"),
        "daily_mtm": daily_mtm,
        "metrics": _read_json(run_dir / "metrics.json"),
        "equity_curve": _read_json(run_dir / "equity_curve.json", default=[]),
        "expiry_pnl_curve": _read_json(run_dir / "expiry_pnl_curve.json", default=[]),
        "average_mtm_by_expiry": average_mtm_by_expiry,
        "trade_metrics": _read_json(run_dir / "trade_metrics.json", default=[]),
        "vix_curve": _read_json(run_dir / "vix_curve.json", default=[]),
    }


def list_runs() -> list[dict]:
    if not RESULTS_DIR.exists():
        return []
    runs = []
    for run_dir in RESULTS_DIR.iterdir():
        if not run_dir.is_dir():
            continue
        metadata = _read_metadata(run_dir)
        metrics = _read_json(run_dir / "metrics.json")
        runs.append(
            {
                "run_id": metadata.get("run_id", run_dir.name),
                "strategy_name": metadata.get("strategy_name", metadata.get("strategy_id", "")),
                "start_date": metadata.get("start_date", ""),
                "end_date": metadata.get("end_date", ""),
                "created_at": metadata.get("created_at", ""),
                "total_pnl": metrics.get("total_pnl", ""),
                "win_rate": metrics.get("win_rate", ""),
                "risk_reward_ratio": metrics.get("risk_reward_ratio", ""),
                "expectancy": metrics.get("expectancy", ""),
                "traded_expiries": metrics.get("traded_expiries", ""),
                "skipped_expiries": metrics.get("skipped_expiries", ""),
            }
        )
    return sorted(runs, key=lambda row: row.get("created_at") or row.get("run_id"), reverse=True)


def load_trade_mtm(run_id: str, trade_id: str) -> dict:
    run = load_run(run_id)
    trades = run["trades"]
    daily_mtm = run["daily_mtm"]
    trade_rows = trades[trades["trade_id"] == trade_id] if not trades.empty else pd.DataFrame()
    mtm_rows = daily_mtm[daily_mtm["trade_id"] == trade_id] if not daily_mtm.empty else pd.DataFrame()
    return {
        "metadata": run["metadata"],
        "trade_id": trade_id,
        "trades": trade_rows,
        "daily_mtm": mtm_rows,
    }


def dataframe_records(df: pd.DataFrame) -> list[dict]:
    if df.empty:
        return []
    return df.where(pd.notna(df), "").to_dict(orient="records")


def dataframe_columns(df: pd.DataFrame) -> list[str]:
    return list(df.columns)


def _read_metadata(run_dir: Path) -> dict:
    path = run_dir / "metadata.json"
    if not path.exists():
        return {"run_id": run_dir.name}
    return json.loads(path.read_text(encoding="utf-8"))


def _read_csv(path: Path) -> pd.DataFrame:
    return pd.read_csv(path) if path.exists() else pd.DataFrame()


def _read_json(path: Path, default=None):
    if path.exists():
        return json.loads(path.read_text(encoding="utf-8"))
    return {} if default is None else default


def _average_mtm_needs_rebuild(rows) -> bool:
    if not rows:
        return True
    return not all("average_mtm_pct_of_premium" in row for row in rows if isinstance(row, dict))


def _run_dir(run_id: str) -> Path:
    run_dir = RESULTS_DIR / run_id
    if not run_dir.exists() or not run_dir.is_dir():
        raise FileNotFoundError(f"Backtest run not found: {run_id}")
    return run_dir
