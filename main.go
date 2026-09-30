package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"log"
	"log/slog"

	"github.com/google/uuid"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/dvher/pogo/internal/secure"
	"github.com/dvher/pogo/internal/store"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed assets/tray.png
var trayIcon []byte

//go:embed build/appicon.png
var appIcon []byte

func main() {
	restoreEnv := chooseGDKBackend()

	dataDir, err := store.DefaultDataDir()
	if err != nil {
		log.Fatal(err)
	}
	key, err := secure.LocalKey(dataDir)
	if err != nil {
		log.Fatalf("encryption key: %v", err)
	}
	cipher, err := secure.NewCipher(key)
	if err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(dataDir, cipher)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer st.Close()

	deviceID := st.Setting(keyDeviceID, "")
	if deviceID == "" {
		deviceID = uuid.NewString()
		st.SetSetting(keyDeviceID, deviceID)
	}

	windows := NewWindowManager(st)
	syncSvc := NewSyncService(st, windows, deviceID)
	notes := &NoteService{store: st, windows: windows, sync: syncSvc, deviceID: deviceID}
	settings := &SettingsService{store: st, sync: syncSvc}

	// One instance per data directory; a second launch opens the manager.
	instanceID := sha256.Sum256([]byte(dataDir))
	app := application.New(application.Options{
		Name:        "Pogo",
		Description: "Markdown sticky notes that float above your windows",
		Icon:        appIcon,
		Services: []application.Service{
			application.NewService(notes),
			application.NewService(settings),
			application.NewService(syncSvc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyAccessory,
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "dev.pogo.desktop.i" + hex.EncodeToString(instanceID[:6]),
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				notes.OpenManager()
			},
		},
		OnShutdown: windows.Stop,
	})
	windows.app, syncSvc.app, notes.app = app, app, app

	manager := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "manager",
		Title:     "Pogo",
		URL:       "/?view=manager",
		Width:     760,
		Height:    560,
		MinWidth:  480,
		MinHeight: 360,
		Hidden:    true,
	})
	manager.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if !windows.quitting.Load() {
			manager.Hide()
			e.Cancel()
		}
	})
	notes.manager = manager

	setupTray(app, notes, syncSvc, windows)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		// GTK has opened its display by now; don't pass GDK_BACKEND on to
		// programs we launch, such as the browser for links.
		restoreEnv()
		windows.Start()
		// First run: create a welcome note so there is something on screen.
		// The emptiness check skips it for databases from before the flag.
		if st.Setting(keyWelcomed, "") == "" {
			st.SetSetting(keyWelcomed, "1")
			if st.Empty() {
				if n, err := notes.Create(); err == nil {
					notes.SetContent(n.ID, welcomeNote)
				}
			}
		}
		go syncSvc.Loop(ctx)
	})

	if err := app.Run(); err != nil {
		slog.Error("app exited", "error", err)
	}
}

func setupTray(app *application.App, notes *NoteService, syncSvc *SyncService, windows *WindowManager) {
	menu := app.NewMenu()
	menu.Add("New note").OnClick(func(*application.Context) { notes.Create() })
	menu.Add("Show all notes").OnClick(func(*application.Context) { notes.ShowAll() })
	menu.Add("Hide all notes").OnClick(func(*application.Context) { notes.HideAll() })
	menu.AddSeparator()
	menu.Add("Manage notes…").OnClick(func(*application.Context) { notes.OpenManager() })
	menu.Add("Sync now").OnClick(func(*application.Context) { go syncSvc.SyncNow() })
	menu.AddSeparator()
	menu.Add("Quit").OnClick(func(*application.Context) {
		windows.Stop()
		app.Quit()
	})

	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetTooltip("Pogo")
	tray.SetMenu(menu)
	tray.OnClick(notes.OpenManager)
}

const welcomeNote = `# Welcome to Pogo 🟨

- [x] Click a note to show its dots
- [ ] 🎨 color · ✏️ edit · 👁 hide · 📌 anchor
- [ ] Click a task to cross it off
- [ ] Drag the note anywhere

Use the **tray icon** to manage notes and set up sync.
`
