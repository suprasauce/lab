"""Local underlying-data ingestion command."""

import argparse
import logging
import os
from pathlib import Path

from dotenv import dotenv_values


def main():
    # Only Breeze credentials may come from .env; DATABASE_URL is OS-only.
    values = dotenv_values(Path(__file__).with_name(".env"))
    for name in ("BREEZE_API_KEY", "BREEZE_API_SECRET", "BREEZE_SESSION_TOKEN"):
        if values.get(name) is not None:
            os.environ.setdefault(name, values[name])
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    parser = argparse.ArgumentParser(description="Fetch underlying candles from Breeze")
    parser.add_argument("--underlying", default="NIFTY")
    parser.add_argument("--start-date", default="2026-09-01")
    parser.add_argument("--end-date", default="2026-09-30")
    parser.add_argument("--timeframe", default="5minute", choices=("1minute", "5minute", "1day"))
    args = parser.parse_args()
    from pipelines.underlying import fetch_underlying

    try:
        fetch_underlying(args.underlying, args.start_date, args.end_date, args.timeframe)
    except (ValueError, RuntimeError) as exc:
        message = str(exc)
        for name in ("BREEZE_API_KEY", "BREEZE_API_SECRET", "BREEZE_SESSION_TOKEN", "DATABASE_URL"):
            value = os.environ.get(name)
            if value:
                message = message.replace(value, "[redacted]")
        logging.error("Underlying ingestion failed: %s", message)
        return 1
    except Exception as exc:
        # SDK/driver exceptions can include secrets. Keep raw exception text out
        # of console logs while allowing direct callers to inspect raised errors.
        logging.error("Underlying ingestion failed (%s); verify API session, input, and database configuration", type(exc).__name__)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
