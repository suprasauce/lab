"""Independent command for option contracts and historical candles."""

import argparse
import logging
import os
from pathlib import Path

from dotenv import dotenv_values

from pipelines.options import ingest_options


def main():
    values = dotenv_values(Path(__file__).with_name(".env"))
    for name in ("BREEZE_API_KEY", "BREEZE_API_SECRET", "BREEZE_SESSION_TOKEN"):
        if values.get(name) is not None:
            os.environ.setdefault(name, values[name])
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    parser = argparse.ArgumentParser(description="Ingest one expiry's option contracts and 5-minute candles")
    parser.add_argument("--underlying", required=True)
    parser.add_argument("--expiry", required=True)
    parser.add_argument("--start-date", required=True)
    parser.add_argument("--end-date", required=True)
    parser.add_argument("--strike-interval-paise", type=int, required=True, help="Strike interval in paise, e.g. 5000 for INR 50")
    parser.add_argument("--strike-padding", type=int, required=True, help="Number of extra strike intervals on each side, e.g. 20")
    args = parser.parse_args()
    try:
        ingest_options(args.underlying, args.expiry, args.start_date, args.end_date, args.strike_interval_paise, args.strike_padding)
    except (ValueError, RuntimeError) as exc:
        message = str(exc)
        for name in ("BREEZE_API_KEY", "BREEZE_API_SECRET", "BREEZE_SESSION_TOKEN", "DATABASE_URL"):
            if os.environ.get(name):
                message = message.replace(os.environ[name], "[redacted]")
        logging.error("%s", message)
        return 1
    except Exception as exc:
        logging.error("Options ingestion failed (%s); verify database configuration", type(exc).__name__)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
