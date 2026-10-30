package notefile

import (
	"time"

	"github.com/google/uuid"

	"github.com/dvher/pogo/pkg/palette"
	"github.com/dvher/pogo/pkg/store"
)

// Report counts what an import did.
type Report struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

// Import adds items to st. Notes exported by Pogo keep their id, so importing
// them again updates the same notes, last write wins. Other files become new
// notes, unless a note with the same content already exists. Imported notes
// are marked dirty so they sync. New notes start hidden on the desktop, like
// notes that arrive by sync.
func Import(st *store.Store, deviceID string, items []Item) (Report, error) {
	var r Report
	live, err := st.List()
	if err != nil {
		return r, err
	}
	have := map[string]bool{}
	for _, n := range live {
		have[n.Content] = true
	}
	now := time.Now().UnixMilli()
	for _, it := range items {
		if len(it.Content) > MaxContent {
			r.Skipped++
			continue
		}
		if it.ID == "" && have[it.Content] {
			r.Skipped++
			continue
		}
		n := store.Note{
			ID: it.ID, Content: it.Content, Color: it.Color,
			CreatedAt: it.CreatedAt, UpdatedAt: it.UpdatedAt, Hidden: true,
		}
		if n.ID == "" {
			n.ID = uuid.NewString()
		}
		if !palette.Valid(n.Color) {
			n.Color = palette.Default
		}
		if n.UpdatedAt == 0 {
			n.UpdatedAt = now
		}
		if n.CreatedAt == 0 {
			n.CreatedAt = n.UpdatedAt
		}
		created, updated, err := st.Import(n, deviceID)
		if err != nil {
			return r, err
		}
		switch {
		case created:
			r.Created++
		case updated:
			r.Updated++
		default:
			r.Skipped++
		}
		have[n.Content] = true
	}
	return r, nil
}
