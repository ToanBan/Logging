import os
import uuid
import structlog
from django.utils.deprecation import MiddlewareMixin

LOG_MODE = os.getenv("LOG_MODE", "none").strip().lower()

class LoggingMiddleware(MiddlewareMixin):
    def process_request(self, request):
        if LOG_MODE == "none":
            return  # ← bỏ qua hết khi none

        req_id = request.headers.get("x-request-id", str(uuid.uuid4()))
        structlog.contextvars.clear_contextvars()
        structlog.contextvars.bind_contextvars(
            req_id=req_id,
            service="django-service"
        )
        request.req_id = req_id

    def process_response(self, request, response):
        if LOG_MODE == "none":
            return response  # ← bỏ qua hết khi none

        req_id = getattr(request, 'req_id', None)
        if req_id:
            response["x-request-id"] = req_id
        return response