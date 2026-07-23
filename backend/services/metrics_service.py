"""Backtest metrics derived from saved result rows."""

from __future__ import annotations

from typing import Any

import numpy as np
import pandas as pd


def build_backtest_metrics(
    *,
    trades: pd.DataFrame,
    skipped_expiries: pd.DataFrame,
    daily_mtm: pd.DataFrame,
) -> dict[str, Any]:
    expiry_pnl = _expiry_pnl(trades)
    mtm_curve = _daily_portfolio_mtm(daily_mtm)
    skip_counts = _skip_counts(skipped_expiries)

    traded_expiries = int(expiry_pnl["expiry_date"].nunique()) if not expiry_pnl.empty else 0
    skipped_count = int(len(skipped_expiries))
    total_expiries = traded_expiries + skipped_count
    winning_expiries = int((expiry_pnl["pnl"] > 0).sum()) if not expiry_pnl.empty else 0
    losing_expiries = int((expiry_pnl["pnl"] < 0).sum()) if not expiry_pnl.empty else 0
    total_pnl = _round(expiry_pnl["pnl"].sum()) if not expiry_pnl.empty else 0.0

    return {
        "total_expiries": total_expiries,
        "traded_expiries": traded_expiries,
        "skipped_expiries": skipped_count,
        "total_trades": int(trades["trade_id"].nunique()) if not trades.empty else 0,
        "total_pnl": total_pnl,
        "average_pnl_per_expiry": _round(total_pnl / traded_expiries) if traded_expiries else 0.0,
        "best_expiry_pnl": _round(expiry_pnl["pnl"].max()) if not expiry_pnl.empty else 0.0,
        "worst_expiry_pnl": _round(expiry_pnl["pnl"].min()) if not expiry_pnl.empty else 0.0,
        "winning_expiries": winning_expiries,
        "losing_expiries": losing_expiries,
        "win_rate": _round((winning_expiries / traded_expiries) * 100) if traded_expiries else 0.0,
        "max_drawdown": _max_drawdown(mtm_curve),
        "max_profit_seen": _round(mtm_curve["mtm"].max()) if not mtm_curve.empty else 0.0,
        "max_loss_seen": _round(mtm_curve["mtm"].min()) if not mtm_curve.empty else 0.0,
        "ending_mtm": _round(mtm_curve["mtm"].iloc[-1]) if not mtm_curve.empty else 0.0,
        "best_mtm_day": _day_for_extreme(mtm_curve, "max"),
        "worst_mtm_day": _day_for_extreme(mtm_curve, "min"),
        "most_common_skip_reason": _most_common_skip_reason(skip_counts),
        "skip_reason_counts": skip_counts,
    }


def build_equity_curve(trades: pd.DataFrame) -> list[dict[str, Any]]:
    realized = _realized_pnl_by_exit_date(trades)
    return _equity_curve(realized)


def build_trade_metrics(*, trades: pd.DataFrame, daily_mtm: pd.DataFrame) -> list[dict[str, Any]]:
    if trades.empty:
        return []

    premiums = _premium_by_trade(trades)
    trade_mtm = _trade_daily_mtm(daily_mtm)
    rows = []
    for _, trade in _trade_summary(trades).iterrows():
        trade_id = str(trade["trade_id"])
        premium = premiums.get(trade_id)
        mtm_rows = trade_mtm[trade_mtm["trade_id"] == trade_id]
        rows.append(
            {
                "trade_id": trade_id,
                "expiry_date": str(trade["expiry_date"]),
                "premium_received": None if premium is None else _round(premium),
                "maxMtm": _max_mtm(mtm_rows),
                "minMtm": _min_mtm(mtm_rows),
                "mtmVolatilityPctOfPremium": _mtm_volatility_pct_of_premium(
                    mtm_rows=mtm_rows,
                    premium_received=premium,
                ),
            }
        )
    return rows


def metric_cards(metrics: dict[str, Any]) -> list[dict[str, Any]]:
    return [
        {"label": "Total PnL", "value": metrics.get("total_pnl", 0.0)},
        {"label": "Win Rate", "value": f"{metrics.get('win_rate', 0.0)}%"},
        {"label": "Max Drawdown", "value": metrics.get("max_drawdown", 0.0)},
        {"label": "Best Expiry", "value": metrics.get("best_expiry_pnl", 0.0)},
        {"label": "Worst Expiry", "value": metrics.get("worst_expiry_pnl", 0.0)},
        {"label": "Avg PnL / Expiry", "value": metrics.get("average_pnl_per_expiry", 0.0)},
        {"label": "Traded Expiries", "value": metrics.get("traded_expiries", 0)},
        {"label": "Skipped Expiries", "value": metrics.get("skipped_expiries", 0)},
        {"label": "Most Common Skip", "value": metrics.get("most_common_skip_reason") or "none"},
    ]


def _expiry_pnl(trades: pd.DataFrame) -> pd.DataFrame:
    if trades.empty:
        return pd.DataFrame(columns=["trade_id", "expiry_date", "pnl"])
    rows = trades.copy()
    rows["leg_pnl"] = (
        pd.to_numeric(rows["entry_price"], errors="coerce")
        - pd.to_numeric(rows["exit_price"], errors="coerce")
    ) * pd.to_numeric(rows["lot_size"], errors="coerce")
    return (
        rows.groupby(["trade_id", "expiry_date"], as_index=False)["leg_pnl"]
        .sum()
        .rename(columns={"leg_pnl": "pnl"})
    )


def _premium_by_trade(trades: pd.DataFrame) -> dict[str, float]:
    if trades.empty:
        return {}
    rows = trades.copy()
    rows["premium"] = pd.to_numeric(rows["entry_price"], errors="coerce") * pd.to_numeric(
        rows["lot_size"], errors="coerce"
    )
    premium = rows.groupby("trade_id")["premium"].sum()
    return {str(trade_id): float(value) for trade_id, value in premium.items()}


def _trade_summary(trades: pd.DataFrame) -> pd.DataFrame:
    return (
        trades[["trade_id", "expiry_date"]]
        .drop_duplicates()
        .sort_values(["expiry_date", "trade_id"])
    )


def _trade_daily_mtm(daily_mtm: pd.DataFrame) -> pd.DataFrame:
    if daily_mtm.empty:
        return pd.DataFrame(columns=["trade_id", "mtm_date", "mtm"])
    rows = daily_mtm.copy()
    rows["mtm"] = pd.to_numeric(rows["mtm"], errors="coerce").fillna(0.0)
    return (
        rows.groupby(["trade_id", "mtm_date"], as_index=False)["mtm"]
        .sum()
        .sort_values(["trade_id", "mtm_date"])
    )


def _mtm_volatility_pct_of_premium(
    *,
    mtm_rows: pd.DataFrame,
    premium_received: float | None,
) -> float | None:
    if premium_received is None or pd.isna(premium_received) or abs(premium_received) == 0:
        return None
    if len(mtm_rows) < 2:
        return 0.0

    mtm_values = pd.to_numeric(
        mtm_rows.sort_values("mtm_date")["mtm"],
        errors="coerce",
    ).dropna()
    if len(mtm_values) < 2:
        return 0.0

    daily_changes = mtm_values.diff().dropna()
    normalized = (daily_changes / abs(float(premium_received))) * 100
    return _round(np.std(normalized.to_numpy(), ddof=0))


def _max_mtm(mtm_rows: pd.DataFrame) -> float | None:
    if mtm_rows.empty:
        return None
    return _round(pd.to_numeric(mtm_rows["mtm"], errors="coerce").max())


def _min_mtm(mtm_rows: pd.DataFrame) -> float | None:
    if mtm_rows.empty:
        return None
    return _round(pd.to_numeric(mtm_rows["mtm"], errors="coerce").min())


def _realized_pnl_by_exit_date(trades: pd.DataFrame) -> pd.DataFrame:
    if trades.empty:
        return pd.DataFrame(columns=["date", "pnl"])
    rows = trades.copy()
    rows["pnl"] = (
        pd.to_numeric(rows["entry_price"], errors="coerce")
        - pd.to_numeric(rows["exit_price"], errors="coerce")
    ) * pd.to_numeric(rows["lot_size"], errors="coerce")
    rows["date"] = rows["exit_date"].astype(str)
    return rows.groupby("date", as_index=False)["pnl"].sum().sort_values("date")


def _equity_curve(realized: pd.DataFrame) -> list[dict[str, Any]]:
    if realized.empty:
        return []
    rows = realized.copy()
    rows["equity"] = rows["pnl"].cumsum()
    return [
        {"date": str(row["date"]), "equity": _round(row["equity"])}
        for _, row in rows.iterrows()
    ]


def _daily_portfolio_mtm(daily_mtm: pd.DataFrame) -> pd.DataFrame:
    if daily_mtm.empty:
        return pd.DataFrame(columns=["mtm_date", "mtm"])
    rows = daily_mtm.copy()
    rows["mtm"] = pd.to_numeric(rows["mtm"], errors="coerce").fillna(0.0)
    return rows.groupby("mtm_date", as_index=False)["mtm"].sum().sort_values("mtm_date")


def _skip_counts(skipped_expiries: pd.DataFrame) -> dict[str, int]:
    if skipped_expiries.empty or "reason" not in skipped_expiries.columns:
        return {}
    counts = skipped_expiries["reason"].fillna("unknown").value_counts()
    return {str(reason): int(count) for reason, count in counts.items()}


def _most_common_skip_reason(skip_counts: dict[str, int]) -> str | None:
    if not skip_counts:
        return None
    return max(skip_counts.items(), key=lambda item: item[1])[0]


def _max_drawdown(mtm_curve: pd.DataFrame) -> float:
    if mtm_curve.empty:
        return 0.0
    running_peak = mtm_curve["mtm"].cummax()
    drawdown = mtm_curve["mtm"] - running_peak
    return _round(drawdown.min())


def _day_for_extreme(mtm_curve: pd.DataFrame, fn: str) -> str | None:
    if mtm_curve.empty:
        return None
    idx = mtm_curve["mtm"].idxmax() if fn == "max" else mtm_curve["mtm"].idxmin()
    return str(mtm_curve.loc[idx, "mtm_date"])


def _round(value) -> float:
    return round(float(value), 2)
