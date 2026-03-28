package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewHTTPClient_ValidURL(t *testing.T) {
	c, err := NewHTTPClient("http://example.com:8080", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.baseURL.Host != "example.com:8080" {
		t.Fatalf("unexpected host: %s", c.baseURL.Host)
	}
}

func TestNewHTTPClient_InvalidURL(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"just-a-host",
		"://missing-scheme",
	}
	for _, u := range cases {
		_, err := NewHTTPClient(u, nil)
		if err == nil {
			t.Errorf("expected error for %q", u)
		}
	}
}

func TestMe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/me" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(MeResponse{
			LoginName:   "alice@example.com",
			DisplayName: "Alice",
			NodeName:    "alice-laptop",
			Tailnet:     "example.com",
		})
	}))
	defer srv.Close()

	c, _ := NewHTTPClient(srv.URL, srv.Client())
	me, err := c.Me(context.Background())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if me.LoginName != "alice@example.com" {
		t.Fatalf("unexpected login: %s", me.LoginName)
	}
	if me.Tailnet != "example.com" {
		t.Fatalf("unexpected tailnet: %s", me.Tailnet)
	}
}

func TestListServers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/servers" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		tags := r.URL.Query()["tag"]
		if len(tags) != 1 || tags[0] != "prod" {
			t.Errorf("expected tag=prod, got %v", tags)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"servers": []Server{
				{ID: 1, Name: "db-1", TailscaleIP: "100.64.0.1", SSHUser: "root"},
				{ID: 2, Name: "db-2", TailscaleIP: "100.64.0.2", SSHUser: "root"},
			},
		})
	}))
	defer srv.Close()

	c, _ := NewHTTPClient(srv.URL, srv.Client())
	servers, err := c.ListServers(context.Background(), []string{"prod"})
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
	if servers[0].Name != "db-1" {
		t.Fatalf("unexpected name: %s", servers[0].Name)
	}
}

func TestSearchServers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		limit := r.URL.Query().Get("limit")
		if q != "prod" {
			t.Errorf("expected q=prod, got %q", q)
		}
		if limit != "10" {
			t.Errorf("expected limit=10, got %q", limit)
		}
		json.NewEncoder(w).Encode(map[string]any{
			"servers": []Server{{ID: 1, Name: "prod-web"}},
		})
	}))
	defer srv.Close()

	c, _ := NewHTTPClient(srv.URL, srv.Client())
	servers, err := c.SearchServers(context.Background(), "prod", nil, 10)
	if err != nil {
		t.Fatalf("SearchServers: %v", err)
	}
	if len(servers) != 1 || servers[0].Name != "prod-web" {
		t.Fatalf("unexpected result: %+v", servers)
	}
}

func TestAddServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		ct := r.Header.Get("Content-Type")
		if !strings.Contains(ct, "application/json") {
			t.Errorf("expected JSON content-type, got %q", ct)
		}
		var req CreateServerRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Name != "web" || req.TailscaleIP != "100.64.0.5" {
			t.Errorf("unexpected body: %+v", req)
		}
		json.NewEncoder(w).Encode(Server{ID: 42, Name: req.Name, TailscaleIP: req.TailscaleIP, SSHUser: req.SSHUser})
	}))
	defer srv.Close()

	c, _ := NewHTTPClient(srv.URL, srv.Client())
	server, err := c.AddServer(context.Background(), CreateServerRequest{
		Name: "web", TailscaleIP: "100.64.0.5", SSHUser: "deploy",
	})
	if err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	if server.ID != 42 || server.Name != "web" {
		t.Fatalf("unexpected: %+v", server)
	}
}

func TestUpdateServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", r.Method)
		}
		if r.URL.Path != "/servers/7" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(Server{ID: 7, Name: "updated"})
	}))
	defer srv.Close()

	c, _ := NewHTTPClient(srv.URL, srv.Client())
	name := "updated"
	server, err := c.UpdateServer(context.Background(), 7, UpdateServerRequest{Name: &name})
	if err != nil {
		t.Fatalf("UpdateServer: %v", err)
	}
	if server.Name != "updated" {
		t.Fatalf("unexpected name: %s", server.Name)
	}
}

func TestDeleteServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/servers/3" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c, _ := NewHTTPClient(srv.URL, srv.Client())
	if err := c.DeleteServer(context.Background(), 3); err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}
}

func TestAPIErrorJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "not authorized"})
	}))
	defer srv.Close()

	c, _ := NewHTTPClient(srv.URL, srv.Client())
	_, err := c.Me(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not authorized") {
		t.Fatalf("expected API error message, got: %v", err)
	}
}

func TestAPIErrorNonJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	c, _ := NewHTTPClient(srv.URL, srv.Client())
	_, err := c.Me(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected status code in error, got: %v", err)
	}
}

func TestListServers_NoTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query params, got %q", r.URL.RawQuery)
		}
		json.NewEncoder(w).Encode(map[string]any{"servers": []Server{}})
	}))
	defer srv.Close()

	c, _ := NewHTTPClient(srv.URL, srv.Client())
	servers, err := c.ListServers(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 0 {
		t.Fatalf("expected empty, got %d", len(servers))
	}
}
