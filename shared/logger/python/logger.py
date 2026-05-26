import json
import os
import sys
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional


LOGS_DIRECTORY = "/shared/logger/logs"
COMBINED_LOG_PATH = f"{LOGS_DIRECTORY}/combined.log"

os.makedirs(LOGS_DIRECTORY, exist_ok=True)

_log_file = open(COMBINED_LOG_PATH, "a", buffering=1)


def _build_entry(service: str, level: str, msg: str, ctx: Optional[Dict[str, Any]] = None) -> dict:
    ctx = ctx or {}
    entry: dict = {
        "timestamp": datetime.now(timezone.utc).isoformat(),
        "level": level,
        "service": service,
        "msg": msg,
    }
    if ctx.get("req_id"):
        entry["req_id"] = ctx["req_id"]
    if ctx.get("trace_id"):
        entry["trace_id"] = ctx["trace_id"]
    if ctx.get("user_id"):
        entry["user_id"] = ctx["user_id"]
    if ctx.get("extra"):
        entry["extra"] = ctx["extra"]
    return entry


def _write(entry: dict):
    _log_file.write(json.dumps(entry) + "\n")
    _log_file.flush()


_kafka_producer = None
_kafka_producer_init = False


def _get_kafka_producer():
    global _kafka_producer, _kafka_producer_init
    if _kafka_producer_init:
        return _kafka_producer
    _kafka_producer_init = True

    from kafka import KafkaProducer, KafkaAdminClient
    from kafka.admin import NewTopic

    broker = os.getenv("KAFKA_BROKER", "kafka:9092")

    try:
        admin = KafkaAdminClient(bootstrap_servers=[broker])
        admin.create_topics([
            NewTopic(name="req-logs", num_partitions=1, replication_factor=1)
        ])
        admin.close()
    except Exception:
        pass

    _kafka_producer = KafkaProducer(
        bootstrap_servers=[broker],
        value_serializer=lambda v: json.dumps(v).encode("utf-8"),
    )
    return _kafka_producer


def _flush_to_kafka(entries: List[dict]):
    try:
        producer = _get_kafka_producer()
        producer.send("req-logs", entries)
        # không flush() — fire and forget giống Node
    except Exception as e:
        # fallback: ghi ra file nếu kafka fail
        for entry in entries:
            _write(entry)
        sys.stderr.write(f"[kafka-flush-error] {e}\n")


class ServiceLogger:
    def __init__(self, service: str):
        self.service = service

    def debug(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        _write(_build_entry(self.service, "debug", msg, ctx))

    def info(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        _write(_build_entry(self.service, "info", msg, ctx))

    def warn(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        _write(_build_entry(self.service, "warn", msg, ctx))

    def error(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        _write(_build_entry(self.service, "error", msg, ctx))

    def fatal(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        _write(_build_entry(self.service, "fatal", msg, ctx))

    def done(self):
        pass  # no-op để interface thống nhất


class RequestLogger:
    def __init__(self, service: str, req_id: str):
        self.service = service
        self.req_id = req_id

    def _log(self, level: str, msg: str, ctx: Optional[Dict[str, Any]] = None):
        ctx = ctx or {}
        ctx["req_id"] = self.req_id
        # gửi thẳng Kafka, không buffer
        _flush_to_kafka([_build_entry(self.service, level, msg, ctx)])

    def debug(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        self._log("debug", msg, ctx)

    def info(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        self._log("info", msg, ctx)

    def warn(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        self._log("warn", msg, ctx)

    def error(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        self._log("error", msg, ctx)

    def fatal(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        self._log("fatal", msg, ctx)

    def done(self):
        pass  # no-op


def create_logger(service: str) -> ServiceLogger:
    return ServiceLogger(service)


def create_request_logger(service: str, req_id: str) -> RequestLogger:
    return RequestLogger(service, req_id)