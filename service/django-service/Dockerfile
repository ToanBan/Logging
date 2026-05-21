FROM python:3.12-slim

WORKDIR /app

COPY service/django-service/requirements.txt ./

RUN pip install --no-cache-dir -r requirements.txt

COPY service/django-service/manage.py ./manage.py

COPY service/django-service/core ./core

COPY shared ./shared

EXPOSE 3006

CMD ["python", "manage.py", "runserver", "0.0.0.0:3006", "--noreload"]
