"""Existing Breeze ingestion workflows."""

from .ingestion import fetch_underlying
from .option_chain import fetch_option_chain


class BreezeProvider:
    def fetch_underlying(self, underlying, start_date, end_date):
        return fetch_underlying(underlying, start_date, end_date)

    def fetch_option_chain(self, underlying, expiry):
        return fetch_option_chain(underlying, expiry)
