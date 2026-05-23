FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive \
    PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PATH="/opt/venv/bin:${PATH}" \
    # Place patchright browsers in a shared, non-root location
    PLAYWRIGHT_BROWSERS_PATH=/opt/playwright-browsers

# System deps + browser runtime libraries
RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 \
    python3-pip \
    python3-venv \
    ca-certificates \
    curl \
    # Browser runtime dependencies (Chromium/Chrome)
    libasound2t64 \
    libatk-bridge2.0-0 \
    libatk1.0-0 \
    libcups2 \
    libdbus-1-3 \
    libdrm2 \
    libgbm1 \
    libgtk-3-0 \
    libnspr4 \
    libnss3 \
    libpango-1.0-0 \
    libx11-6 \
    libxcomposite1 \
    libxdamage1 \
    libxext6 \
    libxfixes3 \
    libxkbcommon0 \
    libxrandr2 \
    libxshmfence1 \
    libxss1 \
    xdg-utils \
    fonts-liberation \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd -r browser && useradd -r -g browser -m -d /home/browser browser

# Python venv
RUN python3 -m venv /opt/venv

WORKDIR /app

# Install both engines: patchright (default) + cloakbrowser (optional)
ARG INSTALL_CLOAKBROWSER=1
COPY pyproject.toml README.md ./
COPY autobrowser_client ./autobrowser_client

RUN pip install --upgrade pip && \
    if [ "$INSTALL_CLOAKBROWSER" = "1" ]; then pip install ".[cloak]"; else pip install .; fi

# Pre-download patchright chrome into shared location (accessible by all users)
RUN patchright install chrome && \
    chmod -R a+rX /opt/playwright-browsers

# Pre-download cloakbrowser binary as the browser user (lands in /home/browser/.cloakbrowser)
RUN if [ "$INSTALL_CLOAKBROWSER" = "1" ]; then \
        su browser -c "/opt/venv/bin/python -c 'from cloakbrowser.download import ensure_binary; ensure_binary()'"; \
    fi

# Create directories for profiles and data
RUN mkdir -p /data/profiles /data/tmp && chown -R browser:browser /data /app

# Switch to non-root user
USER browser

# Default volumes
VOLUME ["/data/profiles"]

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD curl -sf http://127.0.0.1:8080/health || exit 1

ENTRYPOINT ["autobrowser"]
CMD ["serve", "--host", "0.0.0.0", "--port", "8080"]
