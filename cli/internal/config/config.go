package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type ServerEntry struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Store struct {
	Servers []ServerEntry `json:"servers"`
	Active  string        `json:"active"`
}

func Dir() (string, error) {
	if value := strings.TrimSpace(os.Getenv("TIMMY_CONFIG")); value != "" {
		return filepath.Dir(value), nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "timmy"), nil
}

func storePath() (string, error) {
	if value := strings.TrimSpace(os.Getenv("TIMMY_CONFIG")); value != "" {
		return value, nil
	}
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func LoadStore() (Store, error) {
	path, err := storePath()
	if err != nil {
		return Store{}, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Store{}, nil
		}
		return Store{}, err
	}

	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		return Store{}, fmt.Errorf("corrupt config at %s: %w", path, err)
	}
	return store, nil
}

func SaveStore(store Store) error {
	path, err := storePath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func ActiveURL() (string, error) {
	if value := strings.TrimSpace(os.Getenv("TIMMY_API_URL")); value != "" {
		return value, nil
	}

	store, err := LoadStore()
	if err != nil {
		return "", err
	}

	if len(store.Servers) == 0 {
		return "", errors.New("timmy is not initialized -- run: timmy init <server-url>")
	}

	if store.Active == "" {
		return store.Servers[0].URL, nil
	}

	for _, s := range store.Servers {
		if s.Name == store.Active {
			return s.URL, nil
		}
	}

	return "", fmt.Errorf("active server %q not found in config -- run: timmy servers", store.Active)
}

func AddServer(name, rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return fmt.Errorf("invalid url %q -- include scheme (e.g. http://...)", rawURL)
	}
	cleanURL := strings.TrimRight(parsed.String(), "/")

	name = strings.TrimSpace(name)
	if name == "" {
		name = parsed.Host
	}

	store, err := LoadStore()
	if err != nil {
		return err
	}

	for i, s := range store.Servers {
		if s.Name == name {
			store.Servers[i].URL = cleanURL
			return SaveStore(store)
		}
	}

	store.Servers = append(store.Servers, ServerEntry{Name: name, URL: cleanURL})

	if store.Active == "" {
		store.Active = name
	}

	return SaveStore(store)
}

func RemoveServer(name string) error {
	store, err := LoadStore()
	if err != nil {
		return err
	}

	found := false
	filtered := make([]ServerEntry, 0, len(store.Servers))
	for _, s := range store.Servers {
		if s.Name == name {
			found = true
			continue
		}
		filtered = append(filtered, s)
	}
	if !found {
		return fmt.Errorf("server %q not found", name)
	}

	store.Servers = filtered
	if store.Active == name {
		if len(store.Servers) > 0 {
			store.Active = store.Servers[0].Name
		} else {
			store.Active = ""
		}
	}

	return SaveStore(store)
}

func SetActive(name string) error {
	store, err := LoadStore()
	if err != nil {
		return err
	}

	for _, s := range store.Servers {
		if s.Name == name {
			store.Active = name
			return SaveStore(store)
		}
	}

	return fmt.Errorf("server %q not found -- run: timmy servers", name)
}
