package browser

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

func TestNavigateHistoryScreenshot(t *testing.T) {
	s := startSession(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<title>P</title><p id=p>hi</p>"))
	}))
	defer srv.Close()

	res, err := s.Navigate("", srv.URL+"/", 10000)
	if err != nil {
		t.Fatal(err)
	}
	if got := omap.Marshal(res, 0); got != `{"url": "`+srv.URL+`/", "title": "P", "status": 200}` {
		t.Fatal(got)
	}
	if res, err = s.Navigate("page_0", srv.URL+"/missing", 10000); err != nil || get(res, "status") != 404 {
		t.Fatalf("%v %v", res, err)
	}
	if res, err = s.Navigate("", "data:text/html,<title>D</title>", 10000); err != nil ||
		omap.Marshal(res, 0) != `{"url": "data:text/html,<title>D</title>", "title": "D", "status": null}` {
		t.Fatalf("%v %v", res, err)
	}
	if res, err = s.History("", "back", 10000); err != nil || get(res, "url") != srv.URL+"/missing" {
		t.Fatalf("%v %v", res, err)
	}

	png := []byte("\x89PNG")
	for _, l := range []*scenario.Locator{nil, {Type: "css", Value: "#p"}} {
		data, err := s.Screenshot("", false, l, 5000)
		if err != nil || !bytes.HasPrefix(data, png) {
			t.Fatalf("screenshot %v: %v", l, err)
		}
	}
	if _, err := s.Screenshot("", false, &scenario.Locator{Type: "css", Value: "#none"}, 300); err == nil {
		t.Fatal("missing element screenshot succeeded")
	}
}

func get(m *omap.Map, key string) any {
	v, _ := m.Get(key)
	return v
}
