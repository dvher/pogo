# 🟨 Pogo

**Sticky notes that bounce between your devices.**

Pogo (**Po**st-it + **Go**) puts small Markdown notes on your desktop, above your other windows. Drag
them where you want, anchor them in place, and tick off tasks as you go. When you want your notes
on another computer, Pogo syncs through **[Pogo Pad](https://github.com/dvher/pogo_pad)**, a tiny server you host yourself,
with optional end-to-end encryption.

<!-- screenshot: three notes (yellow, pink, blue) floating over an editor -->

## Features

- **Always on top.** Notes float above your windows. They're frameless and quiet, and you can place as many as you like.
- **Markdown.** Write in Markdown and see it rendered. Click a `- [ ]` task to cross it off.
- **One-click controls.** Click a note to show its dots: 🎨 color · ✏️ edit · 👁 hide · 📌 anchor.
- **Anchor.** An anchored note stays put: no accidental drags or resizes.
- **Move and resize.** Drag a note anywhere; resize it from its edges or bottom-right corner.
- **Tray icon.** Search, show, hide and delete notes from one small manager window.
- **Private by default.** Notes and your sync token are encrypted on disk with AES-256-GCM, using a key kept
  in your OS keyring.
- **Self-hosted sync.** Pogo Pad runs on a home server, a Raspberry Pi or a VPS. With end-to-end
  encryption turned on, the server only ever stores unreadable data.

## Install

| Platform | How |
|---|---|
| Ubuntu 24.04+, Debian 13+ | [Build from source](#build-from-source), or build a `.deb` with `wails3 task linux:create:deb` |
| Ubuntu 22.04, Debian 12 | [GTK 3 build](#ubuntu-2204--debian-12) |
| Fedora, Arch, NixOS | [Build from source](#build-from-source) |
| Windows, macOS | Coming soon |
| Android | [Pogo Pocket](https://github.com/dvher/pogo_pocket), a home-screen widget that syncs through the same Pogo Pad |
| iOS | Planned (Pogo Pocket) |

## Sync with Pogo Pad

1. Start Pogo Pad on any machine your devices can reach:
   ```sh
   docker compose up -d
   docker compose exec pogo-pad pogo-pad token create --name my-laptop
   ```
2. In Pogo, go to the tray icon → **Manage notes…** → **Sync**. Enter the server's IP address or hostname,
   its port and the token, then click **Test connection**, tick **Enable sync** and click **Save**.
3. Optional: set an **end-to-end passphrase** under *End-to-end encryption*, and enter the same one on
   your other devices.

Only note text and color sync. Where a note sits on your screen, its size, and whether it's anchored or
hidden stay on each device.
Notes that arrive from another device start hidden: show them from **Manage notes…** when you want them
on screen.

## Privacy, in short

- **On your computer:** notes and your sync token are encrypted. The key lives in your OS keyring
  (GNOME Keyring, KWallet, macOS Keychain or Windows Credential Manager). If no keyring is available,
  it's kept in a file only you can read.
- **On Pogo Pad without E2E:** the server can read note text. Put it behind HTTPS if it's reachable
  outside your home network.
- **On Pogo Pad with E2E:** notes are encrypted before they leave your device, with a key derived
  from your passphrase (Argon2id). If you lose the passphrase, the notes on the server can't be recovered.
  Your local copies are unaffected.

## Build from source

You need **Go 1.26+** (from go.dev, not apt), **Node 20+** and the Wails 3 CLI:

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.26
```

### Ubuntu 24.04+ (GTK 4, the default build)

```sh
sudo apt install build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev
wails3 build                         # → bin/pogo
wails3 task linux:create:deb         # → bin/*.deb (optional)
```

Ubuntu 23.10+ only lets apps create user namespaces if AppArmor allows it, and WebKit's sandbox needs
one. The `.deb` installs a profile for `/usr/local/bin/pogo` (`build/linux/apparmor/pogo`). To run
`bin/pogo` directly, either copy that profile with the path changed into `/etc/apparmor.d/` and load it
with `sudo apparmor_parser -r`, or start it with `WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1`.

### Ubuntu 22.04 / Debian 12

These don't have WebKitGTK 6.0, so build the GTK 3 variant:

```sh
sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
wails3 build EXTRA_TAGS=gtk3
wails3 task linux:create:deb:gtk3    # .deb that depends on the GTK 3 packages
```

Wails plans to drop GTK 3 in v3.1, so the 22.04 build only works while Pogo stays on Wails 3.0.x.

### Nix

```sh
nix develop            # GTK 4 shell
nix develop .#gtk3     # GTK 3 shell for the 22.04 variant
wails3 build
```

### Other distros

Install the GTK 4 and WebKitGTK 6.0 development packages (e.g. `gtk4-devel webkitgtk6.0-devel` on Fedora)
and run `wails3 doctor`.

## Linux desktops

Wayland doesn't let apps keep their windows on top or choose where they go, so Pogo adapts:

| Desktop | Notes stay on top | Position remembered |
|---|---|---|
| niri | ✅ floating layer, above tiled windows (via niri IPC) | ✅ |
| GNOME, KDE and other Wayland desktops | ✅ via Xwayland | ✅ |
| Any X11 session, including "Ubuntu on Xorg" | ✅ | ✅ |

- **Xwayland:** text may look slightly soft with fractional scaling. Set `POGO_NATIVE_WAYLAND=1` to
  use native Wayland instead; notes then act as normal windows.
- **Tray icon:** Ubuntu shows it out of the box. Plain GNOME needs the "AppIndicator and KStatusNotifierItem
  Support" extension, and niri needs a bar with a tray, such as waybar's `tray` module.
- **niri:** a note stays on the workspace it was opened on, and may flash in the tiled layout for a
  split second before it floats. This optional rule for `~/.config/niri/config.kdl` removes the focus
  ring and border from notes:

  ```kdl
  window-rule {
      match app-id=r#"^org\.wails\.pogo$"# title="^Pogo note "
      focus-ring { off; }
      border { off; }
      shadow { on; }
  }
  ```

## Development

```sh
nix develop
wails3 dev                                  # hot reload
go test ./... && (cd frontend && npm test)
```

The Go tests include an end-to-end sync test that builds and runs Pogo Pad. It looks for the server
repo next to this one, as `../server` or `../pogo_pad`, and is skipped if neither exists.

| Environment variable | Purpose |
|---|---|
| `POGO_DATA_DIR` | Use a different data folder (default `~/.config/pogo`). Each folder is a separate instance, which is handy for testing sync between two "devices" on one machine. |
| `POGO_NATIVE_WAYLAND=1` | Don't switch to Xwayland on Wayland desktops other than niri. |
| `WEBKIT_DISABLE_DMABUF_RENDERER=1` | Try this if windows render blank on some GPU drivers (set automatically on NVIDIA). |

### Where things live

| Piece | Where |
|---|---|
| Note windows (frameless, always on top), niri placement | `windows.go` |
| Xwayland fallback | `platform_linux.go` |
| Frontend API: create, edit, hide, anchor, delete | `notes.go` |
| Sync loop, E2E encode/decode, sync settings (shared with Pogo Pocket) | `pkg/pogosync` |
| Sync glue: window updates, events for the manager | `sync.go`, `settings.go` |
| Encrypted local SQLite store | `pkg/store` |
| AES-GCM, Argon2id | `pkg/secure` |
| OS keyring key | `internal/localkey` |
| Pogo Pad HTTP client | `pkg/syncclient` |
| Note colors | `pkg/palette` |
| niri IPC client | `internal/niri` |
| Note UI, Markdown and tasks, manager UI | `frontend/src/NoteView.svelte`, `frontend/src/lib/markdown.ts`, `frontend/src/Manager.svelte` |

The sync protocol is documented in Pogo Pad's [API.md](https://github.com/dvher/pogo_pad/blob/main/API.md).

## License

[MIT](LICENSE)
