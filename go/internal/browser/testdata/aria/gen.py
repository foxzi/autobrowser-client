"""Regenerate the expected aria snapshots with the Python client (the spec).

Run from the repo root in the reference image (see aria_test.go):

  docker run --rm --user "$(id -u):$(id -g)" -e HOME=/tmp \
    -e CLOAKBROWSER_CACHE_DIR=/home/browser/.cloakbrowser -e PYTHONPATH=/w/python \
    -v "$PWD:/w" -w /w/go/internal/browser/testdata/aria \
    --entrypoint /opt/venv/bin/python parsers-autobrowser:latest gen.py > expected.json

The JSON on stdout maps fixture names to snapshots, host:port replaced by HOST.
"""

import http.server
import json
import tempfile
import threading

from autobrowser_client.browser import actions
from autobrowser_client.browser.service import BrowserSession
from autobrowser_client.config import load_settings

class Handler(http.server.SimpleHTTPRequestHandler):
    def log_message(self, *args):
        pass


server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
threading.Thread(target=server.serve_forever, daemon=True).start()
host = f"127.0.0.1:{server.server_port}"

settings = load_settings(env={}, overrides={"profiles": {"dir": tempfile.mkdtemp()}})
s = BrowserSession("gen", settings)
s.start(headless=True)
out = {}


def snap(name):
    text = s.call(actions.page_snapshot, s, "page_0")["snapshot"]
    out[name] = text.replace(host, "HOST").replace(
        f"localhost:{server.server_port}", "LOCALHOST"
    )


def call(fn, *args):
    s.call(fn, s, "page_0", *args)


try:
    for page in ("basic", "frames"):
        call(actions.navigate, f"http://{host}/{page}.html")
        snap(page)
    # Refs found inside frames work as locators.
    call(actions.click_element, "ref", "f3e3")
    call(actions.fill_element, "ref", "f2e3", "typed")
    snap("frames-after")

    call(actions.navigate, f"http://{host}/dynamic.html")
    snap("dynamic-1")
    call(actions.click_element, "ref", "f5e2")
    call(actions.fill_element, "ref", "f5e3", "hello")
    snap("dynamic-2")
finally:
    s.stop()
    server.shutdown()
    print(json.dumps(out, indent=1))
