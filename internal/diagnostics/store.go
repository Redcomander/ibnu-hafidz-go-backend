package diagnostics

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const MaxEvents = 1000
const RetentionDays = 7

var validID = regexp.MustCompile(`^[a-f0-9]{32}$`)

type Event struct {
	ID            string     `json:"id"`
	CreatedAt     time.Time  `json:"created_at"`
	RequestID     string     `json:"request_id"`
	Method        string     `json:"method"`
	Route         string     `json:"route"`
	Handler       string     `json:"handler"`
	Status        int        `json:"status"`
	DurationMS    int64      `json:"duration_ms"`
	Kind          string     `json:"kind"`
	Severity      string     `json:"severity"`
	Summary       string     `json:"summary"`
	Fingerprint   string     `json:"fingerprint"`
	ActivityLogID uint       `json:"activity_log_id,omitempty"`
	Frames        []string   `json:"frames,omitempty"`
	Resolved      bool       `json:"resolved"`
	ResolvedBy    uint       `json:"resolved_by,omitempty"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
}

type Store struct {
	mu             sync.Mutex
	dir            string
	events         []Event
	debugUntil     time.Time
	storageHealthy bool
}

func NewID() string {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic("request ID entropy unavailable")
	}
	return hex.EncodeToString(data[:])
}

// New uses only application-owned event files in a private directory. It never
// reads .env, request payloads, arbitrary files or the existing audit database.
func New(dir string) (*Store, error) {
	store := &Store{dir: dir, storageHealthy: true}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		name := file.Name()
		if !strings.HasPrefix(name, "event-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		id := strings.TrimSuffix(strings.TrimPrefix(name, "event-"), ".json")
		if !validID.MatchString(id) || file.IsDir() {
			continue
		}
		info, err := file.Info()
		if err != nil || info.Size() > 65536 {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var event Event
		if json.Unmarshal(data, &event) == nil && event.ID == id {
			store.events = append(store.events, event)
		}
	}
	if data, err := os.ReadFile(filepath.Join(dir, "settings.json")); err == nil {
		var settings struct {
			DebugUntil time.Time `json:"debug_until"`
		}
		if json.Unmarshal(data, &settings) == nil {
			store.debugUntil = settings.DebugUntil
		}
	}
	store.pruneLocked()
	return store, nil
}

func Memory() *Store { return &Store{} }

func (s *Store) writeLocked(name string, value any) error {
	if s.dir == "" {
		return errors.New("diagnostic persistence unavailable")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(s.dir, ".pending-")
	if err != nil {
		s.storageHealthy = false
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err = file.Write(data); err != nil {
		file.Close()
		s.storageHealthy = false
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		s.storageHealthy = false
		return err
	}
	if err = file.Close(); err != nil {
		s.storageHealthy = false
		return err
	}
	if err = os.Rename(tmp, filepath.Join(s.dir, name)); err != nil {
		s.storageHealthy = false
		return err
	}
	s.storageHealthy = true
	return nil
}

func (s *Store) pruneLocked() {
	sort.Slice(s.events, func(i, j int) bool { return s.events[i].CreatedAt.After(s.events[j].CreatedAt) })
	cutoff := time.Now().Add(-RetentionDays * 24 * time.Hour)
	kept := s.events[:0]
	for _, event := range s.events {
		if len(kept) < MaxEvents && event.CreatedAt.After(cutoff) {
			kept = append(kept, event)
			continue
		}
		if s.dir != "" && validID.MatchString(event.ID) {
			if err := os.Remove(filepath.Join(s.dir, "event-"+event.ID+".json")); err != nil && !os.IsNotExist(err) {
				s.storageHealthy = false
			}
		}
	}
	s.events = kept
}

func (s *Store) Add(event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validID.MatchString(event.ID) {
		return errors.New("invalid diagnostic ID")
	}
	s.events = append(s.events, event)
	err := s.writeLocked("event-"+event.ID+".json", event)
	s.pruneLocked()
	return err
}

func (s *Store) Snapshot() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	result := make([]Event, len(s.events))
	copy(result, s.events)
	return result
}

func (s *Store) Resolve(id string, resolved bool, actor uint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validID.MatchString(id) {
		return os.ErrNotExist
	}
	for i, event := range s.events {
		if event.ID != id {
			continue
		}
		event.Resolved = resolved
		event.ResolvedBy = 0
		event.ResolvedAt = nil
		if resolved {
			now := time.Now().UTC()
			event.ResolvedAt = &now
			event.ResolvedBy = actor
		}
		if err := s.writeLocked("event-"+id+".json", event); err != nil {
			return err
		}
		s.events[i] = event
		return nil
	}
	return os.ErrNotExist
}

func (s *Store) Settings() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]any{"debug_enabled": time.Now().Before(s.debugUntil), "debug_until": s.debugUntil,
		"storage_healthy": s.storageHealthy, "max_events": MaxEvents, "retention_days": RetentionDays}
}

func (s *Store) DebugEnabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Now().Before(s.debugUntil)
}

func (s *Store) SetDebug(enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	until := time.Time{}
	if enabled {
		until = time.Now().UTC().Add(15 * time.Minute)
	}
	if err := s.writeLocked("settings.json", map[string]any{"debug_until": until}); err != nil {
		return err
	}
	s.debugUntil = until
	return nil
}
