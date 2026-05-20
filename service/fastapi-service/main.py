import os
import math
import uuid
import contextlib
import structlog
from fastapi import FastAPI, Request, Response, status, Query
from fastapi.responses import JSONResponse
from starlette.middleware.base import BaseHTTPMiddleware
import psycopg2
import psycopg2.extras

LOG_MODE = os.getenv("LOG_MODE", "none").strip().lower()  # none | structured | selective

# Cấu hình structured logging
processors = [
    structlog.contextvars.merge_contextvars,
    structlog.processors.add_log_level,
    structlog.processors.TimeStamper(fmt="iso"),
]

if LOG_MODE == "none":
    import logging
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

from psycopg2.pool import ThreadedConnectionPool

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

class LoggingMiddleware(BaseHTTPMiddleware):
    async def dispatch(self, request: Request, call_next):
        if LOG_MODE == "none":
            return await call_next(request)  # ← bỏ qua hết, không làm gì

        req_id = request.headers.get("x-request-id", str(uuid.uuid4()))
        structlog.contextvars.clear_contextvars()
        structlog.contextvars.bind_contextvars(
            req_id=req_id,
            service="fastapi-service"
        )
        response: Response = await call_next(request)
        response.headers["x-request-id"] = req_id
        return response

app = FastAPI()
app.add_middleware(LoggingMiddleware)

@app.on_event("startup")
def startup_event():
    try:
        with get_db_cursor() as cur:
            cur.execute("SELECT 1")
            cur.fetchone()
        logger.info("Database connection test successful!")
    except Exception as e:
        logger.error("Database connection test failed", error=str(e))
        os._exit(1)

@app.get("/health")
def health():
    if LOG_MODE == "structured":
        logger.info("health_check")
    return {"ok": True, "service": "fastapi-service"}

@app.get("/messages")
def get_messages(
    page: int = Query(1, ge=1),
    limit: int = Query(10, ge=1, le=100),
    room_origin_id: str = Query(None)
):
    if LOG_MODE == "structured":
        logger.info("get_messages_request", page=page, limit=limit, room_origin_id=room_origin_id)

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

        return {
            "success": True,
            "pagination": { "page": page, "limit": limit },
            "data": messages
        }
    except Exception as e:
        if LOG_MODE in ("structured", "selective"):
            logger.error("get_messages_error", error=str(e))
        return JSONResponse(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            content={"success": False, "error": str(e)}
        )

@app.get("/messages/room/{room_origin_id}")
def get_messages_by_room(
    room_origin_id: str,
    page: int = Query(1, ge=1),
    limit: int = Query(10, ge=1, le=100)
):
    if LOG_MODE == "structured":
        logger.info("get_messages_by_room_request", room_origin_id=room_origin_id)

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
            return JSONResponse(
                status_code=status.HTTP_404_NOT_FOUND,
                content={"success": False, "error": f"No messages found for room {room_origin_id}"}
            )

        for message in messages:
            if message.get("created_at"):
                message["created_at"] = message["created_at"].isoformat()
            if message.get("updated_at"):
                message["updated_at"] = message["updated_at"].isoformat()

        return {
            "success": True,
            "pagination": {"page": page, "limit": limit},
            "data": messages
        }
    except Exception as e:
        if LOG_MODE in ("structured", "selective"):
            logger.error("get_messages_by_room_error", room_origin_id=room_origin_id, error=str(e))
        return JSONResponse(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            content={"success": False, "error": str(e)}
        )