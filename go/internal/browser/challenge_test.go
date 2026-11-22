package browser

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

// fakeCFPage shows a challenge until clicked (or forever); time advances 1 s per sleep.
type fakeCFPage struct {
	challenge, solveOnClick bool
	clicks                  [][2]float64
	now                     *time.Time
}

func (p *fakeCFPage) cfCheck() (bool, error) { return p.challenge, nil }
func (p *fakeCFPage) frames() []cfFrame {
	f := []cfFrame{{id: "main", url: "https://example.com/"}}
	if p.challenge {
		f = append(f, cfFrame{id: "cf", url: "https://challenges.cloudflare.com/cdn-cgi/x"})
	}
	return f
}
func (p *fakeCFPage) frameBox(cfFrame) (*cfBox, error) { return &cfBox{100, 200, 300, 65}, nil }
func (p *fakeCFPage) click(x, y float64) error {
	p.clicks = append(p.clicks, [2]float64{x, y})
	if p.solveOnClick {
		p.challenge = false
	}
	return nil
}
func (p *fakeCFPage) pageURL() string { return "https://example.com/" }
func (p *fakeCFPage) sleep(ms int)    { *p.now = p.now.Add(time.Duration(ms) * time.Millisecond) }

func fakeCF(t *testing.T, challenge, solveOnClick bool, status int) (*fakeCFPage, *cfState) {
	now := time.Unix(1000, 0)
	start := now
	cfNow = func() time.Time { return now }
	t.Cleanup(func() {
		cfNow = time.Now
		if t.Failed() {
			t.Logf("fake clock at %s", now.Sub(start))
		}
	})
	return &fakeCFPage{challenge: challenge, solveOnClick: solveOnClick, now: &now}, &cfState{status: &status}
}

func TestCFNoChallengeReturnsQuickly(t *testing.T) {
	p, st := fakeCF(t, false, true, 200)
	start := *p.now
	data, err := waitCFChallenge(p, 60000, st)
	if err != nil {
		t.Fatal(err)
	}
	if got := omap.Marshal(data, 0); got != `{"challenge": false, "clicks": 0, "url": "https://example.com/"}` {
		t.Errorf("data %s", got)
	}
	if p.now.Sub(start) > 3*time.Second {
		t.Errorf("took %s", p.now.Sub(start))
	}
}

func TestCFSolvedByClick(t *testing.T) {
	p, st := fakeCF(t, true, true, 200)
	data, err := waitCFChallenge(p, 60000, st)
	if err != nil {
		t.Fatal(err)
	}
	if ch, _ := data.Get("challenge"); ch != true {
		t.Error("challenge not seen")
	}
	if n, _ := data.Get("clicks"); n != 1 {
		t.Errorf("clicks %v", n)
	}
	if len(p.clicks) != 1 || p.clicks[0] != [2]float64{130, 232.5} {
		t.Errorf("click points %v", p.clicks)
	}
}

func TestCFLoopTimesOut(t *testing.T) {
	p, st := fakeCF(t, true, false, 200)
	_, err := waitCFChallenge(p, 30000, st)
	ae, ok := err.(*apperr.Error)
	if !ok || ae.Code != apperr.Timeout || !strings.HasPrefix(ae.Message, "Cloudflare challenge not passed in 30000 ms (clicks: ") {
		t.Errorf("err %v", err)
	}
}

func TestCFBlockedPageFails(t *testing.T) {
	p, st := fakeCF(t, false, true, 403)
	_, err := waitCFChallenge(p, 60000, st)
	if err == nil || err.Error() != "Page blocked without challenge: HTTP 403 at https://example.com/" {
		t.Errorf("err %v", err)
	}
	p, st = fakeCF(t, true, true, 403)
	_, err = waitCFChallenge(p, 60000, st)
	if err == nil || !strings.Contains(err.Error(), "blocked after challenge: HTTP 403") {
		t.Errorf("err %v", err)
	}
}

func TestCFStateFromResponses(t *testing.T) {
	st := &cfState{}
	st.onNav(omap.New("cf-mitigated", "challenge"), 403)
	if !st.mitigated || *st.status != 403 {
		t.Errorf("state %+v", st)
	}
	st.onNav(omap.New(), 200)
	if st.mitigated || *st.status != 200 {
		t.Errorf("state %+v", st)
	}
}

func TestSolveChallengeBrowser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/blocked":
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, "no")
		case "/challenge":
			w.Header().Set("cf-mitigated", "challenge")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `<script>window._cf_chl_opt = {}; setTimeout(() => location.href = '/ok', 1500)</script>`)
		case "/f":
			fmt.Fprint(w, "inner")
		case "/frames":
			// cross-site iframe (an out-of-process frame) inside a closed shadow root
			u := strings.Replace("http://"+r.Host, "127.0.0.1", "localhost", 1)
			fmt.Fprintf(w, `<div id=h></div><script>document.getElementById('h').attachShadow({mode: 'closed'})`+
				`.innerHTML = '<iframe style="margin:50px" src="%s/f"></iframe>'</script>`, u)
		default:
			fmt.Fprint(w, "ok")
		}
	}))
	defer srv.Close()
	s := startSession(t)
	run, ok := runner(t, s)

	ok(run("open", "url", srv.URL+"/blocked"))
	res := run("solve-challenge", "timeout", 10000)
	if res.Success || res.Error != "Page blocked without challenge: HTTP 403 at "+srv.URL+"/blocked" {
		t.Errorf("blocked: %v %q", res.Success, res.Error)
	}
	ok(run("open", "url", srv.URL+"/challenge"))
	if got := ok(run("solve-challenge", "timeout", 20000)); got != `{"challenge": true, "clicks": 0, "url": "`+srv.URL+`/ok"}` {
		t.Errorf("challenge: %s", got)
	}

	ok(run("open", "url", srv.URL+"/frames"))
	tb, _ := s.tab("")
	var frames []cfFrame
	poll(5000, func() (bool, error) { frames = tb.frames(); return len(frames) == 2, nil })
	if len(frames) != 2 || !strings.HasPrefix(frames[1].url, "http://localhost:") {
		t.Fatalf("frames %v", frames)
	}
	box, err := tb.frameBox(frames[1])
	if err != nil || box == nil || box.x != 58 || box.width != 304 {
		t.Errorf("box %+v %v", box, err)
	}
}
