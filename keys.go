package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Plugin is one entry of `herdr plugin list --json`, as far as the panel
// uses it.
type Plugin struct {
	ID      string   `json:"plugin_id"`
	Name    string   `json:"name"`
	Enabled bool     `json:"enabled"`
	Source  source   `json:"source"`
	Actions []action `json:"actions"`
	Panes   []pane   `json:"panes"`
}

type source struct {
	Kind  string `json:"kind"`
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
}

type action struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Platforms []string `json:"platforms"`
}

type pane struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Placement string `json:"placement"`
}

// Binding is one [[keys.command]] of herdr's config.toml.
type Binding struct {
	Key         string `toml:"key"`
	Type        string `toml:"type"`
	Command     string `toml:"command"`
	Description string `toml:"description"`
}

// KeyRow is one row of the keys list.
type KeyRow struct {
	ID     string `json:"id"`
	Key    string `json:"key"`
	What   string `json:"what"`
	Via    string `json:"via"`
	Target string `json:"target"` // what `run` takes: action:<id>, pane:<id>, key:<n>, or ""
}

// otherID is the plugins list's row for bindings no plugin can be found for.
const otherID = "_other"

// configPath is herdr's config.toml: HERDR_CONFIG_PATH, else the XDG
// config directory, else ~/.config.
func configPath() (string, error) {
	if p := os.Getenv("HERDR_CONFIG_PATH"); p != "" {
		return p, nil
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "herdr", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("finding herdr's config: %w", err)
	}
	return filepath.Join(home, ".config", "herdr", "config.toml"), nil
}

// readBindings reads the [[keys.command]] entries. A missing config has none.
func readBindings(path string) ([]Binding, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Keys struct {
			Command []Binding `toml:"command"`
		} `toml:"keys"`
	}
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return cfg.Keys.Command, nil
}

var (
	pluginFlag     = regexp.MustCompile(`--plugin[= ]+["']?([^"'\s]+)`)
	entrypointFlag = regexp.MustCompile(`--entrypoint[= ]+["']?([^"'\s]+)`)
)

// attribution says which plugin a binding belongs to, and what it opens:
// an action, a pane, or neither ("" target: some other command).
type attribution struct {
	plugin string
	action string
	pane   string
}

// attribute finds the plugin a binding is for. A plugin_action binding names
// "<plugin id>.<action id>", and plugin ids may hold dots themselves, so the
// longest installed id that prefixes it wins. A shell or popup binding is
// the plugin's when its command names it with --plugin.
func attribute(b Binding, ids []string) attribution {
	if b.Type == "plugin_action" {
		best := ""
		for _, id := range ids {
			if strings.HasPrefix(b.Command, id+".") && len(id) > len(best) {
				best = id
			}
		}
		if best != "" {
			return attribution{plugin: best, action: strings.TrimPrefix(b.Command, best+".")}
		}
		return attribution{}
	}
	m := pluginFlag.FindStringSubmatch(b.Command)
	if m == nil || !slices.Contains(ids, m[1]) {
		return attribution{}
	}
	a := attribution{plugin: m[1]}
	if e := entrypointFlag.FindStringSubmatch(b.Command); e != nil {
		a.pane = e[1]
	}
	return a
}

// onThisPlatform reports whether an action is offered here.
func onThisPlatform(platforms []string) bool {
	if len(platforms) == 0 {
		return true
	}
	here := runtime.GOOS
	if here == "darwin" {
		here = "macos"
	}
	return slices.Contains(platforms, here)
}

// keyRows are the keys list for one plugin: the bindings that open it, then
// its actions and panes that have none. For otherID, the bindings no plugin
// was found for.
func keyRows(pluginID string, plugins []Plugin, bindings []Binding) []KeyRow {
	ids := make([]string, 0, len(plugins))
	for _, p := range plugins {
		ids = append(ids, p.ID)
	}
	var p *Plugin
	for i := range plugins {
		if plugins[i].ID == pluginID {
			p = &plugins[i]
		}
	}
	rows := []KeyRow{}
	boundActions, boundPanes := map[string]bool{}, map[string]bool{}
	for n, b := range bindings {
		a := attribute(b, ids)
		mine := a.plugin == pluginID || (pluginID == otherID && a.plugin == "")
		if !mine {
			continue
		}
		row := KeyRow{ID: fmt.Sprintf("key:%d", n), Key: b.Key, Via: b.Type, Target: fmt.Sprintf("key:%d", n)}
		switch {
		case a.action != "":
			boundActions[a.action] = true
			row.What = actionTitle(p, a.action)
			row.Target = "action:" + a.action
		case a.pane != "":
			boundPanes[a.pane] = true
			row.What = paneTitle(p, a.pane)
			row.Target = "pane:" + a.pane
		default:
			row.What = orDefault(b.Description, b.Command)
		}
		rows = append(rows, row)
	}
	if p == nil {
		return rows
	}
	for _, a := range p.Actions {
		if !boundActions[a.ID] && onThisPlatform(a.Platforms) {
			rows = append(rows, KeyRow{ID: "action:" + a.ID, Key: "no key", What: orDefault(a.Title, a.ID), Via: "action", Target: "action:" + a.ID})
		}
	}
	for _, pn := range p.Panes {
		if !boundPanes[pn.ID] {
			rows = append(rows, KeyRow{ID: "pane:" + pn.ID, Key: "no key", What: orDefault(pn.Title, pn.ID), Via: "pane", Target: "pane:" + pn.ID})
		}
	}
	return rows
}

// unattributed reports whether some command binding belongs to no installed
// plugin; the panel lists those under "Other keys" rather than drop them.
func unattributed(plugins []Plugin, bindings []Binding) bool {
	return len(keyRows(otherID, plugins, bindings)) > 0
}

func actionTitle(p *Plugin, id string) string {
	if p != nil {
		for _, a := range p.Actions {
			if a.ID == id {
				return orDefault(a.Title, id)
			}
		}
	}
	return id
}

func paneTitle(p *Plugin, id string) string {
	if p != nil {
		for _, pn := range p.Panes {
			if pn.ID == id {
				return orDefault(pn.Title, id)
			}
		}
	}
	return id
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}
