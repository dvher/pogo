package main

import "github.com/dvher/pogo/pkg/pogosync"

// SettingsService manages sync settings and end-to-end encryption.
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
