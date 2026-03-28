package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"timmy/backend/internal/auth"
	"timmy/backend/internal/store"
)

type API struct {
	store       store.Store
	tailnetID   int64
	tailnetName string
}

func NewHandler(st store.Store, tailnetID int64, tailnetName string, resolver auth.Resolver) http.Handler {
	api := &API{
		store:       st,
		tailnetID:   tailnetID,
		tailnetName: tailnetName,
	}

	protected := http.NewServeMux()
	protected.HandleFunc("GET /me", api.handleMe)
	protected.HandleFunc("GET /servers", api.handleListServers)
	protected.HandleFunc("GET /servers/search", api.handleSearchServers)
	protected.HandleFunc("POST /servers", api.handleCreateServer)
	protected.HandleFunc("PATCH /servers/{id}", api.handleUpdateServer)
	protected.HandleFunc("DELETE /servers/{id}", api.handleDeleteServer)

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", api.handleHealth)
	root.Handle("/", auth.Middleware(resolver, protected))

	return root
}

func (a *API) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) handleMe(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.IdentityFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "missing request identity")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"login_name":   identity.LoginName,
		"display_name": identity.DisplayName,
		"node_name":    identity.NodeName,
		"tailnet":      a.tailnetName,
	})
}

func (a *API) handleListServers(w http.ResponseWriter, r *http.Request) {
	tags, err := store.NormalizeTags(r.URL.Query()["tag"])
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	servers, err := a.store.ListServers(r.Context(), a.tailnetID, tags)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"servers": servers})
}

func (a *API) handleSearchServers(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}

	tags, err := store.NormalizeTags(r.URL.Query()["tag"])
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	limit := 50
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		value, err := strconv.Atoi(rawLimit)
		if err != nil || value <= 0 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		if value > 200 {
			value = 200
		}
		limit = value
	}

	servers, err := a.store.SearchServers(r.Context(), a.tailnetID, query, tags, limit)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"query":   query,
		"servers": servers,
	})
}

func (a *API) handleCreateServer(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.IdentityFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "missing request identity")
		return
	}

	var input store.CreateServerInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	input, err := store.NormalizeCreateInput(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	server, err := a.store.CreateServer(r.Context(), a.tailnetID, identity.Actor(), input)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, server)
}

func (a *API) handleUpdateServer(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.IdentityFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusInternalServerError, "missing request identity")
		return
	}

	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var input store.UpdateServerInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	input, err = store.NormalizeUpdateInput(input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if input.Name == nil && input.TailscaleIP == nil && input.SSHUser == nil && input.Tags == nil {
		writeError(w, http.StatusBadRequest, "at least one field must be updated")
		return
	}

	server, err := a.store.UpdateServer(r.Context(), a.tailnetID, id, identity.Actor(), input)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, server)
}

func (a *API) handleDeleteServer(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.store.DeleteServer(r.Context(), a.tailnetID, id); err != nil {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"deleted": true, "id": id})
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}

	if decoder.More() {
		return errors.New("request body must contain a single JSON object")
	}

	return nil
}

func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "server not found")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "server already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(payload)
}

func parseID(raw string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("id must be a positive integer")
	}
	return id, nil
}
