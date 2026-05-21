import os
import uuid
import time
import contextlib

import structlog
import psycopg2
import psycopg2.extras

from fastapi import FastAPI, Request, Response, status, Query
from fastapi.responses import JSONResponse
from starlette.middleware.base import BaseHTTPMiddleware
from psycopg2.pool import ThreadedConnectionPool

from shared.logger.python import create_logger


LOG_MODE = os.getenv("LOG_MODE", "structured").strip().lower()

logger = create_logger("fastapi-service")


DATABASE_URL = os.getenv(
    "DATABASE_URL",
    "postgresql://postgres:postgres@postgres:5432/logging_benchmark"
)

db_pool = ThreadedConnectionPool(
    1,
    50,
    dsn=DATABASE_URL
)


@contextlib.contextmanager
def get_db_cursor():
    conn = db_pool.getconn()

    try:
        with conn.cursor(
            cursor_factory=psycopg2.extras.RealDictCursor
        ) as cur:
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
            return await call_next(request)

        req_id = request.headers.get(
            "x-request-id",
            str(uuid.uuid4())
        )

        trace_id = request.headers.get(
            "x-trace-id",
            str(uuid.uuid4())
        )

        structlog.contextvars.clear_contextvars()

        structlog.contextvars.bind_contextvars(
            req_id=req_id,
            trace_id=trace_id,
            method=request.method,
            path=request.url.path,
        )

        response: Response = await call_next(request)

        response.headers["x-request-id"] = req_id
        response.headers["x-trace-id"] = trace_id

        return response


app = FastAPI()

app.add_middleware(LoggingMiddleware)


@app.on_event("startup")
def startup_event():
    try:
        with get_db_cursor() as cur:
            cur.execute("SELECT 1")
            cur.fetchone()

        logger.info(
            "database_connection_success"
        )

    except Exception as e:
        logger.error(
            "database_connection_failed",
            {
                "extra": {
                    "error": str(e)
                }
            }
        )

        os._exit(1)


@app.get("/health")
def health():
    logger.info(
        "health_check",
        {
            "extra": {
                "path": "/health"
            }
        }
    )

    return {
        "ok": True,
        "service": "fastapi-service"
    }


@app.get("/messages")
def get_messages(
    page: int = Query(1, ge=1),
    limit: int = Query(10, ge=1, le=100)
):
    t_start = time.perf_counter()

    logger.info(
        "get_messages_request",
        {
            "extra": {
                "page": page,
                "limit": limit
            }
        }
    )

    offset = (page - 1) * limit

    t_param_done = time.perf_counter()

    try:
        with get_db_cursor() as cur:
            cur.execute(
                """
                SELECT *
                FROM chat_message
                ORDER BY created_at DESC
                LIMIT %s OFFSET %s
                """,
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
            "fastapi_perf_get_all",
            {
                "extra": {
                    "perf_type": "FASTAPI_PERF_GET_ALL",
                    "mode": LOG_MODE.upper(),
                    "parse_param_ms": round(p_params, 3),
                    "db_query_ms": round(p_db, 3),
                    "total_ms": round(p_total, 3),
                    "page": page,
                    "limit": limit,
                }
            }
        )

        return {
            "success": True,
            "pagination": {
                "page": page,
                "limit": limit
            },
            "data": messages
        }

    except Exception as e:
        logger.error(
            "get_messages_error",
            {
                "extra": {
                    "error": str(e),
                    "page": page,
                    "limit": limit,
                }
            }
        )

        return JSONResponse(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            content={
                "success": False,
                "error": str(e)
            }
        )


@app.get("/messages/room/{room_origin_id}")
def get_messages_by_room(
    room_origin_id: str,
    page: int = Query(1, ge=1),
    limit: int = Query(10, ge=1, le=100)
):
    t_start = time.perf_counter()

    logger.info(
        "get_messages_by_room_request",
        {
            "extra": {
                "room_origin_id": room_origin_id,
                "page": page,
                "limit": limit,
            }
        }
    )

    offset = (page - 1) * limit

    t_param_done = time.perf_counter()

    try:
        with get_db_cursor() as cur:
            cur.execute(
                """
                SELECT *
                FROM chat_message
                WHERE room_origin_id = %s
                ORDER BY created_at DESC
                LIMIT %s OFFSET %s
                """,
                (room_origin_id, limit, offset)
            )

            messages = cur.fetchall()

        p_params = (t_param_done - t_start) * 1000
        p_db = (time.perf_counter() - t_param_done) * 1000

        if not messages:
            p_total = (time.perf_counter() - t_start) * 1000

            logger.warn(
                "get_messages_by_room_not_found",
                {
                    "extra": {
                        "room_origin_id": room_origin_id,
                        "parse_param_ms": round(p_params, 3),
                        "db_query_ms": round(p_db, 3),
                        "total_ms": round(p_total, 3),
                    }
                }
            )

            return JSONResponse(
                status_code=status.HTTP_404_NOT_FOUND,
                content={
                    "success": False,
                    "error": f"No messages found for room {room_origin_id}"
                }
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
            "fastapi_perf_by_room",
            {
                "extra": {
                    "perf_type": "FASTAPI_PERF_BY_ROOM",
                    "room_origin_id": room_origin_id,
                    "mode": LOG_MODE.upper(),
                    "parse_param_ms": round(p_params, 3),
                    "db_query_ms": round(p_db, 3),
                    "total_ms": round(p_total, 3),
                    "page": page,
                    "limit": limit,
                }
            }
        )

        return {
            "success": True,
            "pagination": {
                "page": page,
                "limit": limit
            },
            "data": messages
        }

    except Exception as e:
        logger.error(
            "get_messages_by_room_error",
            {
                "extra": {
                    "room_origin_id": room_origin_id,
                    "error": str(e)
                }
            }
        )

        return JSONResponse(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            content={
                "success": False,
                "error": str(e)
            }
        )