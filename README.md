# Sticky Notes — desktop

Markdown sticky notes that float above your other windows.

- **Markdown:** notes are written in Markdown and displayed rendered. Click a `- [ ]` task to cross it off.
- **Dots:** click a note to show its dots: 🎨 color · ✏️ edit · 👁 hide · 📌 anchor. An anchored note can't be dragged or resized.
- **Move and resize:** drag a note anywhere; resize it from its edges or bottom-right corner.
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

`nix develop .#gtk3` is a GTK 3 / WebKit2GTK 4.1 shell for building the Ubuntu 22.04 variant (see below).

## Ubuntu (and other distros)

The app needs **Go 1.26+** (from go.dev, not apt) and **Node 20+** (e.g. NodeSource or nvm).

**Ubuntu 24.04 and newer** (GTK 4, the default build):

```sh
sudo apt install build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.26
wails3 build                         # → bin/notes-desktop
wails3 task linux:create:deb         # → bin/*.deb (optional)
```

**Ubuntu 22.04 / Debian 12** (no WebKitGTK 6.0, so build the GTK 3 variant):

```sh
sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
wails3 build EXTRA_TAGS=gtk3
wails3 task linux:create:deb:gtk3    # deb that depends on GTK 3 packages
```

Wails plans to drop GTK 3 in v3.1, so the 22.04 build only works while this project stays on Wails 3.0.x.

**What to expect on Ubuntu:**
- **Tray icon:** it shows up, because Ubuntu turns on its AppIndicator extension by default. Plain GNOME needs the "AppIndicator and KStatusNotifierItem Support" extension.
- **Staying on top:** GNOME on Wayland doesn't let apps keep windows on top or pick their position. So on any Wayland desktop other than niri, the app automatically runs through Xwayland, where both work. Text may look slightly soft with fractional scaling; set `NOTES_NATIVE_WAYLAND=1` to use native Wayland instead (notes then act as normal windows).
- **"Ubuntu on Xorg" session:** everything works natively.

Other distros: install the GTK 4 and WebKitGTK 6.0 development packages (e.g. `gtk4-devel webkitgtk6.0-devel` on Fedora) and run `wails3 doctor`.

### Useful environment variables

| Variable | Purpose |
|---|---|
| `NOTES_DATA_DIR` | Use a different data directory (default `~/.config/notes-desktop`). Each directory is its own instance, which is handy for testing sync between two "devices" on one machine. |
| `NOTES_NATIVE_WAYLAND=1` | Don't switch to Xwayland on non-niri Wayland desktops. |
| `WEBKIT_DISABLE_DMABUF_RENDERER=1` | Try this if windows render blank on some GPU drivers (set automatically on NVIDIA). |

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
- **Other Wayland desktops (GNOME, KDE, …):** the app runs through Xwayland, where always-on-top and positioning work (see `platform_linux.go`).
- **X11 sessions:** the normal always-on-top and move calls are used.

Notes are resized by dragging their edges or bottom-right corner, using the compositor's native resize. Anchoring a note locks both moving and resizing.

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
