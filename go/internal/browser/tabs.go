package browser

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
)

// TabsFile is the file of saved tab URLs inside a profile directory.
const TabsFile = "tabs.json"

// platformSpoof is the CDP user agent override for a UA of another OS.
type platformSpoof struct {
	platform string
	mobile   bool
	meta     *proto.EmulationUserAgentMetadata
}

var (
	chromeVerRe  = regexp.MustCompile(`chrome/(\d+\.\d+\.\d+\.\d+)`)
	ntVerRe      = regexp.MustCompile(`windows nt (\d+\.\d+)`)
	macVerRe     = regexp.MustCompile(`mac os x (\d+[_.]\d+[_.]\d+)`)
	androidVerRe = regexp.MustCompile(`android (\d+(?:\.\d+)*)`)
	modelRe      = regexp.MustCompile(`;\s*([^;)]+?)\s*(?:Build|MIUI|\))`)
)

// detectPlatformFromUA is _detect_platform_from_ua: nil for a Linux UA.
func detectPlatformFromUA(userAgent string) *platformSpoof {
	if userAgent == "" {
		return nil
	}
	ua := strings.ToLower(userAgent)
	chromeVer := "130.0.0.0"
	if m := chromeVerRe.FindStringSubmatch(ua); m != nil {
		chromeVer = m[1]
	}
	brands := []*proto.EmulationUserAgentBrandVersion{
		{Brand: "Chromium", Version: chromeVer},
		{Brand: "Google Chrome", Version: chromeVer},
		{Brand: "Not/A)Brand", Version: "24.0.0.0"},
	}
	meta := func(platform, platformVersion, arch, model string, mobile bool) *proto.EmulationUserAgentMetadata {
		return &proto.EmulationUserAgentMetadata{
			Platform: platform, PlatformVersion: platformVersion, Architecture: arch, Model: model,
			Mobile: mobile, FullVersionList: brands, FullVersion: chromeVer, Bitness: "64",
		}
	}
	switch {
	case strings.Contains(ua, "windows nt"):
		nt := "10.0"
		if m := ntVerRe.FindStringSubmatch(ua); m != nil {
			nt = m[1]
		}
		return &platformSpoof{platform: "Win32", meta: meta("Windows", nt+".0", "x86", "", false)}
	case strings.Contains(ua, "macintosh") || strings.Contains(ua, "mac os x"):
		mac := "14.0.0"
		if m := macVerRe.FindStringSubmatch(ua); m != nil {
			mac = strings.ReplaceAll(m[1], "_", ".")
		}
		return &platformSpoof{platform: "MacIntel", meta: meta("macOS", mac, "arm", "", false)}
	case strings.Contains(ua, "android"):
		ver := "14"
		if m := androidVerRe.FindStringSubmatch(ua); m != nil {
			ver = m[1]
		}
		model := ""
		if m := modelRe.FindStringSubmatch(userAgent); m != nil {
			model = strings.TrimSpace(m[1])
		}
		return &platformSpoof{platform: "Linux armv8l", mobile: true, meta: meta("Android", ver, "arm", model, true)}
	}
	return nil
}

// tab is one registered page.
type tab struct {
	s      *Session
	id     string
	page   *rod.Page
	target proto.TargetTargetID
	net    network
}

// emulate applies viewport, touch, focus and user agent like the Playwright
// context options do for every page of the context.
func (s *Session) emulate(p *rod.Page) error {
	vp := s.viewport
	if err := (proto.EmulationSetDeviceMetricsOverride{
		Width: vp.Width, Height: vp.Height, DeviceScaleFactor: 1, Mobile: s.mobile,
		ScreenWidth: &vp.Width, ScreenHeight: &vp.Height,
	}).Call(p); err != nil {
		return err
	}
	if s.mobile {
		if err := (proto.EmulationSetTouchEmulationEnabled{Enabled: true, MaxTouchPoints: ptr(1)}).Call(p); err != nil {
			return err
		}
	}
	if err := (proto.EmulationSetFocusEmulationEnabled{Enabled: true}).Call(p); err != nil {
		return err
	}
	if s.ua == "" {
		return nil
	}
	override := proto.EmulationSetUserAgentOverride{UserAgent: s.ua}
	if s.spoof != nil {
		override.Platform = s.spoof.platform
		override.UserAgentMetadata = s.spoof.meta
	}
	return override.Call(p)
}

func ptr[T any](v T) *T { return &v }

// targets lists the page targets of the session context, oldest first.
func (s *Session) targets() ([]proto.TargetTargetID, error) {
	res, err := proto.TargetGetTargets{}.Call(s.root)
	if err != nil {
		return nil, err
	}
	var ids []proto.TargetTargetID
	for _, t := range res.TargetInfos {
		if t.Type != "page" {
			continue
		}
		if s.ctx != s.root && t.BrowserContextID != s.ctx.BrowserContextID {
			continue
		}
		ids = append(ids, t.TargetID)
	}
	return ids, nil
}

// register attaches to a target and gives it the next page_N id.
func (s *Session) register(id proto.TargetTargetID) (*tab, error) {
	p, err := s.root.PageFromTarget(id)
	if err != nil {
		return nil, err
	}
	if err := s.emulate(p); err != nil {
		return nil, err
	}
	t := &tab{s: s, id: fmt.Sprintf("page_%d", s.nextPage), page: p, target: id}
	s.nextPage++
	s.known[id] = true
	s.pages = append(s.pages, t)
	t.listen()
	slog.Info("Registered new tab " + t.id)
	return t, nil
}

func (s *Session) newTab() (*tab, error) {
	p, err := s.ctx.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.emulate(p); err != nil {
		return nil, err
	}
	t := &tab{s: s, id: fmt.Sprintf("page_%d", s.nextPage), page: p, target: p.TargetID}
	s.nextPage++
	s.known[p.TargetID] = true
	s.pages = append(s.pages, t)
	t.listen()
	slog.Info("Registered new tab " + t.id)
	return t, nil
}

// sync registers tabs opened by the page (popups) and drops closed ones.
func (s *Session) sync() {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids, err := s.targets()
	if err != nil {
		return
	}
	alive := map[proto.TargetTargetID]bool{}
	for _, id := range ids {
		alive[id] = true
		if !s.known[id] {
			if _, err := s.register(id); err != nil {
				slog.Warn("Failed to register new tab: " + err.Error())
				s.known[id] = true
			}
		}
	}
	kept := s.pages[:0]
	for _, t := range s.pages {
		if alive[t.target] {
			kept = append(kept, t)
		}
	}
	s.pages = kept
}

// tab is get_page: an empty id is the first tab.
func (s *Session) tab(pageID string) (*tab, error) {
	if s.root != nil {
		s.sync()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pages) == 0 {
		return nil, apperr.New(apperr.BrowserStartFailed, "No pages available in session")
	}
	if pageID == "" {
		return s.pages[0], nil
	}
	for _, t := range s.pages {
		if t.id == pageID {
			return t, nil
		}
	}
	return nil, apperr.New(apperr.PageNotFound, "Page not found: "+pageID)
}

func (t *tab) url() (string, error) {
	info, err := t.page.Info()
	if err != nil {
		return "", err
	}
	return info.URL, nil
}

// saveTabs writes the URLs of the open tabs to tabs.json of the profile.
func (s *Session) saveTabs() {
	if s.profileDir == "" {
		return
	}
	s.sync()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pages) == 0 {
		return
	}
	urls := []any{}
	for _, t := range s.pages {
		if u, err := t.url(); err == nil && u != "" && u != "about:blank" && u != "chrome://newtab/" {
			urls = append(urls, u)
		}
	}
	path := filepath.Join(s.profileDir, TabsFile)
	if len(urls) > 0 {
		if err := os.WriteFile(path, []byte(omap.Marshal(urls, 2)), 0o644); err != nil {
			slog.Warn("Failed to save " + path + ": " + err.Error())
			return
		}
		slog.Info(fmt.Sprintf("Saved %d tab(s) to %s", len(urls), path))
	} else if err := os.Remove(path); err == nil {
		slog.Info("Removed empty " + path)
	}
}

// loadSavedTabs reads tabs.json: the string items of a JSON list.
func loadSavedTabs(profileDir string) []string {
	path := filepath.Join(profileDir, TabsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn(fmt.Sprintf("Failed to load %s: %s", path, err))
		}
		return nil
	}
	v, err := omap.ParseJSON(data)
	if err != nil {
		slog.Warn(fmt.Sprintf("Failed to load %s: %s", path, err))
		return nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil
	}
	var urls []string
	for _, item := range list {
		if u, ok := item.(string); ok && u != "" {
			urls = append(urls, u)
		}
	}
	slog.Info(fmt.Sprintf("Loaded %d saved tab(s) from %s", len(urls), path))
	return urls
}
