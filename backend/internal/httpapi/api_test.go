package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"timmy/backend/internal/auth"
	"timmy/backend/internal/store"
)

func testServer(t *testing.T, opts ...func(*testCfg)) *httptest.Server {
	t.Helper()
	cfg := &testCfg{
		store:    newFakeStore(),
		resolver: staticResolver{identity: auth.Identity{LoginName: "ops@example.com", DisplayName: "Ops", NodeName: "laptop"}},
	}
	for _, opt := range opts {
		opt(cfg)
	}
	handler := NewHandler(cfg.store, 1, "test-tailnet", cfg.resolver)
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

type testCfg struct {
	store    store.Store
	resolver auth.Resolver
}

func withResolver(r auth.Resolver) func(*testCfg) {
	return func(c *testCfg) { c.resolver = r }
}

func withStore(s store.Store) func(*testCfg) {
	return func(c *testCfg) { c.store = s }
}

// ---------- healthz ----------

func TestHealthz(t *testing.T) {
	srv := testServer(t)
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Fatalf("expected ok: %v", body)
	}
}

func TestHealthz_NoAuth(t *testing.T) {
	srv := testServer(t, withResolver(staticResolver{err: errors.New("no whois")}))
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz should not require auth, got %d", resp.StatusCode)
	}
}

// ---------- auth ----------

func TestAuthMiddlewareRejectsUnknownCaller(t *testing.T) {
	srv := testServer(t, withResolver(staticResolver{err: errors.New("no whois")}))
	resp, err := http.Get(srv.URL + "/me")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
	var body map[string]string
	json.NewDecoder(resp.Body).Decode(&body)
	if body["error"] == "" {
		t.Fatal("expected error in JSON body")
	}
}

// ---------- me ----------

func TestMe(t *testing.T) {
	srv := testServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/me", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]string
	readJSON(t, resp, &body)
	if body["login_name"] != "ops@example.com" {
		t.Fatalf("unexpected login: %s", body["login_name"])
	}
	if body["tailnet"] != "test-tailnet" {
		t.Fatalf("unexpected tailnet: %s", body["tailnet"])
	}
}

// ---------- create ----------

func TestCreateServer(t *testing.T) {
	srv := testServer(t)
	resp := doJSON(t, srv, http.MethodPost, "/servers", map[string]any{
		"name": "web-1", "tailscale_ip": "100.64.0.1", "ssh_user": "root", "tags": []string{"prod"},
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var created store.Server
	readJSON(t, resp, &created)
	if created.Name != "web-1" || created.TailscaleIP != "100.64.0.1" {
		t.Fatalf("unexpected: %+v", created)
	}
	if created.CreatedBy != "ops@example.com" {
		t.Fatalf("expected actor, got %q", created.CreatedBy)
	}
}

func TestCreateServer_MissingName(t *testing.T) {
	srv := testServer(t)
	resp := doJSON(t, srv, http.MethodPost, "/servers", map[string]any{
		"name": "", "tailscale_ip": "100.64.0.1", "ssh_user": "root",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateServer_InvalidIP(t *testing.T) {
	srv := testServer(t)
	resp := doJSON(t, srv, http.MethodPost, "/servers", map[string]any{
		"name": "x", "tailscale_ip": "not-an-ip", "ssh_user": "root",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateServer_InvalidSSHUser(t *testing.T) {
	srv := testServer(t)
	resp := doJSON(t, srv, http.MethodPost, "/servers", map[string]any{
		"name": "x", "tailscale_ip": "100.64.0.1", "ssh_user": "bad user!",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestCreateServer_Conflict(t *testing.T) {
	srv := testServer(t)
	body := map[string]any{
		"name": "dup", "tailscale_ip": "100.64.0.1", "ssh_user": "root",
	}
	resp := doJSON(t, srv, http.MethodPost, "/servers", body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d", resp.StatusCode)
	}
	resp2 := doJSON(t, srv, http.MethodPost, "/servers", body)
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate, got %d", resp2.StatusCode)
	}
}

func TestCreateServer_InvalidJSON(t *testing.T) {
	srv := testServer(t)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/servers", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

func TestCreateServer_UnknownFields(t *testing.T) {
	srv := testServer(t)
	resp := doJSON(t, srv, http.MethodPost, "/servers", map[string]any{
		"name": "x", "tailscale_ip": "100.64.0.1", "ssh_user": "root", "bogus_field": "nope",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown fields, got %d", resp.StatusCode)
	}
}

// ---------- list ----------

func TestListServers_Empty(t *testing.T) {
	srv := testServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/servers", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body struct{ Servers []store.Server }
	readJSON(t, resp, &body)
	if len(body.Servers) != 0 {
		t.Fatalf("expected empty, got %d", len(body.Servers))
	}
}

func TestListServers_TagFilter(t *testing.T) {
	srv := testServer(t)
	doJSON(t, srv, http.MethodPost, "/servers", map[string]any{
		"name": "a", "tailscale_ip": "100.64.0.1", "ssh_user": "root", "tags": []string{"prod"},
	})
	doJSON(t, srv, http.MethodPost, "/servers", map[string]any{
		"name": "b", "tailscale_ip": "100.64.0.2", "ssh_user": "root", "tags": []string{"staging"},
	})

	resp := doJSON(t, srv, http.MethodGet, "/servers?tag=prod", nil)
	var body struct{ Servers []store.Server }
	readJSON(t, resp, &body)
	if len(body.Servers) != 1 || body.Servers[0].Name != "a" {
		t.Fatalf("expected only prod server: %+v", body.Servers)
	}
}

// ---------- search ----------

func TestSearchServers(t *testing.T) {
	srv := testServer(t)
	doJSON(t, srv, http.MethodPost, "/servers", map[string]any{
		"name": "prod-db", "tailscale_ip": "100.64.0.1", "ssh_user": "root", "tags": []string{"203.0.113.10"},
	})
	doJSON(t, srv, http.MethodPost, "/servers", map[string]any{
		"name": "staging-web", "tailscale_ip": "100.64.0.2", "ssh_user": "root",
	})

	resp := doJSON(t, srv, http.MethodGet, "/servers/search?q=203.0.113.10", nil)
	var body struct {
		Query   string         `json:"query"`
		Servers []store.Server `json:"servers"`
	}
	readJSON(t, resp, &body)
	if body.Query != "203.0.113.10" {
		t.Fatalf("expected query in response: %q", body.Query)
	}
	if len(body.Servers) != 1 || body.Servers[0].Name != "prod-db" {
		t.Fatalf("unexpected: %+v", body.Servers)
	}
}

func TestSearchServers_MissingQuery(t *testing.T) {
	srv := testServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/servers/search", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestSearchServers_InvalidLimit(t *testing.T) {
	srv := testServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/servers/search?q=x&limit=-1", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad limit, got %d", resp.StatusCode)
	}
}

func TestSearchServers_LimitCapped(t *testing.T) {
	srv := testServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/servers/search?q=x&limit=999", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestSearchServers_EmptyQuery(t *testing.T) {
	srv := testServer(t)
	resp := doJSON(t, srv, http.MethodGet, "/servers/search?q=+", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for whitespace-only query, got %d", resp.StatusCode)
	}
}

// ---------- update ----------

func TestUpdateServer(t *testing.T) {
	srv := testServer(t)
	doJSON(t, srv, http.MethodPost, "/servers", map[string]any{
		"name": "web", "tailscale_ip": "100.64.0.1", "ssh_user": "root", "tags": []string{"prod"},
	})

	resp := doJSON(t, srv, http.MethodPatch, "/servers/1", map[string]any{
		"ssh_user": "deploy", "tags": []string{"prod", "updated"},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var updated store.Server
	readJSON(t, resp, &updated)
	if updated.SSHUser != "deploy" {
		t.Fatalf("ssh_user not updated: %q", updated.SSHUser)
	}
	if !slices.Contains(updated.Tags, "updated") {
		t.Fatalf("tags not updated: %v", updated.Tags)
	}
	if updated.UpdatedBy != "ops@example.com" {
		t.Fatalf("expected actor in updated_by: %q", updated.UpdatedBy)
	}
}

func TestUpdateServer_NoFields(t *testing.T) {
	srv := testServer(t)
	doJSON(t, srv, http.MethodPost, "/servers", map[string]any{
		"name": "x", "tailscale_ip": "100.64.0.1", "ssh_user": "root",
	})
	resp := doJSON(t, srv, http.MethodPatch, "/servers/1", map[string]any{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for no fields, got %d", resp.StatusCode)
	}
}

func TestUpdateServer_NotFound(t *testing.T) {
	srv := testServer(t)
	resp := doJSON(t, srv, http.MethodPatch, "/servers/999", map[string]any{"ssh_user": "x"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestUpdateServer_InvalidID(t *testing.T) {
	srv := testServer(t)
	resp := doJSON(t, srv, http.MethodPatch, "/servers/abc", map[string]any{"ssh_user": "x"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid ID, got %d", resp.StatusCode)
	}
}

func TestUpdateServer_ZeroID(t *testing.T) {
	srv := testServer(t)
	resp := doJSON(t, srv, http.MethodPatch, "/servers/0", map[string]any{"ssh_user": "x"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for zero ID, got %d", resp.StatusCode)
	}
}

func TestUpdateServer_NegativeID(t *testing.T) {
	srv := testServer(t)
	resp := doJSON(t, srv, http.MethodPatch, "/servers/-1", map[string]any{"ssh_user": "x"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for negative ID, got %d", resp.StatusCode)
	}
}

func TestUpdateServer_Conflict(t *testing.T) {
	srv := testServer(t)
	doJSON(t, srv, http.MethodPost, "/servers", map[string]any{
		"name": "a", "tailscale_ip": "100.64.0.1", "ssh_user": "root",
	})
	doJSON(t, srv, http.MethodPost, "/servers", map[string]any{
		"name": "b", "tailscale_ip": "100.64.0.2", "ssh_user": "root",
	})
	resp := doJSON(t, srv, http.MethodPatch, "/servers/2", map[string]any{"name": "a"})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 for name conflict, got %d", resp.StatusCode)
	}
}

// ---------- delete ----------

func TestDeleteServer(t *testing.T) {
	srv := testServer(t)
	doJSON(t, srv, http.MethodPost, "/servers", map[string]any{
		"name": "x", "tailscale_ip": "100.64.0.1", "ssh_user": "root",
	})
	resp := doJSON(t, srv, http.MethodDelete, "/servers/1", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	readJSON(t, resp, &body)
	if body["deleted"] != true {
		t.Fatalf("expected deleted=true: %v", body)
	}
}

func TestDeleteServer_NotFound(t *testing.T) {
	srv := testServer(t)
	resp := doJSON(t, srv, http.MethodDelete, "/servers/999", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDeleteServer_InvalidID(t *testing.T) {
	srv := testServer(t)
	resp := doJSON(t, srv, http.MethodDelete, "/servers/abc", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// ---------- full CRUD flow ----------

func TestServerCRUDAndSearchFlow(t *testing.T) {
	srv := testServer(t)

	createdResp := doJSON(t, srv, http.MethodPost, "/servers", map[string]any{
		"name": "prod-db-1", "tailscale_ip": "100.64.0.10", "ssh_user": "root",
		"tags": []string{"prod", "203.0.113.10", "database"},
	})
	if createdResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createdResp.StatusCode)
	}
	var created store.Server
	readJSON(t, createdResp, &created)
	if created.CreatedBy != "ops@example.com" {
		t.Fatalf("expected actor: %q", created.CreatedBy)
	}

	listResp := doJSON(t, srv, http.MethodGet, "/servers?tag=prod", nil)
	var listed struct{ Servers []store.Server }
	readJSON(t, listResp, &listed)
	if len(listed.Servers) != 1 || listed.Servers[0].Name != "prod-db-1" {
		t.Fatalf("unexpected list: %+v", listed.Servers)
	}

	searchResp := doJSON(t, srv, http.MethodGet, "/servers/search?q=203.0.113.10", nil)
	var searched struct{ Servers []store.Server }
	readJSON(t, searchResp, &searched)
	if len(searched.Servers) != 1 || searched.Servers[0].ID != created.ID {
		t.Fatalf("unexpected search: %+v", searched.Servers)
	}

	updateResp := doJSON(t, srv, http.MethodPatch, "/servers/1", map[string]any{
		"ssh_user": "ubuntu", "tags": []string{"prod", "203.0.113.10", "postgres"},
	})
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("update: %d", updateResp.StatusCode)
	}
	var updated store.Server
	readJSON(t, updateResp, &updated)
	if updated.SSHUser != "ubuntu" || !slices.Contains(updated.Tags, "postgres") {
		t.Fatalf("unexpected: %+v", updated)
	}

	deleteResp := doJSON(t, srv, http.MethodDelete, "/servers/1", nil)
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("delete: %d", deleteResp.StatusCode)
	}

	finalResp := doJSON(t, srv, http.MethodGet, "/servers", nil)
	var finalList struct{ Servers []store.Server }
	readJSON(t, finalResp, &finalList)
	if len(finalList.Servers) != 0 {
		t.Fatalf("expected empty: %+v", finalList.Servers)
	}
}

// ---------- helpers ----------

func doJSON(t *testing.T, server *httptest.Server, method, path string, body any) *http.Response {
	t.Helper()
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
	}
	req, err := http.NewRequest(method, server.URL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func readJSON(t *testing.T, resp *http.Response, dst any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

// ---------- fakes ----------

type staticResolver struct {
	identity auth.Identity
	err      error
}

func (r staticResolver) Resolve(ctx context.Context, remoteAddr string) (*auth.Identity, error) {
	if r.err != nil {
		return nil, r.err
	}
	identity := r.identity
	return &identity, nil
}

type fakeStore struct {
	mu      sync.Mutex
	nextID  int64
	servers map[int64]store.Server
}

func newFakeStore() *fakeStore {
	return &fakeStore{nextID: 1, servers: make(map[int64]store.Server)}
}

func (s *fakeStore) EnsureTailnet(ctx context.Context, name string) (int64, error) {
	return 1, nil
}

func (s *fakeStore) ListServers(ctx context.Context, tailnetID int64, tags []string) ([]store.Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.filteredLocked("", tags, 0), nil
}

func (s *fakeStore) SearchServers(ctx context.Context, tailnetID int64, query string, tags []string, limit int) ([]store.Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.filteredLocked(query, tags, limit), nil
}

func (s *fakeStore) CreateServer(ctx context.Context, tailnetID int64, actor string, input store.CreateServerInput) (store.Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.servers {
		if strings.EqualFold(existing.Name, input.Name) || existing.TailscaleIP == input.TailscaleIP {
			return store.Server{}, store.ErrConflict
		}
	}
	now := time.Now().UTC()
	server := store.Server{
		ID: s.nextID, Name: input.Name, TailscaleIP: input.TailscaleIP,
		SSHUser: input.SSHUser, Tags: copySlice(input.Tags),
		CreatedAt: now, UpdatedAt: now, CreatedBy: actor, UpdatedBy: actor,
	}
	s.servers[server.ID] = server
	s.nextID++
	return server, nil
}

func (s *fakeStore) UpdateServer(ctx context.Context, tailnetID int64, id int64, actor string, input store.UpdateServerInput) (store.Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	server, ok := s.servers[id]
	if !ok {
		return store.Server{}, store.ErrNotFound
	}
	for _, existing := range s.servers {
		if existing.ID == id {
			continue
		}
		if input.Name != nil && strings.EqualFold(existing.Name, *input.Name) {
			return store.Server{}, store.ErrConflict
		}
		if input.TailscaleIP != nil && existing.TailscaleIP == *input.TailscaleIP {
			return store.Server{}, store.ErrConflict
		}
	}
	if input.Name != nil {
		server.Name = *input.Name
	}
	if input.TailscaleIP != nil {
		server.TailscaleIP = *input.TailscaleIP
	}
	if input.SSHUser != nil {
		server.SSHUser = *input.SSHUser
	}
	if input.Tags != nil {
		server.Tags = copySlice(*input.Tags)
	}
	server.UpdatedAt = time.Now().UTC()
	server.UpdatedBy = actor
	s.servers[id] = server
	return server, nil
}

func (s *fakeStore) DeleteServer(ctx context.Context, tailnetID int64, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.servers[id]; !ok {
		return store.ErrNotFound
	}
	delete(s.servers, id)
	return nil
}

func (s *fakeStore) filteredLocked(query string, tags []string, limit int) []store.Server {
	results := make([]store.Server, 0, len(s.servers))
	query = strings.ToLower(strings.TrimSpace(query))
	for _, server := range s.servers {
		if !matchesAllTags(server.Tags, tags) {
			continue
		}
		if query != "" && !matchesQuery(server, query) {
			continue
		}
		results = append(results, server)
	}
	slices.SortFunc(results, func(a, b store.Server) int {
		return strings.Compare(a.Name, b.Name)
	})
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results
}

func matchesAllTags(serverTags, required []string) bool {
	for _, tag := range required {
		found := false
		for _, st := range serverTags {
			if strings.EqualFold(tag, st) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func matchesQuery(server store.Server, query string) bool {
	if strings.Contains(strings.ToLower(server.Name), query) {
		return true
	}
	if strings.Contains(strings.ToLower(server.TailscaleIP), query) {
		return true
	}
	if strings.Contains(strings.ToLower(server.SSHUser), query) {
		return true
	}
	for _, tag := range server.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

func copySlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
