package sessionstore

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/matt/sysiphus/internal/state"
)

type Store struct {
	baseDir      string
	workspaceDir string
	sessionsDir  string
	cwd          string
}

type SessionMeta struct {
	LocalID           string               `json:"local_id"`
	Provider          string               `json:"provider"`
	ProviderSessionID string               `json:"provider_session_id,omitempty"`
	Cwd               string               `json:"cwd"`
	Model             string               `json:"model,omitempty"`
	Mode              string               `json:"mode,omitempty"`
	ApprovalPolicy    state.ApprovalPolicy `json:"approval_policy,omitempty"`
	Title             string               `json:"title,omitempty"`
	Status            string               `json:"status,omitempty"`
	CreatedAt         time.Time            `json:"created_at"`
	UpdatedAt         time.Time            `json:"updated_at"`
}

type EventRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Kind      string    `json:"kind"`
	Ref       string    `json:"ref,omitempty"`
	Text      string    `json:"text,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	Status    string    `json:"status,omitempty"`
	Session   string    `json:"session,omitempty"`
}

type workspaceMeta struct {
	Cwd       string    `json:"cwd"`
	UpdatedAt time.Time `json:"updated_at"`
}

func New(cwd string) *Store {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		abs = cwd
	}
	baseDir := filepath.Join(dataHome(), "workspaces", workspaceKey(abs))
	return &Store{
		baseDir:      dataHome(),
		workspaceDir: baseDir,
		sessionsDir:  filepath.Join(baseDir, "sessions"),
		cwd:          abs,
	}
}

func (s *Store) CreateSession(meta SessionMeta) (SessionMeta, error) {
	if err := s.ensureWorkspace(); err != nil {
		return SessionMeta{}, err
	}
	now := time.Now().UTC()
	if meta.LocalID == "" {
		meta.LocalID = now.Format("20060102T150405.000000000Z0700")
	}
	meta.Cwd = s.cwd
	meta.CreatedAt = now
	meta.UpdatedAt = now
	if meta.Status == "" {
		meta.Status = "active"
	}
	if err := s.writeMeta(meta); err != nil {
		return SessionMeta{}, err
	}
	return meta, nil
}

func (s *Store) ListSessions() ([]state.SessionSummary, error) {
	if err := s.ensureWorkspace(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.sessionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	items := make([]state.SessionSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := s.LoadMeta(entry.Name())
		if err != nil {
			continue
		}
		title := meta.Title
		if title == "" {
			title = meta.LocalID
		}
		items = append(items, state.SessionSummary{
			ID:                meta.LocalID,
			Title:             title,
			UpdatedAt:         meta.UpdatedAt,
			ProviderSessionID: meta.ProviderSessionID,
			Model:             meta.Model,
			Mode:              meta.Mode,
			ApprovalPolicy:    meta.ApprovalPolicy,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
	return items, nil
}

func (s *Store) LoadSession(localID string) (SessionMeta, []EventRecord, error) {
	meta, err := s.LoadMeta(localID)
	if err != nil {
		return SessionMeta{}, nil, err
	}
	path := filepath.Join(s.sessionDir(localID), "events.jsonl")
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return meta, nil, nil
		}
		return SessionMeta{}, nil, err
	}
	defer file.Close()

	records := make([]EventRecord, 0, 128)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record EventRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return SessionMeta{}, nil, err
	}
	return meta, records, nil
}

func (s *Store) LoadMeta(localID string) (SessionMeta, error) {
	path := filepath.Join(s.sessionDir(localID), "meta.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return SessionMeta{}, err
	}
	var meta SessionMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return SessionMeta{}, err
	}
	return meta, nil
}

func (s *Store) AppendEvent(localID string, record EventRecord) error {
	if localID == "" {
		return nil
	}
	if err := s.ensureWorkspace(); err != nil {
		return err
	}
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	}
	path := filepath.Join(s.sessionDir(localID), "events.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return s.UpdateSession(localID, func(meta *SessionMeta) {
		if meta.Title == "" && record.Kind == "user" && record.Text != "" {
			meta.Title = shorten(record.Text, 80)
		}
	})
}

func (s *Store) UpdateSession(localID string, update func(*SessionMeta)) error {
	meta, err := s.LoadMeta(localID)
	if err != nil {
		return err
	}
	update(&meta)
	meta.UpdatedAt = time.Now().UTC()
	return s.writeMeta(meta)
}

func (s *Store) LatestSession() (*state.SessionSummary, error) {
	items, err := s.ListSessions()
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return &items[0], nil
}

func (s *Store) sessionDir(localID string) string {
	return filepath.Join(s.sessionsDir, localID)
}

func (s *Store) writeMeta(meta SessionMeta) error {
	if err := os.MkdirAll(s.sessionDir(meta.LocalID), 0o755); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.sessionDir(meta.LocalID), "meta.json"), meta)
}

func (s *Store) ensureWorkspace() error {
	if err := os.MkdirAll(s.sessionsDir, 0o755); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.workspaceDir, "workspace.json"), workspaceMeta{
		Cwd:       s.cwd,
		UpdatedAt: time.Now().UTC(),
	})
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.tmp", path)
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func dataHome() string {
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "sysiphus")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".sysiphus")
	}
	return filepath.Join(home, ".local", "share", "sysiphus")
}

func workspaceKey(cwd string) string {
	sum := sha256.Sum256([]byte(cwd))
	return hex.EncodeToString(sum[:8])
}

func shorten(text string, limit int) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit-1]) + "…"
}
