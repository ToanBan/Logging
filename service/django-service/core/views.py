import os
import math
import uuid
import contextlib
import time
import structlog
from django.http import JsonResponse
import psycopg2
import psycopg2.extras

from psycopg2.pool import ThreadedConnectionPool

LOG_MODE = os.getenv("LOG_MODE", "none").strip().lower()  # none | structured | selective

processors = [
    structlog.contextvars.merge_contextvars,
    structlog.processors.add_log_level,
    structlog.processors.TimeStamper(fmt="iso"),
]

if LOG_MODE == "none":
    structlog.configure(
        processors=[structlog.processors.JSONRenderer()],
        logger_factory=structlog.WriteLoggerFactory(file=open(os.devnull, "w"))
    )
elif LOG_MODE == "selective":
    import logging
    structlog.configure(
        processors=processors + [structlog.processors.JSONRenderer()],
        context_class=dict,
        wrapper_class=structlog.make_filtering_bound_logger(logging.WARNING),
        logger_factory=structlog.PrintLoggerFactory(),
        cache_logger_on_first_use=True,
    )
else:
    structlog.configure(
        processors=processors + [structlog.processors.JSONRenderer()],
        context_class=dict,
        logger_factory=structlog.PrintLoggerFactory(),
        cache_logger_on_first_use=True,
    )

logger = structlog.get_logger()

DATABASE_URL = os.getenv(
    "DATABASE_URL",
    "postgresql://postgres:postgres@localhost:5433/logging_benchmark"
)

db_pool = ThreadedConnectionPool(1, 50, dsn=DATABASE_URL)

@contextlib.contextmanager
def get_db_cursor():
    conn = db_pool.getconn()
    try:
        with conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
            yield cur
        conn.commit()
    except Exception:
        conn.rollback()
        raise
    finally:
        db_pool.putconn(conn)

try:
    with get_db_cursor() as cur:
        cur.execute("SELECT 1")
        cur.fetchone()
    if LOG_MODE != "none":
        logger.info("Database connection test successful!")
except Exception as e:
    if LOG_MODE != "none":
        logger.error("Database connection test failed", error=str(e))
    os._exit(1)


def health(request):
    if LOG_MODE == "structured":
        logger.info("health_check")
    return JsonResponse({"ok": True, "service": "django-service"})


def get_messages(request):
    t_start = time.perf_counter()

    try:
        page = max(1, int(request.GET.get("page", 1)))
    except ValueError:
        page = 1

    try:
        limit = int(request.GET.get("limit", 10))
        limit = max(1, min(100, limit))
    except ValueError:
        limit = 10

    if LOG_MODE == "structured":
        logger.info("get_messages_request", page=page, limit=limit)

    offset = (page - 1) * limit

    t_param_done = time.perf_counter()

    try:
        with get_db_cursor() as cur:
            cur.execute(
                "SELECT * FROM chat_message ORDER BY created_at DESC LIMIT %s OFFSET %s",
                (limit, offset)
            )
            messages = cur.fetchall()

        for msg in messages:
            if msg.get("created_at"):
                msg["created_at"] = msg["created_at"].isoformat()
            if msg.get("updated_at"):
                msg["updated_at"] = msg["updated_at"].isoformat()

        t_db_done = time.perf_counter()

        p_params = (t_param_done - t_start) * 1000
        p_db = (t_db_done - t_param_done) * 1000
        p_total = (time.perf_counter() - t_start) * 1000

        logger.info(
            f"[DJANGO_PERF_GET_ALL] Mode: {LOG_MODE.upper()} | ParseParam: {p_params:.3f}ms | DB_Query: {p_db:.3f}ms | Total: {p_total:.3f}ms",
            perf_type="DJANGO_PERF_GET_ALL",
            mode=LOG_MODE.upper(),
            parse_param_ms=round(p_params, 3),
            db_query_ms=round(p_db, 3),
            total_ms=round(p_total, 3)
        )

        return JsonResponse({
            "success": True,
            "pagination": {"page": page, "limit": limit},
            "data": messages
        })

    except Exception as e:
        if LOG_MODE in ("structured", "selective"):
            logger.error("get_messages_error", error=str(e))
        return JsonResponse(
            {"success": False, "error": str(e)},
            status=500
        )


def get_messages_by_room(request, room_origin_id):
    t_start = time.perf_counter()

    if LOG_MODE == "structured":
        logger.info("get_messages_by_room_request", room_origin_id=room_origin_id)

    try:
        page = max(1, int(request.GET.get("page", 1)))
    except ValueError:
        page = 1

    try:
        limit = int(request.GET.get("limit", 10))
        limit = max(1, min(100, limit))
    except ValueError:
        limit = 10

    offset = (page - 1) * limit

    t_param_done = time.perf_counter()

    try:
        with get_db_cursor() as cur:
            cur.execute(
                "SELECT * FROM chat_message WHERE room_origin_id = %s ORDER BY created_at DESC LIMIT %s OFFSET %s",
                (room_origin_id, limit, offset)
            )
            messages = cur.fetchall()

        p_params = (t_param_done - t_start) * 1000
        p_db = (time.perf_counter() - t_param_done) * 1000

        if not messages:
            if LOG_MODE in ("structured", "selective"):
                logger.warning("get_messages_by_room_not_found", room_origin_id=room_origin_id)
            
            p_total = (time.perf_counter() - t_start) * 1000
            
            logger.info(
                f"[DJANGO_PERF_BY_ROOM_404] Mode: {LOG_MODE.upper()} | Room: {room_origin_id} | ParseParam: {p_params:.3f}ms | DB_Query: {p_db:.3f}ms | Total: {p_total:.3f}ms",
                perf_type="DJANGO_PERF_BY_ROOM_404",
                mode=LOG_MODE.upper(),
                room_id=room_origin_id,
                parse_param_ms=round(p_params, 3),
                db_query_ms=round(p_db, 3),
                total_ms=round(p_total, 3)
            )

            return JsonResponse(
                {"success": False, "error": f"No messages found for room {room_origin_id}"},
                status=404
            )

        for message in messages:
            if message.get("created_at"):
                message["created_at"] = message["created_at"].isoformat()
            if message.get("updated_at"):
                message["updated_at"] = message["updated_at"].isoformat()

        t_db_done = time.perf_counter()
        p_db = (t_db_done - t_param_done) * 1000
        p_total = (time.perf_counter() - t_start) * 1000

        logger.info(
            f"[DJANGO_PERF_BY_ROOM_200] Mode: {LOG_MODE.upper()} | Room: {room_origin_id} | ParseParam: {p_params:.3f}ms | DB_Query: {p_db:.3f}ms | Total: {p_total:.3f}ms",
            perf_type="DJANGO_PERF_BY_ROOM_200",
            mode=LOG_MODE.upper(),
            room_id=room_origin_id,
            parse_param_ms=round(p_params, 3),
            db_query_ms=round(p_db, 3),
            total_ms=round(p_total, 3)
        )

        return JsonResponse({
            "success": True,
            "pagination": {"page": page, "limit": limit},
            "data": messages
        })

    except Exception as e:
        if LOG_MODE in ("structured", "selective"):
            logger.error("get_messages_by_room_error", room_origin_id=room_origin_id, error=str(e))
        return JsonResponse(
            {"success": False, "error": str(e)},
            status=500
        )