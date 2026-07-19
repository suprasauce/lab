"""File-backed result persistence."""

from __future__ import annotations

import json
from pathlib import Path

import pandas as pd

from backend.config.settings import RESULTS_DIR


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
    (run_dir / "metadata.json").write_text(json.dumps(metadata, indent=2), encoding="utf-8")
    return run_dir


def load_run(run_id: str) -> dict:
    run_dir = _run_dir(run_id)
    metadata = _read_metadata(run_dir)
    return {
        "metadata": metadata,
        "trades": _read_csv(run_dir / "trades.csv"),
        "skipped_expiries": _read_csv(run_dir / "skipped_expiries.csv"),
        "daily_mtm": _read_csv(run_dir / "daily_mtm.csv"),
        "metrics": _read_json(run_dir / "metrics.json"),
        "equity_curve": _read_json(run_dir / "equity_curve.json", default=[]),
    }


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


def _run_dir(run_id: str) -> Path:
    run_dir = RESULTS_DIR / run_id
    if not run_dir.exists() or not run_dir.is_dir():
        raise FileNotFoundError(f"Backtest run not found: {run_id}")
    return run_dir
