import os
from pathlib import Path
import structlog

BASE_DIR = Path(__file__).resolve().parent.parent

# SECURITY WARNING: keep the secret key used in production secret!
SECRET_KEY = 'django-insecure-benchmark-secret-key-123456789'

# SECURITY WARNING: don't run with debug turned on in production!
DEBUG = False

ALLOWED_HOSTS = ['*']

# Application definition
INSTALLED_APPS = []

MIDDLEWARE = [
    'core.middleware.LoggingMiddleware',
]

ROOT_URLCONF = 'core.urls'

WSGI_APPLICATION = 'core.wsgi.application'

# Dummy database since we are bypassing ORM for ultra-high performance connection pooling
DATABASES = {
    'default': {
        'ENGINE': 'django.db.backends.sqlite3',
        'NAME': ':memory:',
    }
}

# Structlog structured logging setup - Unified configuration
LOG_MODE = os.getenv("LOG_MODE", "none").strip().lower()
import logging

processors = [
    structlog.contextvars.merge_contextvars,
    structlog.processors.add_log_level,
    structlog.processors.TimeStamper(fmt="iso"),
]

if LOG_MODE == "none":
    # Disable logging completely
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
    # Only show warnings and errors - suppress info logs and http access logs
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
    # structured mode - show all logs
    structlog.configure(
        processors=processors + [structlog.processors.JSONRenderer()],
        context_class=dict,
        logger_factory=structlog.PrintLoggerFactory(),
        cache_logger_on_first_use=True,
    )
    LOGGING = {
        'version': 1,
        'disable_existing_loggers': False,
    }
