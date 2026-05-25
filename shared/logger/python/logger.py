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


def _get_kafka_producer():
    global _kafka_producer
    if _kafka_producer is None:
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
        producer.flush()
    except Exception as e:
        sys.stderr.write(f"[kafka-flush-error] {e}\n")


BUFFER_SIZE = 64


class RingBuffer:
    def __init__(self):
        self.buffer: List[dict] = []

    def push(self, entry: dict):
        if len(self.buffer) >= BUFFER_SIZE:
            self.buffer.pop(0)
        self.buffer.append(entry)

    def flush(self) -> List[dict]:
        return list(self.buffer)

    def clear(self):
        self.buffer = []


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
        pass


class RequestLogger:
    def __init__(self, service: str, req_id: str):
        self.service = service
        self.req_id = req_id
        self.ring = RingBuffer()

    def _log(self, level: str, msg: str, ctx: Optional[Dict[str, Any]] = None):
        ctx = ctx or {}
        ctx["req_id"] = self.req_id
        self.ring.push(_build_entry(self.service, level, msg, ctx))

    def debug(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        self._log("debug", msg, ctx)

    def info(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        self._log("info", msg, ctx)

    def warn(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        self._log("warn", msg, ctx)
        _flush_to_kafka(self.ring.flush())
        self.ring.clear()

    def error(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        self._log("error", msg, ctx)
        _flush_to_kafka(self.ring.flush())
        self.ring.clear()

    def fatal(self, msg: str, ctx: Optional[Dict[str, Any]] = None):
        self._log("fatal", msg, ctx)
        _flush_to_kafka(self.ring.flush())
        self.ring.clear()

    def done(self):
        self.ring.clear()


def create_logger(service: str) -> ServiceLogger:
    return ServiceLogger(service)


def create_request_logger(service: str, req_id: str) -> RequestLogger:
    return RequestLogger(service, req_id)