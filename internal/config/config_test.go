package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// profileFile writes a configuration file in a temporary directory and returns
// its path.
func profileFile(t *testing.T, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// A missing file is not an error: every command has to work before anything has
// been configured.
func TestLoadMissingFileYieldsAnEmptyDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nothing-here.yaml")

	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(file.Profiles) != 0 {
		t.Errorf("loaded %d profile(s) from a missing file", len(file.Profiles))
	}
	if file.SourcePath() != path {
		t.Errorf("SourcePath = %q, want %q", file.SourcePath(), path)
	}
}

func TestLoadReadsProfiles(t *testing.T) {
	path := profileFile(t, `
default: plant
profiles:
  plant:
    endpoints:
      - opc.tcp://plant:4840
      - opc.tcp://plant-b:4840
    security:
      policy: Basic256Sha256
      mode: SignAndEncrypt
    auth:
      mode: username
      username: operator
    session:
      timeout: 5m
      requestTimeout: 15s
  local:
    endpoints: [opc.tcp://localhost:4840]
`)

	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	name, profile, err := file.Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if name != "plant" {
		t.Errorf("the default profile is %q, want plant", name)
	}
	if len(profile.Endpoints) != 2 || profile.Endpoint() != "opc.tcp://plant:4840" {
		t.Errorf("endpoints = %v, want the two configured, primary first", profile.Endpoints)
	}
	if profile.Security.Policy != "Basic256Sha256" || profile.Auth.Username != "operator" {
		t.Errorf("profile = %+v, want the configured security and credentials", profile)
	}

	if got := file.Names(); len(got) != 2 || got[0] != "local" || got[1] != "plant" {
		t.Errorf("Names = %v, want the profiles in alphabetical order", got)
	}
}

// With exactly one profile there is nothing to choose between, so it is the
// default whether or not the file says so.
func TestSingleProfileIsTheDefault(t *testing.T) {
	path := profileFile(t, "profiles:\n  only:\n    endpoints: [opc.tcp://host:4840]\n")

	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := file.DefaultName(); got != "only" {
		t.Errorf("DefaultName = %q, want only", got)
	}
}

func TestResolveUnknownProfileNamesTheAlternatives(t *testing.T) {
	path := profileFile(t, "profiles:\n  one:\n    endpoints: [opc.tcp://a:4840]\n  two:\n    endpoints: [opc.tcp://b:4840]\n")

	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	_, _, err = file.Resolve("three")
	if err == nil {
		t.Fatal("Resolve accepted an unknown profile")
	}
	if !strings.Contains(err.Error(), "one, two") {
		t.Errorf("error %q does not list the configured profiles", err)
	}
}

// An empty file resolves to an empty profile rather than an error, so flags alone
// are enough to reach a server.
func TestResolveWithNothingConfigured(t *testing.T) {
	file, err := Load(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	name, profile, err := file.Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if name != "" {
		t.Errorf("resolved to %q, want no profile", name)
	}
	if got := profile.Endpoint(); got != DefaultEndpoint {
		t.Errorf("Endpoint = %q, want the built-in default", got)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	profile := file.Upsert("plant")
	profile.Endpoints = []string{"opc.tcp://plant:4840"}
	profile.Security.Policy = "Basic256Sha256"
	profile.Auth.Username = "operator"

	if err := file.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// A file that can hold a password must not be world readable.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("mode = %o, want 600", mode)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	name, saved, err := reloaded.Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if name != "plant" {
		t.Errorf("the first profile added did not become the default, got %q", name)
	}
	if saved.Security.Policy != "Basic256Sha256" || saved.Auth.Username != "operator" {
		t.Errorf("reloaded profile = %+v, want what was saved", saved)
	}
}

func TestRemoveClearsTheDefault(t *testing.T) {
	path := profileFile(t, "default: plant\nprofiles:\n  plant:\n    endpoints: [opc.tcp://a:4840]\n")

	file, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := file.Remove("plant"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if file.Default != "" {
		t.Errorf("Default = %q, want it cleared with the profile it named", file.Default)
	}
	if err := file.Remove("plant"); err == nil {
		t.Error("Remove accepted a profile that is not there")
	}
}

func TestSet(t *testing.T) {
	for _, test := range []struct {
		key   string
		value string
		check func(*Profile) bool
	}{
		{"endpoint", "opc.tcp://a:4840,opc.tcp://b:4840", func(p *Profile) bool { return len(p.Endpoints) == 2 }},
		{"security.policy", "Basic256", func(p *Profile) bool { return p.Security.Policy == "Basic256" }},
		{"security.insecure", "true", func(p *Profile) bool { return p.Security.Insecure }},
		{"auth.username", "operator", func(p *Profile) bool { return p.Auth.Username == "operator" }},
		{"session.timeout", "90s", func(p *Profile) bool { return p.Session.Timeout == "90s" }},
		{"subscription.queueSize", "25", func(p *Profile) bool { return p.Subscription.QueueSize == 25 }},
		{"session.locales", "en-GB, de", func(p *Profile) bool { return len(p.Session.Locales) == 2 && p.Session.Locales[1] == "de" }},
	} {
		t.Run(test.key, func(t *testing.T) {
			profile := &Profile{}
			if err := profile.Set(test.key, test.value); err != nil {
				t.Fatalf("Set(%q, %q): %v", test.key, test.value, err)
			}
			if !test.check(profile) {
				t.Errorf("Set(%q, %q) left the profile as %+v", test.key, test.value, profile)
			}
		})
	}
}

func TestSetRejectsBadInput(t *testing.T) {
	profile := &Profile{}

	for _, test := range []struct{ key, value string }{
		{"nonsense.key", "x"},
		{"security.insecure", "maybe"},
		{"session.timeout", "a while"},
		{"subscription.queueSize", "lots"},
	} {
		if err := profile.Set(test.key, test.value); err == nil {
			t.Errorf("Set(%q, %q) succeeded, want an error", test.key, test.value)
		}
	}
}

func TestSettableKeysAreAllSettable(t *testing.T) {
	values := map[string]string{
		"security.insecure":      "true",
		"session.noReconnect":    "true",
		"session.timeout":        "1m",
		"session.requestTimeout": "1s",
		"session.dialTimeout":    "1s",
		"subscription.interval":  "500ms",
		"subscription.queueSize": "10",
	}

	for _, key := range SettableKeys {
		value, ok := values[key]
		if !ok {
			value = "value"
		}
		if err := (&Profile{}).Set(key, value); err != nil {
			t.Errorf("Set(%q, %q): %v", key, value, err)
		}
	}
}

func TestRedactedHidesThePassword(t *testing.T) {
	profile := &Profile{}
	profile.Auth.Password = "hunter2"

	if got := profile.Redacted().Auth.Password; got == "hunter2" {
		t.Error("Redacted returned the password in plain text")
	}
	if profile.Auth.Password != "hunter2" {
		t.Error("Redacted modified the original profile")
	}
}

// The configuration belongs in whichever per-user configuration directory the
// platform defines — %AppData% on Windows, where most OPC UA clients run — rather
// than in a location this tool invents.
func TestDirFollowsThePlatformConvention(t *testing.T) {
	parent, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("the platform reports no configuration directory: %v", err)
	}

	dir := Dir()
	if want := filepath.Join(parent, "opcua"); dir != want {
		t.Errorf("Dir = %q, want %q", dir, want)
	}

	// The certificates live beside the configuration.
	if got := CertDir(); got != filepath.Join(dir, "certs") {
		t.Errorf("CertDir = %q, want it beside the configuration", got)
	}
	if got := Path(""); got != filepath.Join(dir, "config.yaml") {
		t.Errorf("Path = %q, want it inside the configuration directory", got)
	}
}

func TestPathPrefersAnExplicitOverride(t *testing.T) {
	t.Setenv("OPCUA_CONFIG", "/from/env.yaml")

	if got := Path("/explicit.yaml"); got != "/explicit.yaml" {
		t.Errorf("Path with an override = %q, want the override", got)
	}
	if got := Path(""); got != "/from/env.yaml" {
		t.Errorf("Path with no override = %q, want the environment's", got)
	}

	t.Setenv("OPCUA_CONFIG", "")
	if got := Path(""); got != filepath.Join(Dir(), "config.yaml") {
		t.Errorf("Path = %q, want the platform's configuration directory", got)
	}
}
