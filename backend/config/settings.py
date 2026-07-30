"""Strategy and runtime configuration."""

from __future__ import annotations

import os
from dataclasses import dataclass
from datetime import date
from pathlib import Path

from dotenv import load_dotenv

BACKTEST_ROOT = Path(__file__).resolve().parent.parent.parent
DATA_DIR = BACKTEST_ROOT / "data"
DB_PATH = DATA_DIR / "market_data.duckdb"
RESULTS_DIR = BACKTEST_ROOT / "results"

LOT_SIZE_CHANGE_DATE = date(2024, 11, 20)


def lot_size_for_date(d: date) -> int:
    return 75 if d >= LOT_SIZE_CHANGE_DATE else 50


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
