FROM python:3.12-slim

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY manage.py ./
COPY core ./core

EXPOSE 3006

CMD ["python", "manage.py", "runserver", "0.0.0.0:3006", "--noreload"]
