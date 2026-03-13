FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PATH="/opt/venv/bin:${PATH}"

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 \
    python3-pip \
    python3-venv \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN python3 -m venv /opt/venv

WORKDIR /app

COPY pyproject.toml README.md ./
COPY autobrowser_client ./autobrowser_client
COPY docs ./docs
COPY tz.md ./tz.md

RUN pip install --upgrade pip && pip install .

ENTRYPOINT ["autobrowser"]
CMD ["version"]
