"""Local parquet cache for 5-minute candles."""

from __future__ import annotations

from datetime import date
from pathlib import Path

import pandas as pd

from settings import DATA_DIR

NIFTY_CACHE = DATA_DIR / "5min" / "nifty"
OPTIONS_CACHE = DATA_DIR / "5min" / "options"


def _nifty_path(d: date) -> Path:
    return NIFTY_CACHE / f"{d.isoformat()}.parquet"


def _option_path(expiry: date, strike: int, right: str, d: date) -> Path:
    key = f"{expiry.strftime('%Y%m%d')}_{strike}_{right.lower()}"
    return OPTIONS_CACHE / key / f"{d.isoformat()}.parquet"


def has_nifty_day(d: date) -> bool:
    return _nifty_path(d).exists()


def has_option_day(expiry: date, strike: int, right: str, d: date) -> bool:
    return _option_path(expiry, strike, right, d).exists()


def load_nifty_day(d: date) -> pd.DataFrame:
    path = _nifty_path(d)
    if not path.exists():
        return pd.DataFrame()
    return pd.read_parquet(path)


def load_option_day(expiry: date, strike: int, right: str, d: date) -> pd.DataFrame:
    path = _option_path(expiry, strike, right, d)
    if not path.exists():
        return pd.DataFrame()
    return pd.read_parquet(path)


def save_nifty_day(d: date, df: pd.DataFrame) -> None:
    path = _nifty_path(d)
    path.parent.mkdir(parents=True, exist_ok=True)
    df.to_parquet(path, index=False)


def save_option_day(expiry: date, strike: int, right: str, d: date, df: pd.DataFrame) -> None:
    path = _option_path(expiry, strike, right, d)
    path.parent.mkdir(parents=True, exist_ok=True)
    df.to_parquet(path, index=False)
