package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupTempConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	t.Setenv("TIMMY_CONFIG", path)
	t.Setenv("TIMMY_API_URL", "")
	return path
}

func readStore(t *testing.T, path string) Store {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read store: %v", err)
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatalf("unmarshal store: %v", err)
	}
	return s
}

func TestLoadStore_NoFile(t *testing.T) {
	setupTempConfig(t)
	store, err := LoadStore()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.Servers) != 0 {
		t.Fatalf("expected empty servers, got %d", len(store.Servers))
	}
}

func TestLoadStore_CorruptJSON(t *testing.T) {
	path := setupTempConfig(t)
	if err := os.WriteFile(path, []byte("{bad json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadStore()
	if err == nil {
		t.Fatal("expected error for corrupt JSON")
	}
}

func TestAddServer_FirstBecomesActive(t *testing.T) {
	path := setupTempConfig(t)
	if err := AddServer("alpha", "http://alpha.example.com:8080"); err != nil {
		t.Fatalf("add: %v", err)
	}
	s := readStore(t, path)
	if len(s.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(s.Servers))
	}
	if s.Active != "alpha" {
		t.Fatalf("expected active=alpha, got %q", s.Active)
	}
	if s.Servers[0].URL != "http://alpha.example.com:8080" {
		t.Fatalf("unexpected URL: %s", s.Servers[0].URL)
	}
}

func TestAddServer_SecondDoesNotChangeActive(t *testing.T) {
	setupTempConfig(t)
	if err := AddServer("alpha", "http://alpha:8080"); err != nil {
		t.Fatal(err)
	}
	if err := AddServer("beta", "http://beta:8080"); err != nil {
		t.Fatal(err)
	}
	store, _ := LoadStore()
	if store.Active != "alpha" {
		t.Fatalf("expected active=alpha, got %q", store.Active)
	}
	if len(store.Servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(store.Servers))
	}
}

func TestAddServer_DuplicateNameUpdatesURL(t *testing.T) {
	setupTempConfig(t)
	if err := AddServer("alpha", "http://old:8080"); err != nil {
		t.Fatal(err)
	}
	if err := AddServer("alpha", "http://new:9090"); err != nil {
		t.Fatal(err)
	}
	store, _ := LoadStore()
	if len(store.Servers) != 1 {
		t.Fatalf("expected 1 server after update, got %d", len(store.Servers))
	}
	if store.Servers[0].URL != "http://new:9090" {
		t.Fatalf("expected updated URL, got %s", store.Servers[0].URL)
	}
}

func TestAddServer_NameDefaultsToHost(t *testing.T) {
	setupTempConfig(t)
	if err := AddServer("", "http://myhost.example.com:3000"); err != nil {
		t.Fatal(err)
	}
	store, _ := LoadStore()
	if store.Servers[0].Name != "myhost.example.com:3000" {
		t.Fatalf("expected hostname as name, got %q", store.Servers[0].Name)
	}
}

func TestAddServer_TrimsTrailingSlash(t *testing.T) {
	setupTempConfig(t)
	if err := AddServer("x", "http://host:8080/"); err != nil {
		t.Fatal(err)
	}
	store, _ := LoadStore()
	if store.Servers[0].URL != "http://host:8080" {
		t.Fatalf("expected no trailing slash, got %s", store.Servers[0].URL)
	}
}

func TestAddServer_InvalidURL(t *testing.T) {
	setupTempConfig(t)
	if err := AddServer("bad", "not-a-url"); err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestRemoveServer_RemovesEntry(t *testing.T) {
	setupTempConfig(t)
	_ = AddServer("a", "http://a:1")
	_ = AddServer("b", "http://b:2")

	if err := RemoveServer("a"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	store, _ := LoadStore()
	if len(store.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(store.Servers))
	}
	if store.Servers[0].Name != "b" {
		t.Fatalf("expected b to remain, got %s", store.Servers[0].Name)
	}
}

func TestRemoveServer_ActiveFallsToNext(t *testing.T) {
	setupTempConfig(t)
	_ = AddServer("a", "http://a:1")
	_ = AddServer("b", "http://b:2")

	if err := RemoveServer("a"); err != nil {
		t.Fatal(err)
	}
	store, _ := LoadStore()
	if store.Active != "b" {
		t.Fatalf("expected active=b after removing active, got %q", store.Active)
	}
}

func TestRemoveServer_RemoveLastClearsActive(t *testing.T) {
	setupTempConfig(t)
	_ = AddServer("only", "http://only:1")
	if err := RemoveServer("only"); err != nil {
		t.Fatal(err)
	}
	store, _ := LoadStore()
	if store.Active != "" {
		t.Fatalf("expected empty active, got %q", store.Active)
	}
}

func TestRemoveServer_NotFound(t *testing.T) {
	setupTempConfig(t)
	if err := RemoveServer("ghost"); err == nil {
		t.Fatal("expected error for missing server")
	}
}

func TestSetActive(t *testing.T) {
	setupTempConfig(t)
	_ = AddServer("a", "http://a:1")
	_ = AddServer("b", "http://b:2")

	if err := SetActive("b"); err != nil {
		t.Fatal(err)
	}
	store, _ := LoadStore()
	if store.Active != "b" {
		t.Fatalf("expected active=b, got %q", store.Active)
	}
}

func TestSetActive_NotFound(t *testing.T) {
	setupTempConfig(t)
	_ = AddServer("a", "http://a:1")
	if err := SetActive("nonexistent"); err == nil {
		t.Fatal("expected error for missing server")
	}
}

func TestActiveURL_ReturnsActiveServerURL(t *testing.T) {
	setupTempConfig(t)
	_ = AddServer("a", "http://a:1")
	_ = AddServer("b", "http://b:2")
	_ = SetActive("b")

	url, err := ActiveURL()
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://b:2" {
		t.Fatalf("expected http://b:2, got %s", url)
	}
}

func TestActiveURL_FallsBackToFirst(t *testing.T) {
	setupTempConfig(t)
	store := Store{
		Servers: []ServerEntry{{Name: "x", URL: "http://x:1"}},
		Active:  "",
	}
	if err := SaveStore(store); err != nil {
		t.Fatal(err)
	}

	url, err := ActiveURL()
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://x:1" {
		t.Fatalf("expected fallback to first server, got %s", url)
	}
}

func TestActiveURL_EnvOverride(t *testing.T) {
	setupTempConfig(t)
	_ = AddServer("a", "http://a:1")
	t.Setenv("TIMMY_API_URL", "http://override:9999")

	url, err := ActiveURL()
	if err != nil {
		t.Fatal(err)
	}
	if url != "http://override:9999" {
		t.Fatalf("expected env override, got %s", url)
	}
}

func TestActiveURL_EmptyStoreErrors(t *testing.T) {
	setupTempConfig(t)
	_, err := ActiveURL()
	if err == nil {
		t.Fatal("expected error for empty store")
	}
}

func TestActiveURL_StaleActiveErrors(t *testing.T) {
	setupTempConfig(t)
	store := Store{
		Servers: []ServerEntry{{Name: "a", URL: "http://a:1"}},
		Active:  "deleted-server",
	}
	if err := SaveStore(store); err != nil {
		t.Fatal(err)
	}
	_, err := ActiveURL()
	if err == nil {
		t.Fatal("expected error for stale active reference")
	}
}
