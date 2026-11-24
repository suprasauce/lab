"""Upstox workflow placeholders; no API calls or database writes yet."""


class UpstoxProvider:
    def fetch_underlying(self, underlying, start_date, end_date):
        # TODO: Fetch and persist Upstox underlying candles.
        raise NotImplementedError("Upstox underlying ingestion is not implemented")

    def fetch_option_chain(self, underlying, expiry):
        # TODO: Discover Upstox contracts and ingest their historical candles.
        raise NotImplementedError("Upstox option-chain ingestion is not implemented")
