"""Unit tests for platform spoofing (OS detection from User-Agent)."""

from __future__ import annotations

import pytest

from autobrowser_client.browser.service import _detect_platform_from_ua


# --- Windows ---

def test_detect_windows_10():
    ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
    result = _detect_platform_from_ua(ua)
    assert result is not None
    assert result["platform"] == "Win32"
    meta = result["userAgentMetadata"]
    assert meta["platform"] == "Windows"
    assert meta["platformVersion"] == "10.0.0"
    assert meta["architecture"] == "x86"
    assert meta["bitness"] == "64"
    assert meta["mobile"] is False


def test_detect_windows_11():
    ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"
    result = _detect_platform_from_ua(ua)
    assert result is not None
    assert result["platform"] == "Win32"
    # Chrome version extracted correctly
    meta = result["userAgentMetadata"]
    versions = [b["version"] for b in meta["fullVersionList"] if b["brand"] == "Google Chrome"]
    assert versions == ["130.0.0.0"]


def test_detect_windows_7():
    ua = "Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/109.0.0.0 Safari/537.36"
    result = _detect_platform_from_ua(ua)
    assert result is not None
    assert result["platform"] == "Win32"
    assert result["userAgentMetadata"]["platformVersion"] == "6.1.0"


# --- macOS ---

def test_detect_macos():
    ua = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
    result = _detect_platform_from_ua(ua)
    assert result is not None
    assert result["platform"] == "MacIntel"
    meta = result["userAgentMetadata"]
    assert meta["platform"] == "macOS"
    assert meta["platformVersion"] == "10.15.7"
    assert meta["architecture"] == "arm"
    assert meta["mobile"] is False


def test_detect_macos_14():
    ua = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0_0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
    result = _detect_platform_from_ua(ua)
    assert result is not None
    assert result["platform"] == "MacIntel"
    assert result["userAgentMetadata"]["platformVersion"] == "14.0.0"


# --- Linux (no spoofing) ---

def test_detect_linux_returns_none():
    ua = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
    result = _detect_platform_from_ua(ua)
    assert result is None


def test_detect_linux_ubuntu_returns_none():
    ua = "Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0"
    result = _detect_platform_from_ua(ua)
    assert result is None


# --- Chrome version extraction ---

def test_chrome_version_extracted():
    ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.6422.112 Safari/537.36"
    result = _detect_platform_from_ua(ua)
    meta = result["userAgentMetadata"]
    assert meta["fullVersion"] == "125.0.6422.112"
    chrome_brands = [b for b in meta["fullVersionList"] if b["brand"] == "Google Chrome"]
    assert chrome_brands[0]["version"] == "125.0.6422.112"


def test_no_chrome_version_uses_default():
    ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
    result = _detect_platform_from_ua(ua)
    assert result is not None
    meta = result["userAgentMetadata"]
    assert meta["fullVersion"] == "130.0.0.0"


# --- Edge cases ---

def test_empty_ua():
    result = _detect_platform_from_ua("")
    assert result is None


def test_unknown_ua():
    result = _detect_platform_from_ua("curl/8.0")
    assert result is None
