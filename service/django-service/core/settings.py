import os
from pathlib import Path
import structlog

BASE_DIR = Path(__file__).resolve().parent.parent

SECRET_KEY = 'django-insecure-benchmark-secret-key-123456789'

DEBUG = False

ALLOWED_HOSTS = ['*']

INSTALLED_APPS = []

MIDDLEWARE = [
    'core.middleware.LoggingMiddleware',
]

ROOT_URLCONF = 'core.urls'

WSGI_APPLICATION = 'core.wsgi.application'

DATABASES = {
    'default': {
        'ENGINE': 'django.db.backends.sqlite3',
        'NAME': ':memory:',
    }
}

LOG_MODE = os.getenv("LOG_MODE", "none").strip().lower()
import logging

processors = [
    structlog.contextvars.merge_contextvars,
    structlog.processors.add_log_level,
    structlog.processors.TimeStamper(fmt="iso"),
]

if LOG_MODE == "none":
    logging.disable(logging.CRITICAL)
    structlog.configure(
        processors=[structlog.processors.JSONRenderer()],
        logger_factory=structlog.WriteLoggerFactory(file=open(os.devnull, "w")),
        cache_logger_on_first_use=True,
    )
    LOGGING = {
        'version': 1,
        'disable_existing_loggers': True,
    }
elif LOG_MODE == "selective":
    structlog.configure(
        processors=processors + [structlog.processors.JSONRenderer()],
        context_class=dict,
        wrapper_class=structlog.make_filtering_bound_logger(logging.WARNING),
        logger_factory=structlog.PrintLoggerFactory(),
        cache_logger_on_first_use=True,
    )
    LOGGING = {
        'version': 1,
        'disable_existing_loggers': False,
        'formatters': {
            'standard': {
                'format': '{levelname} {name} {message}',
                'style': '{',
            },
        },
        'handlers': {
            'null': {
                'class': 'logging.NullHandler',
            },
        },
        'loggers': {
            'django.server': {
                'handlers': ['null'],
                'level': 'CRITICAL',
                'propagate': False,
            },
            'django.request': {
                'handlers': ['null'],
                'level': 'CRITICAL',
                'propagate': False,
            },
            'django.db': {
                'handlers': ['null'],
                'level': 'CRITICAL',
                'propagate': False,
            },
        },
    }
else:
    structlog.configure(
        processors=processors + [structlog.processors.JSONRenderer()],
        context_class=dict,
        logger_factory=structlog.PrintLoggerFactory(),
        cache_logger_on_first_use=True,
    )
    LOGGING = {
        'version': 1,
        'disable_existing_loggers': False,
        'handlers': {
            'null': {
                'class': 'logging.NullHandler',
            },
        },
        'loggers': {
            'django.server': {
                'handlers': ['null'],
                'level': 'CRITICAL',
                'propagate': False,
            },
            'django.request': {
                'handlers': ['null'],
                'level': 'CRITICAL',
                'propagate': False,
            },
        },
    }