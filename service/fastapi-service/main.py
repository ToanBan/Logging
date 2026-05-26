import os
import uuid
import contextlib

import psycopg2
import psycopg2.extras

from fastapi import FastAPI, Request, Response, status, Query
from fastapi.responses import JSONResponse
from starlette.middleware.base import BaseHTTPMiddleware
from psycopg2.pool import ThreadedConnectionPool

from shared.logger.python.logger import create_logger, create_request_logger

LOG_MODE = os.getenv("LOG_MODE", "none").strip().lower()

logger = create_logger("fastapi-service")

DATABASE_URL = os.getenv(
    "DATABASE_URL",
    "postgresql://postgres:postgres@postgres:5432/logging_benchmark"
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
        req_id = request.headers.get("x-request-id", str(uuid.uuid4()))
        request.state.req_id = req_id

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

      

    except Exception as e:
        logger.error("database connection failed", {"extra": {"error": str(e)}})
        os._exit(1)


@app.get("/health")
def health(request: Request):
    if LOG_MODE == "structured":
        logger.info("health check", {"req_id": request.state.req_id})

    return {"ok": True, "service": "fastapi-service"}


@app.get("/messages")
def get_messages(
    request: Request,
    page: int = Query(1, ge=1),
    limit: int = Query(10, ge=1, le=100)
):
    req_id = request.state.req_id
    req_log = create_request_logger("fastapi-service", req_id) if LOG_MODE == "kafka" else logger

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

        return {"success": True, "pagination": {"page": page, "limit": limit}, "data": messages}

    except Exception as e:
        if LOG_MODE in ("structured", "selective", "kafka"):
            req_log.error("get messages error", {"req_id": req_id, "extra": {"error": str(e)}})

        return JSONResponse(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            content={"success": False, "error": str(e)}
        )


@app.get("/messages/room/{room_origin_id}")
def get_messages_by_room(
    request: Request,
    room_origin_id: str,
    page: int = Query(1, ge=1),
    limit: int = Query(10, ge=1, le=100)
):
    req_id = request.state.req_id
    req_log = create_request_logger("fastapi-service", req_id) if LOG_MODE == "kafka" else logger

    if LOG_MODE in ("structured", "kafka"):
        req_log.info("get messages by room request", {"req_id": req_id, "extra": {"room_origin_id": room_origin_id}})

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

            return JSONResponse(
                status_code=status.HTTP_404_NOT_FOUND,
                content={"success": False, "error": f"No messages found for room {room_origin_id}"}
            )

        for message in messages:
            if message.get("created_at"):
                message["created_at"] = message["created_at"].isoformat()
            if message.get("updated_at"):
                message["updated_at"] = message["updated_at"].isoformat()

        return {"success": True, "pagination": {"page": page, "limit": limit}, "data": messages}

    except Exception as e:
        if LOG_MODE in ("structured", "selective", "kafka"):
            req_log.error("get messages by room error", {"req_id": req_id, "extra": {"room_origin_id": room_origin_id, "error": str(e)}})

        return JSONResponse(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            content={"success": False, "error": str(e)}
        )