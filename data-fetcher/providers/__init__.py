"""Select a provider once; each provider owns both ingestion workflows."""


def create_provider(name):
    if name == "breeze":
        from .breeze import BreezeProvider
        return BreezeProvider()
    if name == "upstox":
        from .upstox import UpstoxProvider
        return UpstoxProvider()
    raise ValueError(f"Unsupported MARKET_DATA_PROVIDER {name!r}: expected breeze or upstox")
