# herdr-plugin-manager

A herdr plugin that lists your installed plugins and, for the one you
select, the keys that open its actions and panes.

```
┌Plugins─────────────────────────────────────────────────┐
│ Installed                                              │
│ ▸ herdr-file-viewer   smarzban/herdr-file-viewer   on  │
│   Auto Title          local                        on  │
│   Other keys          config.toml                      │
│   [e Enable] [d Disable] [u Uninstall]                 │
│                                                        │
│ Keys for herdr-file-viewer                             │
│ › prefix+f        Open file viewer        plugin_action│
│   prefix+shift+f  Open file viewer (tab)  plugin_action│
│   no key          Files                   pane         │
│   [r Run]                                              │
└────────────────────────────────────────────────────────┘
```

- **Keys** are the `[[keys.command]]` bindings in your herdr `config.toml`
  that open the plugin: a `plugin_action` binding, or a shell or popup
  binding whose command names the plugin with `--plugin`. They are followed
  by the plugin's actions and panes that have no key, so you can see what
  is there to bind. A binding no plugin can be found for is listed under
  **Other keys**, not dropped.
- **Enable / Disable** (`e`, `d`) turn a plugin on or off in herdr.
  Disabling asks first. The manager will not disable itself.
- **Uninstall** (`u`) removes a plugin from herdr, after asking. herdr
  deletes only a copy it cloned itself (a plugin installed from GitHub); a
  linked plugin's folder stays. Every plugin's settings and state stay, so
  installing or linking it again picks them up. The manager will not
  uninstall itself.
- **Run** (`r`) closes the panel, then runs the selected row: it invokes the
  action, opens the pane (it retries for a moment while herdr closes the
  panel's popup), or does what the key does. The panel cannot show what
  happened, so a failure comes as a herdr notification.

It reads herdr's `config.toml` and never writes it. It finds the file
through `HERDR_CONFIG_PATH`, then `$XDG_CONFIG_HOME/herdr`, then
`~/.config/herdr`.

## Install

```sh
herdr plugin link /path/to/herdr-plugin-manager   # after `make build`
herdr plugin action invoke komi.plugin-manager.open
```

Installing from GitHub (`herdr plugin install <owner>/herdr-plugin-manager`)
builds both binaries with Go 1.26. To bind a key, add this to herdr's
`config.toml` and run `herdr server reload-config`:

```toml
[[keys.command]]
key = "prefix+m"
type = "plugin_action"
command = "komi.plugin-manager.open"
description = "plugin manager"
```

## How it is built

The panel is drawn by [herdr-panel](https://github.com/AlexK-Notable/herdr-panel)
from `panel.toml`. The keys list follows the selected plugin with
`{sel.plugins.id}`, and Run uses `after = "close_then_run"`. Both features
came with herdr-panel v0.2.0, which `go.mod` pins. `herdr-plugin-manager`
supplies the rows and runs the actions. Run with no arguments, it prints its
usage and starts nothing.

`make check` vets, tests and validates the panel. The end-to-end test lives
in herdr-panel (`TestPluginManager`, opt-in through
`HERDR_PANEL_PLUGINMANAGER_DIR`). It drives this plugin in an isolated herdr.

MIT licensed.
