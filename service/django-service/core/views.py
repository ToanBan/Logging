import os
import uuid
import contextlib

from django.http import JsonResponse, HttpRequest

import psycopg2
import psycopg2.extras
from psycopg2.pool import ThreadedConnectionPool

from shared.logger.python.logger import create_logger, create_request_logger

LOG_MODE = os.getenv("LOG_MODE", "none").strip().lower()

logger = create_logger("django-service")

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

except Exception as e:
    logger.error("database connection failed", {"extra": {"error": str(e)}})
    os._exit(1)


def get_req_id(request: HttpRequest) -> str:
    return request.headers.get("x-request-id", str(uuid.uuid4()))


def health(request: HttpRequest):
    if LOG_MODE == "structured":
        logger.info("health check", {"req_id": get_req_id(request)})

    return JsonResponse({"ok": True, "service": "django-service"})


def get_messages(request: HttpRequest):
    req_id = get_req_id(request)
    req_log = create_request_logger("django-service", req_id) if LOG_MODE == "kafka" else logger

    try:
        page = max(1, int(request.GET.get("page", 1)))
    except ValueError:
        page = 1

    try:
        limit = max(1, min(100, int(request.GET.get("limit", 10))))
    except ValueError:
        limit = 10

    if LOG_MODE in ("structured", "kafka"):
        req_log.info("get messages request", {"req_id": req_id, "extra": {"page": page, "limit": limit}})

    offset = (page - 1) * limit

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

        return JsonResponse({
            "success": True,
            "pagination": {"page": page, "limit": limit},
            "data": messages
        })

    except Exception as e:
        if LOG_MODE in ("structured", "selective", "kafka"):
            req_log.error("get messages error", {"req_id": req_id, "extra": {"error": str(e)}})

        return JsonResponse({"success": False, "error": str(e)}, status=500)


def get_messages_by_room(request: HttpRequest, room_origin_id: str):
    req_id = get_req_id(request)
    req_log = create_request_logger("django-service", req_id) if LOG_MODE == "kafka" else logger

    if LOG_MODE in ("structured", "kafka"):
        req_log.info("get messages by room request", {"req_id": req_id, "extra": {"room_origin_id": room_origin_id}})

    try:
        page = max(1, int(request.GET.get("page", 1)))
    except ValueError:
        page = 1

    try:
        limit = max(1, min(100, int(request.GET.get("limit", 10))))
    except ValueError:
        limit = 10

    offset = (page - 1) * limit

    try:
        with get_db_cursor() as cur:
            cur.execute(
                "SELECT * FROM chat_message WHERE room_origin_id = %s ORDER BY created_at DESC LIMIT %s OFFSET %s",
                (room_origin_id, limit, offset)
            )
            messages = cur.fetchall()

        if not messages:
            if LOG_MODE in ("structured", "selective", "kafka"):
                req_log.warn("room not found", {"req_id": req_id, "extra": {"room_origin_id": room_origin_id}})

            return JsonResponse(
                {"success": False, "error": f"No messages found for room {room_origin_id}"},
                status=404
            )

        for message in messages:
            if message.get("created_at"):
                message["created_at"] = message["created_at"].isoformat()
            if message.get("updated_at"):
                message["updated_at"] = message["updated_at"].isoformat()

        return JsonResponse({
            "success": True,
            "pagination": {"page": page, "limit": limit},
            "data": messages
        })

    except Exception as e:
        if LOG_MODE in ("structured", "selective", "kafka"):
            req_log.error("get messages by room error", {"req_id": req_id, "extra": {"room_origin_id": room_origin_id, "error": str(e)}})

        return JsonResponse({"success": False, "error": str(e)}, status=500)