FROM python:3.10-alpine

WORKDIR /app
COPY requirements.txt requirements.txt

# Install dependencies
RUN pip install --no-cache-dir  -r requirements.txt

# Run applications
CMD ["python", "-m", "uvicorn", "app:app", "--port", "8089", "--host", "0.0.0.0"]
