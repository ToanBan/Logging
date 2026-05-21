FROM python:3.12-slim

WORKDIR /app

COPY service/fastapi-service/requirements.txt ./

RUN pip install --no-cache-dir -r requirements.txt

COPY service/fastapi-service/main.py ./main.py

COPY shared ./shared

EXPOSE 3005

CMD ["uvicorn", "main:app", "--host", "0.0.0.0", "--port", "3005", "--no-access-log"]