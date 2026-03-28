package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"timmy/cli/internal/client"
)

func newTestApp(svc client.Service, runner *fakeSSHRunner) (*App, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	app := New(runner, nil, &stdout, &stderr)
	app.lazyClient = svc
	return app, &stdout, &stderr
}

func TestRunListJSONOutput(t *testing.T) {
	service := &fakeClient{
		listServers: []client.Server{
			{ID: 1, Name: "prod-db-1", TailscaleIP: "100.64.0.10", SSHUser: "root", Tags: []string{"prod", "database"}},
		},
	}

	app, stdout, _ := newTestApp(service, &fakeSSHRunner{})

	if err := app.Run(context.Background(), []string{"ls", "--json"}); err != nil {
		t.Fatalf("run ls --json: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, `"servers"`) || !strings.Contains(output, `"prod-db-1"`) {
		t.Fatalf("unexpected JSON output: %s", output)
	}
}

func TestResolveServerQueryPrefersExactMatch(t *testing.T) {
	service := &fakeClient{
		searchServers: []client.Server{
			{ID: 1, Name: "prod-db-1", TailscaleIP: "100.64.0.10", SSHUser: "root", Tags: []string{"prod"}},
			{ID: 2, Name: "prod", TailscaleIP: "100.64.0.11", SSHUser: "root", Tags: []string{"ops"}},
		},
	}

	app, _, _ := newTestApp(service, &fakeSSHRunner{})
	server, err := app.resolveServerQuery(context.Background(), service, "prod", false, false)
	if err != nil {
		t.Fatalf("resolve query: %v", err)
	}
	if server.ID != 2 {
		t.Fatalf("expected exact name match, got %+v", server)
	}
}

func TestRunSSHBuildsOpenSSHDestination(t *testing.T) {
	service := &fakeClient{
		searchServers: []client.Server{
			{ID: 1, Name: "prod-db-1", TailscaleIP: "100.64.0.10", SSHUser: "root", Tags: []string{"prod"}},
		},
	}
	runner := &fakeSSHRunner{}

	app, _, _ := newTestApp(service, runner)
	if err := app.Run(context.Background(), []string{"ssh", "prod-db-1"}); err != nil {
		t.Fatalf("run ssh: %v", err)
	}

	if runner.destination != "root@100.64.0.10" {
		t.Fatalf("unexpected SSH destination %q", runner.destination)
	}
}

type fakeClient struct {
	me            client.MeResponse
	listServers   []client.Server
	searchServers []client.Server
	added         client.Server
	updated       client.Server
	deleteID      int64
}

func (f *fakeClient) Me(ctx context.Context) (client.MeResponse, error) {
	return f.me, nil
}

func (f *fakeClient) ListServers(ctx context.Context, tags []string) ([]client.Server, error) {
	return append([]client.Server(nil), f.listServers...), nil
}

func (f *fakeClient) SearchServers(ctx context.Context, query string, tags []string, limit int) ([]client.Server, error) {
	return append([]client.Server(nil), f.searchServers...), nil
}

func (f *fakeClient) AddServer(ctx context.Context, request client.CreateServerRequest) (client.Server, error) {
	return f.added, nil
}

func (f *fakeClient) UpdateServer(ctx context.Context, id int64, request client.UpdateServerRequest) (client.Server, error) {
	f.updated.ID = id
	return f.updated, nil
}

func (f *fakeClient) DeleteServer(ctx context.Context, id int64) error {
	f.deleteID = id
	return nil
}

type fakeSSHRunner struct {
	destination string
}

func (f *fakeSSHRunner) Run(ctx context.Context, destination string) error {
	f.destination = destination
	return nil
}
