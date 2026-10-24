"""Tests for the Cloudflare solve-challenge step logic (fake page, no browser)."""
from __future__ import annotations

import pytest

from autobrowser_client.browser import executor
from autobrowser_client.errors import AppError, ErrorCode


class FakeFrameElement:
    def bounding_box(self):
        return {"x": 100, "y": 200, "width": 300, "height": 65}


class FakeFrame:
    def __init__(self, url):
        self.url = url

    def frame_element(self):
        return FakeFrameElement()


class FakeMouse:
    def __init__(self, page):
        self.page = page
        self.clicks = []

    def click(self, x, y):
        self.clicks.append((x, y))
        if self.page.solve_on_click:
            self.page.challenge = False


class FakePage:
    """Shows a challenge until clicked (or forever); time advances 1 s per wait."""

    def __init__(self, challenge=True, solve_on_click=True, status=200):
        self.challenge = challenge
        self.solve_on_click = solve_on_click
        self.status = status
        self.url = "https://example.com/"
        self.main_frame = object()
        self.mouse = FakeMouse(self)

    @property
    def frames(self):
        frames = [FakeFrame(self.url)]
        if self.challenge:
            frames.append(FakeFrame("https://challenges.cloudflare.com/cdn-cgi/x"))
        return frames

    def evaluate(self, js):
        if js == executor._NAV_STATUS_JS:
            return self.status
        return self.challenge

    def on(self, event, handler):
        pass

    def remove_listener(self, event, handler):
        pass

    def wait_for_timeout(self, ms):
        FakeClock.now += ms / 1000


class FakeClock:
    now = 0.0

    @classmethod
    def monotonic(cls):
        return cls.now


@pytest.fixture(autouse=True)
def fake_time(monkeypatch):
    FakeClock.now = 0.0
    monkeypatch.setattr(executor.time, "monotonic", FakeClock.monotonic)


def test_no_challenge_returns_quickly():
    page = FakePage(challenge=False)
    data = executor.solve_cf_challenge(page, 60000)
    assert data == {"challenge": False, "clicks": 0, "url": "https://example.com/"}
    assert FakeClock.now <= 3


def test_challenge_solved_by_click():
    page = FakePage()
    data = executor.solve_cf_challenge(page, 60000)
    assert data["challenge"] is True
    assert data["clicks"] == 1
    assert page.mouse.clicks == [(130, 232.5)]


def test_challenge_loop_times_out():
    page = FakePage(solve_on_click=False)
    with pytest.raises(AppError) as exc:
        executor.solve_cf_challenge(page, 30000)
    assert exc.value.code == ErrorCode.TIMEOUT


def test_blocked_page_fails():
    page = FakePage(challenge=False, status=403)
    with pytest.raises(AppError, match="HTTP 403"):
        executor.solve_cf_challenge(page, 60000)
