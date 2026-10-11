// herdr-plugin-manager lists the installed herdr plugins and, for the one
// selected, the keys that open its actions and panes. The panel itself is
// herdr-panel's (panel.toml); this program supplies its rows and runs its
// actions.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// selfID is this plugin's id: it never disables itself.
const selfID = "komi.plugin-manager"

const usage = `usage: herdr-plugin-manager <command>
  plugins              the installed plugins, as rows (JSON)
  keys <plugin>        a plugin's keys, actions and panes, as rows (JSON)
  enable <plugin>      enable a plugin
  disable <plugin>     disable a plugin
  run <plugin> <what>  run an action:<id>, open a pane:<id>, or press key:<n>
  open                 open the manager's popup`

func main() {
	os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr))
}

func dispatch(args []string, stdout, stderr io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	h := herdr{bin: herdrBin()}
	switch {
	case len(args) == 1 && args[0] == "plugins":
		return rowsOut(stdout, stderr, func() (any, error) { return pluginRows(ctx, h) })
	case len(args) == 2 && args[0] == "keys":
		return rowsOut(stdout, stderr, func() (any, error) { return keysFor(ctx, h, args[1]) })
	case len(args) == 2 && (args[0] == "enable" || args[0] == "disable"):
		msg, err := setEnabled(ctx, h, args[1], args[0] == "enable")
		return result(stdout, msg, err)
	case len(args) == 3 && args[0] == "run":
		// Runs after the panel has closed: nobody sees its output, so a
		// failure becomes a herdr notification.
		msg, err := runTarget(ctx, h, args[1], args[2])
		if err != nil {
			h.notify(ctx, "Plugins: could not run it", err.Error())
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintln(stdout, msg)
		return 0
	case len(args) == 1 && args[0] == "open":
		if err := h.openPane(ctx, selfID, "manager"); err != nil {
			h.notify(ctx, "Plugins", err.Error())
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	default:
		// Never start anything when run bare: print what it takes.
		fmt.Fprintln(stderr, usage)
		return 2
	}
}

func rowsOut(stdout, stderr io.Writer, f func() (any, error)) int {
	v, err := f()
	if err != nil {
		_ = json.NewEncoder(stdout).Encode(map[string]any{"ok": false, "message": err.Error()})
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := json.NewEncoder(stdout).Encode(v); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// result prints herdr-panel's {"ok", "message"} contract.
func result(stdout io.Writer, msg string, err error) int {
	if err != nil {
		_ = json.NewEncoder(stdout).Encode(map[string]any{"ok": false, "message": err.Error()})
		return 1
	}
	_ = json.NewEncoder(stdout).Encode(map[string]any{"ok": true, "message": msg})
	return 0
}

// PluginRow is one row of the plugins list.
type PluginRow struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Source      string `json:"source"`
	State       string `json:"state"`
	DisableNote string `json:"disable_note"`
}

func pluginRows(ctx context.Context, h herdr) ([]PluginRow, error) {
	plugins, err := h.plugins(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]PluginRow, 0, len(plugins)+1)
	for _, p := range plugins {
		r := PluginRow{ID: p.ID, Name: orDefault(p.Name, p.ID), Source: sourceText(p.Source), State: "off",
			DisableNote: "Its actions, panes and keys stop working until it is enabled again."}
		if p.Enabled {
			r.State = "on"
		}
		if p.ID == selfID {
			r.DisableNote = "This is the plugin manager itself; it will not disable itself."
		}
		rows = append(rows, r)
	}
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	bindings, err := readBindings(path)
	if err != nil {
		return nil, err
	}
	if unattributed(plugins, bindings) {
		rows = append(rows, PluginRow{ID: otherID, Name: "Other keys", Source: "config.toml", State: "",
			DisableNote: "These are keys, not a plugin."})
	}
	return rows, nil
}

func sourceText(s source) string {
	switch {
	case s.Kind == "github" && s.Owner != "":
		return s.Owner + "/" + s.Repo
	case s.Kind != "":
		return s.Kind
	}
	return ""
}

func keysFor(ctx context.Context, h herdr, pluginID string) ([]KeyRow, error) {
	plugins, err := h.plugins(ctx)
	if err != nil {
		return nil, err
	}
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	bindings, err := readBindings(path)
	if err != nil {
		return nil, err
	}
	return keyRows(pluginID, plugins, bindings), nil
}

func setEnabled(ctx context.Context, h herdr, id string, on bool) (string, error) {
	switch {
	case id == otherID:
		return "", errors.New("these are keys, not a plugin")
	case id == selfID && !on:
		return "", errors.New("the plugin manager will not disable itself")
	}
	verb, done := "disable", "disabled"
	if on {
		verb, done = "enable", "enabled"
	}
	if _, err := h.run(ctx, "plugin", verb, id); err != nil {
		return "", err
	}
	return done + " " + id, nil
}

// runTarget runs what a keys row names. It starts once the panel has exited,
// but herdr may still be closing its popup, so opening a pane retries a
// refusal for a moment.
func runTarget(ctx context.Context, h herdr, pluginID, target string) (string, error) {
	kind, id, ok := strings.Cut(target, ":")
	if !ok || id == "" {
		return "", fmt.Errorf("nothing to run for %q", target)
	}
	switch kind {
	case "action":
		if _, err := h.run(ctx, "plugin", "action", "invoke", pluginID+"."+id); err != nil {
			return "", err
		}
		return "invoked " + pluginID + "." + id, nil
	case "pane":
		if err := h.openPane(ctx, pluginID, id); err != nil {
			return "", err
		}
		return "opened " + pluginID + " " + id, nil
	case "key":
		n, err := strconv.Atoi(id)
		if err != nil {
			return "", fmt.Errorf("nothing to run for %q", target)
		}
		path, err := configPath()
		if err != nil {
			return "", err
		}
		bindings, err := readBindings(path)
		if err != nil {
			return "", err
		}
		if n < 0 || n >= len(bindings) {
			return "", errors.New("that key is no longer in config.toml")
		}
		return pressKey(ctx, h, bindings[n])
	}
	return "", fmt.Errorf("nothing to run for %q", target)
}

// pressKey does what a binding does when its key is pressed.
func pressKey(ctx context.Context, h herdr, b Binding) (string, error) {
	switch b.Type {
	case "plugin_action":
		if _, err := h.run(ctx, "plugin", "action", "invoke", b.Command); err != nil {
			return "", err
		}
	case "shell":
		// The user's own command from their own config, run as herdr runs a
		// shell binding.
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", b.Command)
		cmd.Env = append(os.Environ(), "HERDR_BIN_PATH="+h.bin)
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("%s: %v: %s", b.Key, err, strings.TrimSpace(string(out)))
		}
	default:
		return "", fmt.Errorf("%s is a %s binding, which only herdr can run: press the key", b.Key, b.Type)
	}
	return "ran " + b.Key, nil
}
