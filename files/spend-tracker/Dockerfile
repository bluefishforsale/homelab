FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY collector.py vendors.json ./
USER nobody
ENTRYPOINT ["python", "-u", "collector.py"]
