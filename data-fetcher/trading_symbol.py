"""Lab-specific symbols: UNDERLYING-YYYYMMDD-RUPEE_STRIKE-CE/PE."""

import re
from datetime import date


def generate_option_trading_symbol(underlying, expiry, strike_paise, option_type):
    underlying = underlying.upper()
    if not re.fullmatch(r"[A-Z][A-Z0-9]*", underlying):
        raise ValueError("Invalid underlying for trading symbol")
    if not isinstance(expiry, date):
        raise ValueError("Expiry must be a calendar date")
    if isinstance(strike_paise, bool) or not isinstance(strike_paise, int) or not 0 < strike_paise <= 2**63 - 1:
        raise ValueError("Strike must be positive integer paise fitting BIGINT")
    if option_type not in ("CALL", "PUT"):
        raise ValueError("Option type must be CALL or PUT")
    strike = str(strike_paise // 100)
    suffix = "CE" if option_type == "CALL" else "PE"
    expiry_text = f"{expiry.year:04d}{expiry.month:02d}{expiry.day:02d}"
    return f"{underlying}-{expiry_text}-{strike}-{suffix}"
