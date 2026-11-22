import threading
from concurrent.futures import ThreadPoolExecutor

from autobrowser_client.browser.service import BrowserSession
from autobrowser_client.config import load_settings


def test_call_runs_on_one_thread_from_any_caller():
    session = BrowserSession("sess_test", load_settings())
    with ThreadPoolExecutor(4) as pool:
        idents = set(pool.map(lambda _: session.call(threading.get_ident), range(8)))
    assert len(idents) == 1
    assert threading.get_ident() not in idents


def test_nested_call_runs_inline():
    session = BrowserSession("sess_test", load_settings())
    outer = session.call(
        lambda: (threading.get_ident(), session.call(threading.get_ident))
    )
    assert outer[0] == outer[1]
