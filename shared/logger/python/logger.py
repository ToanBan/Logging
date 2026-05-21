import logging
import sys
from typing import Any, Dict, Optional

import structlog


logging.basicConfig(
    format="%(message)s",
    stream=sys.stdout,
    level=logging.INFO,
)

structlog.configure(
    processors=[
        structlog.processors.TimeStamper(
            fmt="iso",
            key="timestamp",
            utc=True,
        ),
        structlog.processors.add_log_level,
        structlog.processors.JSONRenderer(),
    ],
    logger_factory=structlog.PrintLoggerFactory(),
    wrapper_class=structlog.make_filtering_bound_logger(logging.INFO),
)


class ServiceLogger:
    def __init__(self, service: str):
        self.service = service
        self.logger = structlog.get_logger()

    def _log(
        self,
        level: str,
        msg: str,
        ctx: Optional[Dict[str, Any]] = None,
    ):
        ctx = ctx or {}

        payload = {
            "service": self.service,
            "req_id": ctx.get("req_id"),
            "trace_id": ctx.get("trace_id"),
            "user_id": ctx.get("user_id"),
            "extra": ctx.get("extra", {}),
        }

        log_method = getattr(self.logger, level)

        log_method(
            msg,
            **payload,
        )

    def debug(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        self._log("debug", msg, ctx)

    def info(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        self._log("info", msg, ctx)

    def warn(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        self._log("warning", msg, ctx)

    def error(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        self._log("error", msg, ctx)

    def fatal(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        self._log("critical", msg, ctx)


def create_logger(service: str) -> ServiceLogger:
    return ServiceLogger(service)