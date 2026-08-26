package workbench

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"penthertz/rfswift/remote"
)

// MissionSecret is deliberately metadata-only. Values live in the operating
// system credential store and therefore never enter projects, reports, or MCP
// evidence indexes.
type MissionSecret struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Kind     string `json:"kind"`
	Username string `json:"username,omitempty"`
	Source   string `json:"source"`
	Note     string `json:"note,omitempty"`
	Created  string `json:"created"`
	Updated  string `json:"updated"`
	HasValue bool   `json:"hasValue"`
}

func (s *Store) secretsPath(ws, mission string) string {
	return filepath.Join(s.missionDir(ws, mission), "secrets.json")
}

func (s *Store) LoadSecrets(ws, mission string) ([]MissionSecret, error) {
	var values []MissionSecret
	err := readJSON(s.secretsPath(ws, mission), &values)
	if errors.Is(err, os.ErrNotExist) {
		return []MissionSecret{}, nil
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Updated > values[j].Updated })
	return values, err
}

func (s *Store) SaveSecrets(ws, mission string, values []MissionSecret) error {
	if err := os.MkdirAll(s.missionDir(ws, mission), 0o700); err != nil {
		return err
	}
	return writeJSON(s.secretsPath(ws, mission), values)
}

func secretRef(root, ws, mission, id string) string {
	sum := sha256.Sum256([]byte(root + "\x00" + ws + "\x00" + mission + "\x00" + id))
	return "workbench-mission-secret-" + hex.EncodeToString(sum[:])
}

func newSecretID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "S-" + hex.EncodeToString(b), nil
}

func saveMissionSecret(store *Store, vault remote.SecretStore, ws, mission string, item MissionSecret, value string) (MissionSecret, error) {
	item.Label, item.Kind, item.Source = strings.TrimSpace(item.Label), strings.TrimSpace(item.Kind), strings.TrimSpace(item.Source)
	value = strings.TrimSpace(value)
	if item.Label == "" || item.Source == "" || value == "" {
		return MissionSecret{}, errors.New("label, exact source, and secret value are required")
	}
	if item.Kind == "" {
		item.Kind = "other"
	}
	items, err := store.LoadSecrets(ws, mission)
	if err != nil {
		return MissionSecret{}, err
	}
	if item.ID == "" {
		item.ID, err = newSecretID()
		if err != nil {
			return MissionSecret{}, err
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	item.Updated, item.HasValue = now, true
	if item.Created == "" {
		item.Created = now
	}
	ref := secretRef(store.Root, ws, mission, item.ID)
	if err := vault.Set(ref, []byte(value)); err != nil {
		return MissionSecret{}, fmt.Errorf("store secret in OS credential vault: %w", err)
	}
	replaced := false
	for i := range items {
		if items[i].ID == item.ID {
			item.Created = items[i].Created
			items[i], replaced = item, true
		}
	}
	if !replaced {
		items = append(items, item)
	}
	if err := store.SaveSecrets(ws, mission, items); err != nil {
		if !replaced {
			_ = vault.Delete(ref)
		}
		return MissionSecret{}, err
	}
	return item, nil
}

func (a *App) ListSecrets(mission string) ([]MissionSecret, error) {
	if err := a.requireMission(mission); err != nil {
		return nil, err
	}
	return a.store.LoadSecrets(a.ws, mission)
}

func (a *App) SaveSecret(mission string, item MissionSecret, value string) (MissionSecret, error) {
	if err := a.requireMission(mission); err != nil {
		return MissionSecret{}, err
	}
	return saveMissionSecret(a.store, a.secretStore, a.ws, mission, item, value)
}

func (a *App) RevealSecret(mission, id string) (string, error) {
	if err := a.requireMission(mission); err != nil {
		return "", err
	}
	if !validSecretID(id) {
		return "", errors.New("invalid secret ID")
	}
	items, err := a.store.LoadSecrets(a.ws, mission)
	if err != nil {
		return "", err
	}
	for _, item := range items {
		if item.ID == id {
			b, e := a.secretStore.Get(secretRef(a.store.Root, a.ws, mission, id))
			return string(b), e
		}
	}
	return "", errors.New("secret not found")
}

func (a *App) DeleteSecret(mission, id string) error {
	if err := a.requireMission(mission); err != nil {
		return err
	}
	if !validSecretID(id) {
		return errors.New("invalid secret ID")
	}
	items, err := a.store.LoadSecrets(a.ws, mission)
	if err != nil {
		return err
	}
	next := items[:0]
	found := false
	for _, item := range items {
		if item.ID == id {
			found = true
		} else {
			next = append(next, item)
		}
	}
	if !found {
		return errors.New("secret not found")
	}
	if err := a.secretStore.Delete(secretRef(a.store.Root, a.ws, mission, id)); err != nil {
		return fmt.Errorf("delete from OS credential vault: %w", err)
	}
	return a.store.SaveSecrets(a.ws, mission, next)
}

func validSecretID(id string) bool {
	if !strings.HasPrefix(id, "S-") || len(id) != 26 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(id, "S-"))
	return err == nil
}
