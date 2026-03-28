package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------- Identity.Actor ----------

func TestActor_PrefersLoginName(t *testing.T) {
	id := Identity{LoginName: "alice@co.com", DisplayName: "Alice", NodeName: "laptop"}
	if id.Actor() != "alice@co.com" {
		t.Fatalf("expected login name, got %q", id.Actor())
	}
}

func TestActor_FallsToDisplayName(t *testing.T) {
	id := Identity{DisplayName: "Alice", NodeName: "laptop"}
	if id.Actor() != "Alice" {
		t.Fatalf("expected display name, got %q", id.Actor())
	}
}

func TestActor_FallsToNodeName(t *testing.T) {
	id := Identity{NodeName: "laptop"}
	if id.Actor() != "laptop" {
		t.Fatalf("expected node name, got %q", id.Actor())
	}
}

func TestActor_Empty(t *testing.T) {
	empty := Identity{}
	if empty.Actor() != "" {
		t.Fatal("expected empty actor")
	}
}

// ---------- Context round-trip ----------

func TestContextRoundTrip(t *testing.T) {
	identity := Identity{LoginName: "a@b.com", DisplayName: "A", NodeName: "n"}
	ctx := WithIdentity(context.Background(), identity)
	got, ok := IdentityFromContext(ctx)
	if !ok {
		t.Fatal("expected identity in context")
	}
	if got.LoginName != "a@b.com" || got.DisplayName != "A" || got.NodeName != "n" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestContextMissing(t *testing.T) {
	_, ok := IdentityFromContext(context.Background())
	if ok {
		t.Fatal("expected no identity")
	}
}

// ---------- firstLabel ----------

func TestFirstLabel_WithDot(t *testing.T) {
	if firstLabel("laptop.tail1234.ts.net") != "laptop" {
		t.Fatal("expected first label before dot")
	}
}

func TestFirstLabel_NoDot(t *testing.T) {
	if firstLabel("laptop") != "laptop" {
		t.Fatal("expected full name when no dot")
	}
}

func TestFirstLabel_Empty(t *testing.T) {
	if firstLabel("") != "" {
		t.Fatal("expected empty string")
	}
}

func TestFirstLabel_LeadingDot(t *testing.T) {
	if firstLabel(".hidden") != "" {
		t.Fatal("expected empty for leading dot")
	}
}

// ---------- Middleware ----------

func TestMiddleware_PassesIdentity(t *testing.T) {
	resolver := &fakeResolver{identity: &Identity{LoginName: "test@co.com"}}
	var gotIdentity Identity
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := IdentityFromContext(r.Context())
		if ok {
			gotIdentity = id
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := Middleware(resolver, inner)
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if gotIdentity.LoginName != "test@co.com" {
		t.Fatalf("identity not propagated: %+v", gotIdentity)
	}
}

func TestMiddleware_Rejects401(t *testing.T) {
	resolver := &fakeResolver{err: errors.New("whois failed")}
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("inner handler should not be called")
	})

	handler := Middleware(resolver, inner)
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}

	var body map[string]string
	json.NewDecoder(rec.Body).Decode(&body)
	if !strings.Contains(body["error"], "tailscale") {
		t.Fatalf("expected tailscale error message: %v", body)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("expected JSON content-type: %s", rec.Header().Get("Content-Type"))
	}
}

func TestMiddleware_NilResolverPassesThrough(t *testing.T) {
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	handler := Middleware(nil, inner)
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected inner handler to be called")
	}
}

// ---------- fakes ----------

type fakeResolver struct {
	identity *Identity
	err      error
}

func (f *fakeResolver) Resolve(ctx context.Context, remoteAddr string) (*Identity, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.identity, nil
}
