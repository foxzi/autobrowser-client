package cli

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/foxzi/autobrowser-client/go/internal/fingerprint"
	"github.com/foxzi/autobrowser-client/go/internal/omap"
	"github.com/foxzi/autobrowser-client/go/internal/scenario"
)

// skillFS holds a byte-identical copy of python/skills/autobrowser/SKILL.md
// (go:embed cannot reach ../python; CI diffs the two).
//
//go:embed skill/SKILL.md
var skillFS embed.FS

func validateScript(e *env) error {
	fs := newFlags(e, "validate-script")
	var file string
	fs.StringVar(&file, "file", "", "Scenario file.")
	fs.StringVar(&file, "f", "", "Scenario file (short).")
	if err := parse(fs, e.args); err != nil {
		return err
	}
	if err := checkFile(file); err != nil {
		return err
	}
	script, err := scenario.Load(file)
	if err != nil {
		return err
	}
	fmt.Fprintln(e.stdout, omap.Marshal(omap.New("success", true, "steps", len(script.Steps),
		"stopOnError", script.StopOnError), 2))
	return nil
}

// checkFile is Click's exists=True, dir_okay=False check of --file.
func checkFile(file string) error {
	if file == "" {
		return &usageError{"Missing option '--file' / '-f'."}
	}
	st, err := os.Stat(file)
	switch {
	case err != nil:
		return &usageError{fmt.Sprintf("Invalid value for '--file' / '-f': File '%s' does not exist.", file)}
	case st.IsDir():
		return &usageError{fmt.Sprintf("Invalid value for '--file' / '-f': File '%s' is a directory.", file)}
	}
	return nil
}

func generateFingerprint(e *env) error {
	fs := newFlags(e, "generate-fingerprint")
	profile := fs.String("profile", "", "Profile directory path")
	platform := fs.String("platform", "windows", "Target platform: windows, linux, android")
	seed := fs.String("seed", "", "Fixed seed value")
	force := fs.Bool("force", false, "Overwrite existing fingerprint.json")
	if err := parse(fs, e.args); err != nil {
		return err
	}
	if !flagSet(fs, "profile") {
		return &usageError{"Missing option '--profile'."}
	}
	var seedPtr *int
	if flagSet(fs, "seed") {
		n, err := strconv.Atoi(strings.TrimSpace(*seed))
		if err != nil {
			return &usageError{fmt.Sprintf("Invalid value for '--seed': '%s' is not a valid int.", *seed)}
		}
		seedPtr = &n
	}
	if !knownPlatform(*platform) {
		fmt.Fprintf(e.stdout, "Unknown platform: %s. Use: windows, linux, android\n", *platform)
		return &exitStatus{1}
	}
	if _, err := os.Stat(*profile); err != nil {
		if err := os.MkdirAll(*profile, 0o777); err != nil {
			return err
		}
		fmt.Fprintf(e.stdout, "Created profile directory: %s\n", *profile)
	}
	path := filepath.Join(*profile, fingerprint.File)
	if _, err := os.Stat(path); err == nil && !*force {
		fmt.Fprintf(e.stdout, "fingerprint.json already exists at %s\n", path)
		fmt.Fprintln(e.stdout, "Use --force to overwrite")
		return &exitStatus{1}
	}
	fp := fingerprint.Generate(seedPtr, fingerprint.Platform(*platform))
	if err := fingerprint.Save(*profile, fp); err != nil {
		return err
	}
	fmt.Fprintln(e.stdout, omap.Marshal(fp.JSConfig(), 2))
	return nil
}

func flagSet(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) { found = found || f.Name == name })
	return found
}

func knownPlatform(platform string) bool {
	return platform == "windows" || platform == "linux" || platform == "android"
}

func installSkill(e *env) error {
	fs := newFlags(e, "install-skill")
	dir := fs.String("dir", "~/.claude/skills", "Skills directory of the agent client.")
	if err := parse(fs, e.args); err != nil {
		return err
	}
	data, err := skillFS.ReadFile("skill/SKILL.md")
	if err != nil {
		return err
	}
	target := filepath.Join(expandUser(*dir), "autobrowser", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o777); err != nil {
		return err
	}
	if err := os.WriteFile(target, data, 0o666); err != nil {
		return err
	}
	fmt.Fprintln(e.stdout, target)
	return nil
}

// expandUser is Path.expanduser for a leading "~" or "~/".
func expandUser(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return home + path[1:]
		}
	}
	return path
}
