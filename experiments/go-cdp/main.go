// Prototype: drive the cloakbrowser Chromium binary over plain CDP with go-rod.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
)

type site struct {
	name string
	url  string
	wait time.Duration
}

// cdpCheck is the classic Runtime.enable probe: the stack getter of a logged
// Error only fires when a CDP client has the Runtime domain enabled.
const cdpCheck = `data:text/html,<title>pending</title><script>
let hit = false; const e = new Error();
Object.defineProperty(e, 'stack', {get() { hit = true; return ''; }});
console.debug(e);
setTimeout(() => { document.title = hit ? 'CDP DETECTED' : 'CDP not detected'; }, 500);
</script>`

var defaultSites = []site{
	{"cdpcheck", cdpCheck, time.Second},
	{"sannysoft", "https://bot.sannysoft.com/", 3 * time.Second},
	{"creepjs", "https://abrahamjuliot.github.io/creepjs/", 8 * time.Second},
	{"browserscan", "https://www.browserscan.net/bot-detection", 5 * time.Second},
	{"areyouheadless", "https://arh.antoinevastel.com/bots/areyouheadless", 2 * time.Second},
	{"botd", "https://fingerprintjs.github.io/BotD/main/", 4 * time.Second},
	{"criptonoticias", "https://www.criptonoticias.com/", 0},
}

var runtimeEnable bool

type summary struct {
	Name       string  `json:"name"`
	URL        string  `json:"url"`
	FinalURL   string  `json:"final_url"`
	Title      string  `json:"title"`
	Webdriver  string  `json:"navigator_webdriver"`
	UserAgent  string  `json:"user_agent"`
	Cloudflare string  `json:"cloudflare,omitempty"`
	Error      string  `json:"error,omitempty"`
	Seconds    float64 `json:"seconds"`
}

func main() {
	bin := flag.String("bin", "/home/browser/.cloakbrowser/chromium-146.0.7680.177.5/chrome", "chromium binary")
	platform := flag.String("platform", "windows", "fingerprint platform: windows|linux")
	headless := flag.Bool("headless", true, "run headless")
	useStealth := flag.Bool("stealth", false, "inject go-rod/stealth script")
	out := flag.String("out", "out", "output dir")
	url := flag.String("url", "", "visit only this URL")
	flag.BoolVar(&runtimeEnable, "runtime-enable", false, "call Runtime.enable on each page (to validate the CDP probe)")
	flag.Parse()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}

	start := time.Now()
	l := launcher.New().
		Bin(*bin).
		Headless(*headless).
		Leakless(false).
		Delete("enable-automation").
		Set("no-sandbox").
		Set("fingerprint", strconv.Itoa(rand.Intn(1<<31))).
		Set("fingerprint-platform", *platform).
		Set("window-size", "1366,900")
	wsURL, err := l.Launch()
	if err != nil {
		log.Fatal(err)
	}
	// rod emulates a default laptop device (Chrome/114 Mac UA) unless disabled
	browser := rod.New().ControlURL(wsURL).NoDefaultDevice().MustConnect()
	defer browser.MustClose()
	log.Printf("browser up in %v, args: %v", time.Since(start), l.FormatArgs())

	sites := defaultSites
	if *url != "" {
		sites = []site{{"custom", *url, 3 * time.Second}}
	}
	for _, s := range sites {
		sum := visit(browser, s, *useStealth, *out)
		b, _ := json.MarshalIndent(sum, "", "  ")
		os.WriteFile(filepath.Join(*out, s.name+".json"), b, 0o644)
		fmt.Println(string(b))
	}
}

func visit(browser *rod.Browser, s site, useStealth bool, out string) (sum summary) {
	sum = summary{Name: s.name, URL: s.url}
	t0 := time.Now()
	defer func() {
		if r := recover(); r != nil {
			sum.Error = fmt.Sprint(r)
		}
		sum.Seconds = time.Since(t0).Seconds()
	}()

	var page *rod.Page
	if useStealth {
		page = stealth.MustPage(browser)
	} else {
		page = browser.MustPage("")
	}
	defer page.Close()
	page.MustSetViewport(1366, 900, 1, false)
	if runtimeEnable {
		proto.RuntimeEnable{}.Call(page)
	}

	p := page.Timeout(45 * time.Second)
	if err := p.Navigate(s.url); err != nil {
		sum.Error = err.Error()
	}
	p.WaitLoad()
	p.WaitIdle(10 * time.Second)
	time.Sleep(s.wait)

	if s.name == "criptonoticias" || strings.Contains(s.url, "criptonoticias") {
		sum.Cloudflare = waitCloudflare(page)
	}

	if info, err := page.Info(); err == nil {
		sum.FinalURL, sum.Title = info.URL, info.Title
	}
	if r, err := page.Eval(`() => String(navigator.webdriver)`); err == nil {
		sum.Webdriver = r.Value.Str()
	}
	if r, err := page.Eval(`() => navigator.userAgent`); err == nil {
		sum.UserAgent = r.Value.Str()
	}

	img, err := page.Timeout(30*time.Second).Screenshot(true, &proto.PageCaptureScreenshot{Format: proto.PageCaptureScreenshotFormatPng})
	if err != nil {
		sum.Error = strings.TrimSpace(sum.Error + " screenshot: " + err.Error())
	} else {
		os.WriteFile(filepath.Join(out, s.name+".png"), img, 0o644)
	}
	html, _ := page.HTML()
	os.WriteFile(filepath.Join(out, s.name+".html"), []byte(html), 0o644)
	return sum
}

// waitCloudflare waits up to 30s for the challenge title to go away,
// clicking the Turnstile checkbox once if its iframe shows up.
func waitCloudflare(page *rod.Page) string {
	clicked := false
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		info, err := page.Info()
		if err == nil && !strings.Contains(info.Title, "Just a moment") {
			return "passed"
		}
		if !clicked {
			// the iframe lives in a closed shadow root, so target the widget
			// container: the parent of the hidden turnstile response input
			if el, err := widget(page); err == nil {
				if box, err := el.Shape(); err == nil && len(box.Quads) > 0 && box.Box().Height > 0 {
					b := box.Box()
					// the checkbox sits near the left edge of the widget
					page.Mouse.MustMoveTo(b.X+30, b.Y+b.Height/2)
					page.Mouse.MustClick(proto.InputMouseButtonLeft)
					clicked = true
					log.Printf("clicked turnstile at %.0f,%.0f", b.X+30, b.Y+b.Height/2)
				}
			}
		}
		time.Sleep(time.Second)
	}
	if clicked {
		return "blocked (turnstile clicked)"
	}
	return "blocked"
}

func widget(page *rod.Page) (*rod.Element, error) {
	input, err := page.Timeout(time.Second).Element(`input[name="cf-turnstile-response"]`)
	if err != nil {
		return nil, err
	}
	return input.Parent()
}
