from __future__ import annotations

import pytest

from autobrowser_client.browser import service
from autobrowser_client.errors import BrowserStartError


def test_resolve_geoip_passes_proxy(monkeypatch):
    calls = []

    def fake(proxy_url):
        calls.append(proxy_url)
        return "Europe/Helsinki", "fi-FI"

    monkeypatch.setattr("cloakbrowser.geoip.resolve_proxy_geo", fake)
    assert service._resolve_geoip("http://127.0.0.1:9") == ("Europe/Helsinki", "fi-FI")
    assert calls == ["http://127.0.0.1:9"]


def test_resolve_geoip_failure_is_start_error(monkeypatch):
    def fail(proxy_url):
        raise RuntimeError("could not discover the egress IP")

    monkeypatch.setattr("cloakbrowser.geoip.resolve_proxy_geo", fail)
    with pytest.raises(BrowserStartError, match="egress IP"):
        service._resolve_geoip(None)
