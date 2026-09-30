# Sticky Notes — desktop

Markdown sticky notes that float above your other windows.

- **Markdown:** notes are written in Markdown and displayed rendered. Click a `- [ ]` task to cross it off.
- **Dots:** click a note to show its dots: 🎨 color · ✏️ edit · 👁 hide · 📌 anchor. An anchored note can't be dragged or resized.
- **Move and resize:** drag a note anywhere; resize it from the bottom-right corner.
- **Tray icon:** opens the manager (search, show/hide, delete) and the sync settings.
- **Encryption:** note content and the sync token are encrypted in the local database with AES-256-GCM. The key lives in your OS keyring (Secret Service / Keychain / Credential Manager).
- **Sync:** optional, through a self-hosted [`notes-server`](../server), with optional end-to-end encryption.

Built with [Wails v3](https://v3.wails.io) (Go + Svelte/TypeScript).

## Develop (NixOS / Nix)

```sh
nix develop                     # go, node, gtk4, webkitgtk 6, pkg-config
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.26
wails3 dev                      # hot reload
wails3 build                    # → bin/notes-desktop
go test ./... && (cd frontend && npm test)
```

The Go tests include an end-to-end sync test. It builds and runs `../server`, so keep both repos side by side in `notes_app/`.

On other distros, install the GTK4 and WebKitGTK 6.0 development packages and run `wails3 doctor`.

### Useful environment variables

| Variable | Purpose |
|---|---|
| `NOTES_DATA_DIR` | Use a different data directory (default `~/.config/notes-desktop`). Each directory is its own instance, which is handy for testing sync between two "devices" on one machine. |
| `WEBKIT_DISABLE_DMABUF_RENDERER=1` | Try this if windows render blank on some GPU drivers. |

## How it works

| Piece | Where |
|---|---|
| Note windows (frameless, always on top), niri placement | `windows.go` |
| Frontend API: create/edit/hide/anchor/delete | `notes.go` |
| Sync loop, E2E encode/decode | `sync.go` |
| Sync settings, test connection, E2E on/off | `settings.go` |
| Encrypted local SQLite store | `internal/store` |
| Keyring key, AES-GCM, Argon2id | `internal/secure` |
| niri IPC client | `internal/niri` |
| Note UI, Markdown/task rendering, manager UI | `frontend/src/NoteView.svelte`, `frontend/src/lib/markdown.ts`, `frontend/src/Manager.svelte` |

Only `content` and `color` sync. Position, size, anchored and hidden stay local to each device.

## Linux / Wayland notes

Wayland doesn't let apps keep their windows on top or choose where they go:
- **niri:** the app talks to niri's IPC socket. Notes go on the floating layer, which sits above tiled windows. Their positions are restored and saved when you drag them.
- **X11 sessions:** the normal always-on-top and move calls are used.
- **Other Wayland compositors:** notes open as normal windows.

Optional niri window rule to remove the focus ring and border from notes (add it to `~/.config/niri/config.kdl`):

```kdl
window-rule {
    match app-id=r#"^org\.wails\.sticky_notes$"# title="^Sticky note "
    focus-ring { off; }
    border { off; }
    shadow { on; }
}
```

Limitations on niri:
- Floating windows belong to one workspace, so a note stays on the workspace it was opened on.
- A note may flash in the tiled layout for a split second before it floats.

The tray icon needs a StatusNotifierItem host, e.g. waybar's `tray` module.

## Sync setup

1. On the server: `notes-server token create --name my-laptop`
2. Tray → **Manage notes…** → **Sync**: enter the host/IP, port and token → **Test connection** → tick **Enable sync** → **Save**.
3. Optional: under *End-to-end encryption*, set a passphrase. Enter the same passphrase on your other devices.
