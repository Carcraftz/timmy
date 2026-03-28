package auth

import "context"

type Identity struct {
	LoginName   string `json:"login_name"`
	DisplayName string `json:"display_name"`
	NodeName    string `json:"node_name"`
}

func (i Identity) Actor() string {
	switch {
	case i.LoginName != "":
		return i.LoginName
	case i.DisplayName != "":
		return i.DisplayName
	default:
		return i.NodeName
	}
}

type contextKey struct{}

func WithIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, contextKey{}, identity)
}

func IdentityFromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(contextKey{}).(Identity)
	return identity, ok
}
