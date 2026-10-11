package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// herdr runs the herdr CLI: the one a plugin pane is told about in
// HERDR_BIN_PATH, else whichever is on PATH.
type herdr struct{ bin string }

func herdrBin() string {
	if b := os.Getenv("HERDR_BIN_PATH"); b != "" {
		return b
	}
	return "herdr"
}

func (h herdr) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, h.bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("herdr %s: %s", strings.Join(args, " "), errText(stderr.Bytes(), err))
	}
	return stdout.Bytes(), nil
}

// errText is herdr's error message: its JSON error on stderr when there is
// one, else the raw text.
func errText(stderr []byte, err error) string {
	var e struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(bytes.TrimSpace(stderr), &e) == nil && e.Error.Message != "" {
		if e.Error.Code != "" {
			return e.Error.Code + ": " + e.Error.Message
		}
		return e.Error.Message
	}
	if s := strings.TrimSpace(string(stderr)); s != "" {
		return s
	}
	return err.Error()
}

func (h herdr) plugins(ctx context.Context) ([]Plugin, error) {
	out, err := h.run(ctx, "plugin", "list", "--json")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Result struct {
			Plugins []Plugin `json:"plugins"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return nil, fmt.Errorf("reading herdr's plugin list: %w", err)
	}
	return resp.Result.Plugins, nil
}

// busyTries bounds how long openPane waits for another popup to close.
var busyTries, busyWait = 20, 100 * time.Millisecond

// openPane opens a plugin pane where its manifest places it. herdr allows
// one popup at a time and may still be closing the manager's, so a ui_busy
// refusal is retried for a couple of seconds.
func (h herdr) openPane(ctx context.Context, pluginID, entrypoint string) error {
	var err error
	for range busyTries {
		_, err = h.run(ctx, "plugin", "pane", "open", "--plugin", pluginID, "--entrypoint", entrypoint, "--focus")
		if err == nil || !strings.Contains(err.Error(), "ui_busy") {
			return err
		}
		time.Sleep(busyWait)
	}
	return err
}

func (h herdr) notify(ctx context.Context, title, body string) {
	_, _ = h.run(ctx, "notification", "show", title, "--body", body)
}
