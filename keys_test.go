package main

import (
	"os"
	"path/filepath"
	"testing"
)

var ids = []string{"herdr", "herdr.auto-title", "viewer", "houser.claude-usage"} // "herdr" first: the longest match must win, not the first

func TestAttribute(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		b    Binding
		want attribution
	}{
		{"plugin action", Binding{Type: "plugin_action", Command: "viewer.open"}, attribution{plugin: "viewer", action: "open"}},
		{"dotted id, longest wins", Binding{Type: "plugin_action", Command: "herdr.auto-title.open-settings"}, attribution{plugin: "herdr.auto-title", action: "open-settings"}},
		{"unknown plugin action", Binding{Type: "plugin_action", Command: "gone.open"}, attribution{}},
		{"shell pane open", Binding{Type: "shell", Command: `"$HERDR_BIN_PATH" plugin pane open --plugin houser.claude-usage --entrypoint usage`}, attribution{plugin: "houser.claude-usage", pane: "usage"}},
		{"equals and quotes", Binding{Type: "popup", Command: `herdr plugin pane open --plugin="viewer" --entrypoint='files'`}, attribution{plugin: "viewer", pane: "files"}},
		{"names no plugin", Binding{Type: "shell", Command: "/home/user/bin/usage-strip"}, attribution{}},
		{"names an unknown plugin", Binding{Type: "shell", Command: "herdr plugin pane open --plugin gone --entrypoint x"}, attribution{}},
	} {
		if got := attribute(c.b, ids); got != c.want {
			t.Errorf("%s: attribute = %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestKeyRows(t *testing.T) {
	t.Parallel()
	plugins := []Plugin{{
		ID: "viewer",
		Actions: []action{
			{ID: "open", Title: "Open"},
			{ID: "tab", Title: "Open in a tab"},
			{ID: "win", Title: "Windows only", Platforms: []string{"windows"}},
		},
		Panes: []pane{{ID: "files", Title: "Files"}, {ID: "log", Title: "Log"}},
	}}
	bindings := []Binding{
		{Key: "prefix+f", Type: "plugin_action", Command: "viewer.open"},
		{Key: "prefix+g", Type: "shell", Command: "herdr plugin pane open --plugin viewer --entrypoint files"},
		{Key: "prefix+u", Type: "shell", Command: "/home/user/bin/strip", Description: "usage strip"},
	}
	got := keyRows("viewer", plugins, bindings)
	want := []KeyRow{
		{ID: "key:0", Key: "prefix+f", What: "Open", Via: "plugin_action", Target: "action:open"},
		{ID: "key:1", Key: "prefix+g", What: "Files", Via: "shell", Target: "pane:files"},
		{ID: "action:tab", Key: "no key", What: "Open in a tab", Via: "action", Target: "action:tab"},
		{ID: "pane:log", Key: "no key", What: "Log", Via: "pane", Target: "pane:log"},
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %+v\nwant %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	other := keyRows(otherID, plugins, bindings)
	if len(other) != 1 || other[0].Key != "prefix+u" || other[0].What != "usage strip" || other[0].Target != "key:2" {
		t.Errorf("other keys = %+v", other)
	}
	if !unattributed(plugins, bindings) || unattributed(plugins, bindings[:2]) {
		t.Error("unattributed is wrong")
	}
}

func TestConfigPath(t *testing.T) { //nolint:paralleltest // sets the environment
	t.Setenv("HERDR_CONFIG_PATH", "/x/config.toml")
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	if p, _ := configPath(); p != "/x/config.toml" {
		t.Errorf("with HERDR_CONFIG_PATH: %s", p)
	}
	t.Setenv("HERDR_CONFIG_PATH", "")
	if p, _ := configPath(); p != "/xdg/herdr/config.toml" {
		t.Errorf("with XDG_CONFIG_HOME: %s", p)
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "/home/u")
	if p, _ := configPath(); p != "/home/u/.config/herdr/config.toml" {
		t.Errorf("fallback: %s", p)
	}
}

func TestReadBindings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if b, err := readBindings(filepath.Join(dir, "missing.toml")); err != nil || b != nil {
		t.Errorf("missing config: %v %v", b, err)
	}
	path := filepath.Join(dir, "config.toml")
	src := "[theme]\nname = \"x\"\n[keys]\nprefix = \"ctrl+b\"\n[[keys.command]]\nkey = \"prefix+f\"\ntype = \"plugin_action\"\ncommand = \"viewer.open\"\n"
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	b, err := readBindings(path)
	if err != nil || len(b) != 1 || b[0].Command != "viewer.open" {
		t.Errorf("bindings = %+v, %v", b, err)
	}
}
