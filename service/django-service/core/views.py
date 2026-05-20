import os
import contextlib
import structlog
from django.http import JsonResponse
import psycopg2
import psycopg2.extras

from psycopg2.pool import ThreadedConnectionPool

LOG_MODE = os.getenv("LOG_MODE", "none").strip().lower()

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

# Kiểm tra kết nối khi boot
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
    try:
        page = max(1, int(request.GET.get("page", 1)))
    except ValueError:
        page = 1

    try:
        limit = int(request.GET.get("limit", 10))
        limit = max(1, min(100, limit))
    except ValueError:
        limit = 10

    room_origin_id = request.GET.get("room_origin_id", None)

    if LOG_MODE == "structured":
        logger.info("get_messages_request", page=page, limit=limit)

    offset = (page - 1) * limit

    try:
        with get_db_cursor() as cur:
            if room_origin_id:
                cur.execute(
                    "SELECT * FROM chat_message WHERE room_origin_id = %s ORDER BY created_at DESC LIMIT %s OFFSET %s",
                    (room_origin_id, limit, offset)
                )
            else:
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
        if LOG_MODE in ("structured", "selective"):
            logger.error("get_messages_error", error=str(e))
        return JsonResponse(
            {"success": False, "error": str(e)},
            status=500
        )


def get_messages_by_room(request, room_origin_id):
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

    try:
        with get_db_cursor() as cur:
            cur.execute(
                "SELECT * FROM chat_message WHERE room_origin_id = %s ORDER BY created_at DESC LIMIT %s OFFSET %s",
                (room_origin_id, limit, offset)
            )
            messages = cur.fetchall()

        if not messages:
            if LOG_MODE in ("structured", "selective"):
                logger.warning("get_messages_by_room_not_found", room_origin_id=room_origin_id)
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
        if LOG_MODE in ("structured", "selective"):
            logger.error("get_messages_by_room_error", room_origin_id=room_origin_id, error=str(e))
        return JsonResponse(
            {"success": False, "error": str(e)},
            status=500
        )