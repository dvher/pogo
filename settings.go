package main

import (
	"github.com/dvher/pogo/internal/autostart"
	"github.com/dvher/pogo/pkg/pogosync"
)

// SettingsService manages app settings: starting at login, sync and
// end-to-end encryption.
type SettingsService struct {
	sync *SyncService
}

func (s *SettingsService) Get() pogosync.Settings { return s.sync.engine.Settings() }

// Save stores settings. Pointing at a different server resets the sync
// cursor and queues every note for upload.
func (s *SettingsService) Save(in pogosync.Settings) (pogosync.Settings, error) {
	return s.sync.engine.SaveSettings(in)
}

// TestConnection checks the given (unsaved) settings against the server.
func (s *SettingsService) TestConnection(in pogosync.Settings) (string, error) {
	return s.sync.engine.TestConnection(in)
}

// EnableE2E turns on end-to-end encryption, or unlocks it on this device if
// the server already uses it.
func (s *SettingsService) EnableE2E(passphrase string) error {
	return s.sync.engine.EnableE2E(passphrase)
}

// DisableE2E turns off end-to-end encryption for every device on the server.
func (s *SettingsService) DisableE2E() error { return s.sync.engine.DisableE2E() }

// Startup says whether Pogo starts when the user logs in.
type Startup struct {
	Supported bool `json:"supported"`
	Enabled   bool `json:"enabled"`
}

// GetStartup reports whether Pogo starts at login. It reads the OS entry, so
// it reflects changes made outside Pogo.
func (s *SettingsService) GetStartup() (Startup, error) {
	on, err := autostart.Enabled()
	return Startup{Supported: autostart.Supported(), Enabled: on}, err
}

// SetStartup makes Pogo start at login, or stops it from doing so.
func (s *SettingsService) SetStartup(enabled bool) (Startup, error) {
	if err := autostart.Set(enabled); err != nil {
		return Startup{}, err
	}
	return s.GetStartup()
}
