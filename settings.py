"""Strategy and runtime configuration."""

from __future__ import annotations

import os
from dataclasses import dataclass, field
from datetime import date, time, timedelta
from pathlib import Path

from dotenv import load_dotenv

BACKTEST_ROOT = Path(__file__).resolve().parent
DATA_DIR = BACKTEST_ROOT / "data"
DB_PATH = DATA_DIR / "market_data.duckdb"
RESULTS_DIR = BACKTEST_ROOT / "results"

LOT_SIZE_CHANGE_DATE = date(2024, 11, 20)


def lot_size_for_date(d: date) -> int:
    return 75 if d >= LOT_SIZE_CHANGE_DATE else 50


@dataclass
class StrategyConfig:
    entry_dte: int = 45
    entry_time: time = time(9, 30)
    exit_time: time = time(15, 30)
    strike_offset: int = 6
    lot_size: int | None = None  # None = auto from entry date
    start_date: date = field(default_factory=lambda: date.today() - timedelta(days=365))
    end_date: date = field(default_factory=lambda: date.today())

    def resolve_lot_size(self, entry_date: date) -> int:
        return self.lot_size if self.lot_size is not None else lot_size_for_date(entry_date)


@dataclass
class BreezeCredentials:
    api_key: str
    api_secret: str
    session_token: str


def load_credentials() -> BreezeCredentials:
    load_dotenv(BACKTEST_ROOT / ".env")
    api_key = os.getenv("BREEZE_API_KEY", "")
    api_secret = os.getenv("BREEZE_API_SECRET", "")
    session_token = os.getenv("BREEZE_SESSION_TOKEN", "")
    if not all([api_key, api_secret, session_token]):
        raise ValueError(
            "Missing Breeze credentials. Copy .env.example to .env and fill in "
            "BREEZE_API_KEY, BREEZE_API_SECRET, BREEZE_SESSION_TOKEN."
        )
    return BreezeCredentials(api_key, api_secret, session_token)
