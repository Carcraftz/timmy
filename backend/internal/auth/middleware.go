package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"tailscale.com/client/tailscale/apitype"
)

type Resolver interface {
	Resolve(ctx context.Context, remoteAddr string) (*Identity, error)
}

type WhoIsClient interface {
	WhoIs(ctx context.Context, remoteAddr string) (*apitype.WhoIsResponse, error)
}

type LocalIdentityResolver struct {
	client WhoIsClient
}

func NewLocalIdentityResolver(client WhoIsClient) *LocalIdentityResolver {
	return &LocalIdentityResolver{client: client}
}

func (r *LocalIdentityResolver) Resolve(ctx context.Context, remoteAddr string) (*Identity, error) {
	if r == nil || r.client == nil {
		return nil, errors.New("tailscale identity resolver is not configured")
	}

	who, err := r.client.WhoIs(ctx, remoteAddr)
	if err != nil {
		return nil, err
	}

	identity := &Identity{}
	if who.UserProfile != nil {
		identity.LoginName = strings.TrimSpace(who.UserProfile.LoginName)
		identity.DisplayName = strings.TrimSpace(who.UserProfile.DisplayName)
	}
	if who.Node != nil {
		identity.NodeName = firstLabel(strings.TrimSpace(who.Node.ComputedName))
	}

	if identity.Actor() == "" {
		return nil, errors.New("tailscale identity is empty")
	}

	return identity, nil
}

func Middleware(resolver Resolver, next http.Handler) http.Handler {
	if resolver == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, err := resolver.Resolve(r.Context(), r.RemoteAddr)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unable to verify tailscale identity"})
			return
		}

		next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), *identity)))
	})
}

func firstLabel(name string) string {
	if name == "" {
		return ""
	}
	if idx := strings.IndexByte(name, '.'); idx >= 0 {
		return name[:idx]
	}
	return name
}
