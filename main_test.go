package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeHerdr is a herdr CLI that logs every call and answers from files: a
// plugin list, and a count of ui_busy refusals to give before a pane opens.
func fakeHerdr(t *testing.T, busy int) (logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "calls")
	list := `{"result":{"plugins":[` +
		`{"plugin_id":"viewer","name":"Viewer","enabled":true,"source":{"kind":"github","owner":"o","repo":"r"},"actions":[{"id":"open","title":"Open"}],"panes":[{"id":"files","title":"Files"}]},` +
		`{"plugin_id":"komi.plugin-manager","name":"Plugin Manager","enabled":true,"source":{"kind":"local"}}]}}`
	if err := os.WriteFile(filepath.Join(dir, "list.json"), []byte(list), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "busy"), []byte(strings.Repeat("x", busy)), 0o600); err != nil {
		t.Fatal(err)
	}
	script := `#!/bin/sh
d=$(dirname "$0")
echo "$*" >> "` + logPath + `"
case "$*" in
  "plugin list --json") cat "$d/list.json" ;;
  "plugin pane open"*)
    n=$(wc -c < "$d/busy")
    if [ "$n" -gt 0 ]; then
      head -c $((n-1)) "$d/busy" > "$d/busy.new"; mv "$d/busy.new" "$d/busy"
      echo '{"error":{"code":"ui_busy","message":"a popup is open"}}' >&2; exit 1
    fi ;;
  "plugin action invoke gone."*) echo '{"error":{"code":"not_found","message":"no such action"}}' >&2; exit 1 ;;
esac
`
	bin := filepath.Join(dir, "herdr")
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil { //nolint:gosec // a test executable
		t.Fatal(err)
	}
	t.Setenv("HERDR_BIN_PATH", bin)
	cfg := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(cfg, []byte("[[keys.command]]\nkey = \"prefix+f\"\ntype = \"plugin_action\"\ncommand = \"viewer.open\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HERDR_CONFIG_PATH", cfg)
	return logPath
}

func calls(t *testing.T, logPath string) []string {
	t.Helper()
	data, _ := os.ReadFile(logPath)
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func do(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := dispatch(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestBareRunPrintsUsage(t *testing.T) { //nolint:paralleltest // sets the environment
	log := fakeHerdr(t, 0)
	code, _, stderr := do()
	if code != 2 || !strings.Contains(stderr, "usage:") {
		t.Errorf("bare: code %d, stderr %q", code, stderr)
	}
	if data, _ := os.ReadFile(log); len(data) != 0 {
		t.Errorf("a bare run called herdr: %s", data)
	}
}

func TestPluginsAndKeys(t *testing.T) { //nolint:paralleltest // sets the environment
	fakeHerdr(t, 0)
	code, out, _ := do("plugins")
	var rows []PluginRow
	if code != 0 || json.Unmarshal([]byte(out), &rows) != nil || len(rows) != 2 || rows[0].Source != "o/r" || rows[0].State != "on" {
		t.Fatalf("plugins: %d %s", code, out)
	}
	code, out, _ = do("keys", "viewer")
	var keys []KeyRow
	if code != 0 || json.Unmarshal([]byte(out), &keys) != nil || len(keys) != 2 || keys[0].Key != "prefix+f" || keys[1].Target != "pane:files" {
		t.Fatalf("keys: %d %s", code, out)
	}
}

func TestDisable(t *testing.T) { //nolint:paralleltest // sets the environment
	log := fakeHerdr(t, 0)
	code, out, _ := do("disable", "komi.plugin-manager")
	if code == 0 || !strings.Contains(out, `"ok":false`) || !strings.Contains(out, "will not disable itself") {
		t.Errorf("self-disable: %d %s", code, out)
	}
	if code, out, _ = do("disable", "viewer"); code != 0 || !strings.Contains(out, `"ok":true`) {
		t.Errorf("disable: %d %s", code, out)
	}
	if got := calls(t, log); got[len(got)-1] != "plugin disable viewer" {
		t.Errorf("calls %q", got)
	}
	for _, c := range calls(t, log) {
		if strings.Contains(c, "komi.plugin-manager") {
			t.Errorf("herdr was asked to touch the manager: %q", c)
		}
	}
}

func TestRunPaneRetriesWhileAPopupCloses(t *testing.T) { //nolint:paralleltest // sets globals and the environment
	log := fakeHerdr(t, 3)
	defer func(n int, d time.Duration) { busyTries, busyWait = n, d }(busyTries, busyWait)
	busyWait = time.Millisecond
	code, out, stderr := do("run", "viewer", "pane:files")
	if code != 0 || !strings.Contains(out, "opened viewer files") {
		t.Fatalf("run: %d %q %q", code, out, stderr)
	}
	opens := 0
	for _, c := range calls(t, log) {
		if strings.HasPrefix(c, "plugin pane open --plugin viewer --entrypoint files") {
			opens++
		}
	}
	if opens != 4 {
		t.Errorf("%d opens, want 3 refused and 1 that worked", opens)
	}
}

func TestRunFailureNotifies(t *testing.T) { //nolint:paralleltest // sets the environment
	log := fakeHerdr(t, 0)
	code, _, _ := do("run", "gone", "action:open")
	if code == 0 {
		t.Fatal("a failed run reported success")
	}
	got := calls(t, log)
	if last := got[len(got)-1]; !strings.HasPrefix(last, "notification show Plugins: could not run it --body") || !strings.Contains(last, "no such action") {
		t.Errorf("no notification: %q", got)
	}
}

func TestUninstall(t *testing.T) { //nolint:paralleltest // sets the environment
	log := fakeHerdr(t, 0)
	for _, id := range []string{"komi.plugin-manager", otherID} {
		if code, out, _ := do("uninstall", id); code == 0 || !strings.Contains(out, `"ok":false`) {
			t.Errorf("uninstall %s: %d %s", id, code, out)
		}
	}
	if code, out, _ := do("uninstall", "viewer"); code != 0 || !strings.Contains(out, `"ok":true`) {
		t.Errorf("uninstall: %d %s", code, out)
	}
	got := calls(t, log)
	if got[len(got)-1] != "plugin uninstall viewer" {
		t.Errorf("calls %q", got)
	}
	for _, c := range got {
		if strings.Contains(c, "komi.plugin-manager") || strings.Contains(c, otherID) {
			t.Errorf("herdr was asked to uninstall what it must not: %q", c)
		}
	}
}

func TestUninstallNotes(t *testing.T) { //nolint:paralleltest // sets the environment
	fakeHerdr(t, 0)
	_, out, _ := do("plugins")
	var rows []PluginRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatal(err)
	}
	notes := map[string]string{}
	for _, r := range rows {
		notes[r.ID] = r.UninstallNote
	}
	if n := notes["viewer"]; !strings.Contains(n, "deletes its copy of o/r") || !strings.Contains(n, "settings stay") {
		t.Errorf("installed plugin note %q", n)
	}
	if n := notes["komi.plugin-manager"]; !strings.Contains(n, "will not uninstall itself") {
		t.Errorf("self note %q", n)
	}
}
