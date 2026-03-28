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

func TestAuthMiddlewareRejectsUnknownCaller(t *testing.T) {
	handler := NewHandler(newFakeStore(), 1, "example-tailnet", staticResolver{err: errors.New("no whois")})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	resp, err := http.Get(server.URL + "/me")
	if err != nil {
		t.Fatalf("GET /me: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestServerCRUDAndSearchFlow(t *testing.T) {
	handler := NewHandler(
		newFakeStore(),
		1,
		"example-tailnet",
		staticResolver{identity: auth.Identity{
			LoginName:   "ops@example.com",
			DisplayName: "Ops",
			NodeName:    "laptop",
		}},
	)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	createdResp := doJSON(t, server, http.MethodPost, "/servers", map[string]any{
		"name":         "prod-db-1",
		"tailscale_ip": "100.64.0.10",
		"ssh_user":     "root",
		"tags":         []string{"prod", "203.0.113.10", "database"},
	})
	if createdResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createdResp.StatusCode)
	}

	var created store.Server
	readJSON(t, createdResp, &created)
	if created.CreatedBy != "ops@example.com" {
		t.Fatalf("expected created_by to be set, got %q", created.CreatedBy)
	}

	listResp := doJSON(t, server, http.MethodGet, "/servers?tag=prod", nil)
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for list, got %d", listResp.StatusCode)
	}

	var listed struct {
		Servers []store.Server `json:"servers"`
	}
	readJSON(t, listResp, &listed)
	if len(listed.Servers) != 1 || listed.Servers[0].Name != "prod-db-1" {
		t.Fatalf("unexpected list response: %+v", listed.Servers)
	}

	searchResp := doJSON(t, server, http.MethodGet, "/servers/search?q=203.0.113.10", nil)
	if searchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for search, got %d", searchResp.StatusCode)
	}

	var searched struct {
		Servers []store.Server `json:"servers"`
	}
	readJSON(t, searchResp, &searched)
	if len(searched.Servers) != 1 || searched.Servers[0].ID != created.ID {
		t.Fatalf("unexpected search results: %+v", searched.Servers)
	}

	updateResp := doJSON(t, server, http.MethodPatch, "/servers/1", map[string]any{
		"ssh_user": "ubuntu",
		"tags":     []string{"prod", "203.0.113.10", "postgres"},
	})
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for update, got %d", updateResp.StatusCode)
	}

	var updated store.Server
	readJSON(t, updateResp, &updated)
	if updated.SSHUser != "ubuntu" || !slices.Contains(updated.Tags, "postgres") {
		t.Fatalf("unexpected updated server: %+v", updated)
	}

	deleteResp := doJSON(t, server, http.MethodDelete, "/servers/1", nil)
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for delete, got %d", deleteResp.StatusCode)
	}

	finalListResp := doJSON(t, server, http.MethodGet, "/servers", nil)
	if finalListResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for final list, got %d", finalListResp.StatusCode)
	}

	var finalListed struct {
		Servers []store.Server `json:"servers"`
	}
	readJSON(t, finalListResp, &finalListed)
	if len(finalListed.Servers) != 0 {
		t.Fatalf("expected no servers left, got %+v", finalListed.Servers)
	}
}

func doJSON(t *testing.T, server *httptest.Server, method, path string, body any) *http.Response {
	t.Helper()

	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body: %v", err)
		}
	}

	req, err := http.NewRequest(method, server.URL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build request: %v", err)
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
		t.Fatalf("decode response body: %v", err)
	}
}

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
	return &fakeStore{
		nextID:  1,
		servers: make(map[int64]store.Server),
	}
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

	results := s.filteredLocked(query, tags, limit)
	return results, nil
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
		ID:          s.nextID,
		Name:        input.Name,
		TailscaleIP: input.TailscaleIP,
		SSHUser:     input.SSHUser,
		Tags:        copySlice(input.Tags),
		CreatedAt:   now,
		UpdatedAt:   now,
		CreatedBy:   actor,
		UpdatedBy:   actor,
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
		for _, serverTag := range serverTags {
			if strings.EqualFold(tag, serverTag) {
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
