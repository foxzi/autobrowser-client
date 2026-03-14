#!/usr/bin/env bash
# Docker smoke test: build image, start API, verify health + browser session.
set -euo pipefail

IMAGE="autobrowser-client:dev"
CONTAINER_NAME="smoke-test-api"
PORT=18090

cleanup() {
    echo "Cleaning up..."
    docker rm -f "$CONTAINER_NAME" 2>/dev/null || true
}
trap cleanup EXIT

echo "=== Building Docker image ==="
docker compose build api --quiet

echo "=== Starting API container ==="
docker run -d \
    --name "$CONTAINER_NAME" \
    -p "$PORT:8080" \
    "$IMAGE"

echo "=== Waiting for API to be ready ==="
for i in $(seq 1 30); do
    if curl -sf "http://127.0.0.1:$PORT/health" >/dev/null 2>&1; then
        echo "API is healthy"
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo "FAIL: API did not become healthy"
        docker logs "$CONTAINER_NAME"
        exit 1
    fi
    sleep 1
done

echo "=== Testing health endpoint ==="
HEALTH=$(curl -sf "http://127.0.0.1:$PORT/health")
echo "$HEALTH" | grep -q '"ok"' || { echo "FAIL: health check"; exit 1; }
echo "OK: $HEALTH"

echo "=== Testing ready endpoint ==="
READY=$(curl -sf "http://127.0.0.1:$PORT/ready")
echo "$READY" | grep -q '"ready"' || { echo "FAIL: ready check"; exit 1; }
echo "OK: $READY"

echo "=== Testing info endpoint ==="
INFO=$(curl -sf "http://127.0.0.1:$PORT/api/v1/info")
echo "$INFO" | grep -q 'autobrowser-client' || { echo "FAIL: info"; exit 1; }
echo "OK: $INFO"

echo "=== Testing browser session start ==="
START=$(curl -sf -X POST "http://127.0.0.1:$PORT/api/v1/browser/start" \
    -H "Content-Type: application/json" \
    -d '{"urls": ["https://www.opennet.ru"], "headless": true}')
echo "$START" | grep -q '"success":true' || { echo "FAIL: browser start"; exit 1; }
SESSION_ID=$(echo "$START" | python3 -c "import sys,json; print(json.load(sys.stdin)['sessionId'])")
echo "OK: session=$SESSION_ID"

echo "=== Testing session cookies ==="
COOKIES=$(curl -sf "http://127.0.0.1:$PORT/api/v1/session/$SESSION_ID/cookies")
echo "$COOKIES" | grep -q '"success":true' || { echo "FAIL: cookies"; exit 1; }
echo "OK: cookies retrieved"

echo "=== Testing session stop ==="
STOP=$(curl -sf -X POST "http://127.0.0.1:$PORT/api/v1/session/$SESSION_ID/stop")
echo "$STOP" | grep -q '"stopped"' || { echo "FAIL: stop"; exit 1; }
echo "OK: session stopped"

echo "=== Testing CLI version ==="
VERSION=$(docker run --rm "$IMAGE" version)
echo "$VERSION" | grep -q '0.1.0' || { echo "FAIL: version"; exit 1; }
echo "OK: version=$VERSION"

echo ""
echo "=== All Docker smoke tests passed ==="
