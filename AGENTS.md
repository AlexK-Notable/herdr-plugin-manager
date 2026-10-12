# AGENTS.md

herdr-plugin-manager is a [herdr](https://herdr.dev) plugin. Its popup lists
the installed plugins and, for the one selected, the keys that open its
actions and panes. The panel is drawn by
[herdr-panel](https://github.com/AlexK-Notable/herdr-panel) from
`panel.toml`. This repository's Go program, `herdr-plugin-manager`, only
supplies the panel's rows and carries out its actions. Read
[`README.md`](./README.md) first; this file holds the rules for working here.

## Layout

```
main.go          subcommands: plugins, keys, enable, disable, run, open
keys.go          herdr's config.toml bindings, and which plugin each belongs to
herdr.go         the herdr CLI: plugin list, pane open with the ui_busy retry, notifications
panel.toml       the panel (herdr-panel spec 1, requires >= 0.2)
herdr-plugin.toml  the manifest: two [[build]] steps, the popup pane, the open action
```

`go.mod` pins herdr-panel as a tool. The second `[[build]]` step builds
that pinned version into `bin/herdr-panel`.

## Hard rules

1. **Never write herdr's `config.toml`.** The manager reads it to find the
   key bindings, and that is all. Binding a key is the user's edit.
2. **Never touch a live herdr session in tests or experiments.** The
   end-to-end test is `TestPluginManager` in herdr-panel's `integration/`
   module. It runs this plugin in an isolated herdr, where every XDG
   directory is redirected and every `HERDR_*` variable is removed. Run it
   there, with `HERDR_PANEL_PLUGINMANAGER_DIR` set to this checkout after
   `make build`.
3. **Run with no arguments, the program prints its usage and exits 2.**
   It never starts anything when called bare. Keep it that way: a smoke
   test that runs a plugin binary bare must not set something off.
4. **Run's result is never seen.** `after = "close_then_run"` runs it after
   the panel has closed. A failure must reach the user some other way: a
   herdr notification.
5. **The manager never disables itself.** `selfID` must match the
   manifest's `id`.
6. **Commands never go through a shell**, with one exception: pressing a
   `shell` binding runs that binding's own command from the user's config,
   exactly as herdr would.

## Working here

- `make check` builds, vets, runs the tests with `-race`, and validates
  `panel.toml` with the pinned herdr-panel.
- To build against a herdr-panel checkout instead of the pin, run
  `go build -o bin/herdr-panel` from that checkout. Never commit a
  `replace` directive.
- A new herdr-panel feature that `panel.toml` uses means raising both the
  pin and `requires`.
- Commits: present tense, saying what changed and why. MIT; contributions
  are inbound = outbound.
- `main` is protected: force-pushes and deleting it are refused, and a pull
  request merges only when every CI job passes (an admin may still push
  directly). The required checks are the CI job names. Renaming or adding a
  job means updating the protection's list (`gh api
  repos/AlexK-Notable/herdr-plugin-manager/branches/main/protection`), or
  pull requests wait on a check that never reports.
