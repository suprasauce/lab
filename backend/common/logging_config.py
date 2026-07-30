"""Application logging setup."""

from __future__ import annotations

import logging

LOG_FORMAT = "%(asctime)s %(levelname)-5s [%(threadName)s] %(name)s - %(message)s"


def configure_logging(level: int = logging.INFO) -> None:
    root = logging.getLogger()
    root.setLevel(level)
    if not root.handlers:
        handler = logging.StreamHandler()
        handler.setFormatter(logging.Formatter(LOG_FORMAT))
        root.addHandler(handler)
    for handler in root.handlers:
        handler.setFormatter(logging.Formatter(LOG_FORMAT))
