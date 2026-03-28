package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"timmy/cli/internal/client"
)

// ---------- helpers ----------

func newTestApp(svc client.Service, runner *fakeSSHRunner) (*App, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	app := New(runner, nil, &stdout, &stderr)
	app.lazyClient = svc
	return app, &stdout, &stderr
}

func run(t *testing.T, app *App, args ...string) {
	t.Helper()
	if err := app.Run(context.Background(), args); err != nil {
		t.Fatalf("Run(%v): %v", args, err)
	}
}

func runErr(t *testing.T, app *App, args ...string) error {
	t.Helper()
	return app.Run(context.Background(), args)
}

// ---------- routing ----------

func TestRun_NoArgs(t *testing.T) {
	app, _, _ := newTestApp(&fakeClient{}, &fakeSSHRunner{})
	if err := runErr(t, app); err == nil {
		t.Fatal("expected error for no args")
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	app, _, _ := newTestApp(&fakeClient{}, &fakeSSHRunner{})
	if err := runErr(t, app, "bogus"); err == nil {
		t.Fatal("expected error for unknown command")
	}
}

func TestRun_Help(t *testing.T) {
	app, _, stderr := newTestApp(&fakeClient{}, &fakeSSHRunner{})
	run(t, app, "help")
	if !strings.Contains(stderr.String(), "timmy init") {
		t.Fatalf("help output missing init: %s", stderr.String())
	}
}

func TestRun_HelpFlags(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		app, _, stderr := newTestApp(&fakeClient{}, &fakeSSHRunner{})
		run(t, app, flag)
		if !strings.Contains(stderr.String(), "Usage:") {
			t.Fatalf("%s did not print usage", flag)
		}
	}
}

// ---------- me ----------

func TestRunMe_Text(t *testing.T) {
	svc := &fakeClient{me: client.MeResponse{
		LoginName: "alice@corp.com", DisplayName: "Alice", NodeName: "alice-mac", Tailnet: "corp.com",
	}}
	app, stdout, _ := newTestApp(svc, &fakeSSHRunner{})
	run(t, app, "me")
	out := stdout.String()
	for _, want := range []string{"alice@corp.com", "Alice", "alice-mac", "corp.com"} {
		if !strings.Contains(out, want) {
			t.Errorf("me output missing %q: %s", want, out)
		}
	}
}

func TestRunMe_JSON(t *testing.T) {
	svc := &fakeClient{me: client.MeResponse{LoginName: "bob@co.com"}}
	app, stdout, _ := newTestApp(svc, &fakeSSHRunner{})
	run(t, app, "me", "--json")
	var parsed client.MeResponse
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed.LoginName != "bob@co.com" {
		t.Fatalf("unexpected: %+v", parsed)
	}
}

// ---------- add ----------

func TestRunAdd_Success(t *testing.T) {
	svc := &fakeClient{added: client.Server{ID: 99, Name: "web", TailscaleIP: "100.64.0.5", SSHUser: "deploy"}}
	app, stdout, _ := newTestApp(svc, &fakeSSHRunner{})
	run(t, app, "add", "--name", "web", "--ip", "100.64.0.5", "--user", "deploy", "--tag", "prod", "--json")

	var parsed client.Server
	if err := json.Unmarshal(stdout.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if parsed.ID != 99 || parsed.Name != "web" {
		t.Fatalf("unexpected: %+v", parsed)
	}
	if svc.addRequest.Tags[0] != "prod" {
		t.Fatalf("tag not passed: %+v", svc.addRequest)
	}
}

func TestRunAdd_MissingName(t *testing.T) {
	app, _, _ := newTestApp(&fakeClient{}, &fakeSSHRunner{})
	err := runErr(t, app, "add", "--ip", "100.64.0.1")
	if err == nil || !strings.Contains(err.Error(), "--name") {
		t.Fatalf("expected name error, got: %v", err)
	}
}

func TestRunAdd_MissingIP(t *testing.T) {
	app, _, _ := newTestApp(&fakeClient{}, &fakeSSHRunner{})
	err := runErr(t, app, "add", "--name", "foo")
	if err == nil || !strings.Contains(err.Error(), "--ip") {
		t.Fatalf("expected ip error, got: %v", err)
	}
}

func TestRunAdd_DefaultUser(t *testing.T) {
	svc := &fakeClient{added: client.Server{ID: 1, Name: "x", SSHUser: "root"}}
	app, _, _ := newTestApp(svc, &fakeSSHRunner{})
	run(t, app, "add", "--name", "x", "--ip", "100.64.0.1", "--json")
	if svc.addRequest.SSHUser != "root" {
		t.Fatalf("expected default user=root, got %q", svc.addRequest.SSHUser)
	}
}

// ---------- ls ----------

func TestRunList_Table(t *testing.T) {
	svc := &fakeClient{listServers: []client.Server{
		{ID: 1, Name: "alpha", TailscaleIP: "100.64.0.1", SSHUser: "root", Tags: []string{"prod"}},
		{ID: 2, Name: "beta", TailscaleIP: "100.64.0.2", SSHUser: "deploy", Tags: []string{"staging"}},
	}}
	app, stdout, _ := newTestApp(svc, &fakeSSHRunner{})
	run(t, app, "ls")
	out := stdout.String()
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Fatalf("missing servers: %s", out)
	}
	if !strings.Contains(out, "ID") || !strings.Contains(out, "NAME") {
		t.Fatalf("missing header: %s", out)
	}
}

func TestRunList_JSON(t *testing.T) {
	svc := &fakeClient{listServers: []client.Server{
		{ID: 1, Name: "prod-db-1", TailscaleIP: "100.64.0.10", SSHUser: "root", Tags: []string{"prod", "database"}},
	}}
	app, stdout, _ := newTestApp(svc, &fakeSSHRunner{})
	run(t, app, "ls", "--json")
	output := stdout.String()
	if !strings.Contains(output, `"servers"`) || !strings.Contains(output, `"prod-db-1"`) {
		t.Fatalf("unexpected JSON output: %s", output)
	}
}

func TestRunList_Empty(t *testing.T) {
	app, stdout, _ := newTestApp(&fakeClient{}, &fakeSSHRunner{})
	run(t, app, "ls")
	if !strings.Contains(stdout.String(), "No servers found") {
		t.Fatalf("expected empty message: %s", stdout.String())
	}
}

func TestRunList_RejectsPositionalArgs(t *testing.T) {
	app, _, _ := newTestApp(&fakeClient{}, &fakeSSHRunner{})
	err := runErr(t, app, "ls", "extra")
	if err == nil {
		t.Fatal("expected error for positional args")
	}
}

func TestRunList_TagFiltering(t *testing.T) {
	svc := &fakeClient{listServers: []client.Server{{ID: 1, Name: "x"}}}
	app, _, _ := newTestApp(svc, &fakeSSHRunner{})
	run(t, app, "ls", "--tag", "prod", "--tag", "us-east", "--json")
	if len(svc.listTags) != 2 || svc.listTags[0] != "prod" || svc.listTags[1] != "us-east" {
		t.Fatalf("tags not passed: %v", svc.listTags)
	}
}

// ---------- search ----------

func TestRunSearch_JSON(t *testing.T) {
	svc := &fakeClient{searchServers: []client.Server{{ID: 1, Name: "match"}}}
	app, stdout, _ := newTestApp(svc, &fakeSSHRunner{})
	run(t, app, "search", "--json", "match")

	var parsed map[string]any
	json.Unmarshal(stdout.Bytes(), &parsed)
	if parsed["query"] != "match" {
		t.Fatalf("expected query in output: %s", stdout.String())
	}
}

func TestRunSearch_MissingQuery(t *testing.T) {
	app, _, _ := newTestApp(&fakeClient{}, &fakeSSHRunner{})
	err := runErr(t, app, "search")
	if err == nil || !strings.Contains(err.Error(), "query") {
		t.Fatalf("expected query error, got: %v", err)
	}
}

func TestRunSearch_PassesLimitAndTags(t *testing.T) {
	svc := &fakeClient{searchServers: []client.Server{{ID: 1, Name: "x"}}}
	app, _, _ := newTestApp(svc, &fakeSSHRunner{})
	run(t, app, "search", "--limit", "5", "--tag", "db", "--json", "q")
	if svc.searchLimit != 5 {
		t.Fatalf("expected limit=5, got %d", svc.searchLimit)
	}
	if len(svc.searchTags) != 1 || svc.searchTags[0] != "db" {
		t.Fatalf("expected tags=[db], got %v", svc.searchTags)
	}
}

// ---------- ssh ----------

func TestRunSSH_BuildsDestination(t *testing.T) {
	svc := &fakeClient{searchServers: []client.Server{
		{ID: 1, Name: "prod-db-1", TailscaleIP: "100.64.0.10", SSHUser: "root", Tags: []string{"prod"}},
	}}
	runner := &fakeSSHRunner{}
	app, _, _ := newTestApp(svc, runner)
	run(t, app, "ssh", "prod-db-1")
	if runner.destination != "root@100.64.0.10" {
		t.Fatalf("unexpected SSH destination %q", runner.destination)
	}
}

func TestRunSSH_MissingQuery(t *testing.T) {
	app, _, _ := newTestApp(&fakeClient{}, &fakeSSHRunner{})
	err := runErr(t, app, "ssh")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunSSH_ExactFlag(t *testing.T) {
	svc := &fakeClient{searchServers: []client.Server{
		{ID: 1, Name: "prod-db-1", TailscaleIP: "100.64.0.10", SSHUser: "root"},
		{ID: 2, Name: "prod-db-2", TailscaleIP: "100.64.0.11", SSHUser: "root"},
	}}
	app, _, _ := newTestApp(svc, &fakeSSHRunner{})
	err := runErr(t, app, "ssh", "--exact", "prod")
	if err == nil || !strings.Contains(err.Error(), "did not match") {
		t.Fatalf("expected exact-only error, got: %v", err)
	}
}

func TestRunSSH_FirstFlag(t *testing.T) {
	svc := &fakeClient{searchServers: []client.Server{
		{ID: 1, Name: "prod-db-1", TailscaleIP: "100.64.0.10", SSHUser: "root"},
		{ID: 2, Name: "prod-db-2", TailscaleIP: "100.64.0.11", SSHUser: "deploy"},
	}}
	runner := &fakeSSHRunner{}
	app, _, _ := newTestApp(svc, runner)
	run(t, app, "ssh", "--first", "prod")
	if runner.destination != "root@100.64.0.10" {
		t.Fatalf("expected first match, got %q", runner.destination)
	}
}

// ---------- update ----------

func TestRunUpdate_Success(t *testing.T) {
	svc := &fakeClient{
		searchServers: []client.Server{{ID: 5, Name: "web", TailscaleIP: "100.64.0.5", SSHUser: "root"}},
		updated:       client.Server{ID: 5, Name: "web-renamed"},
	}
	app, stdout, _ := newTestApp(svc, &fakeSSHRunner{})
	run(t, app, "update", "--name", "web-renamed", "--json", "web")

	var parsed client.Server
	json.Unmarshal(stdout.Bytes(), &parsed)
	if parsed.Name != "web-renamed" {
		t.Fatalf("unexpected: %+v", parsed)
	}
}

func TestRunUpdate_NoFields(t *testing.T) {
	app, _, _ := newTestApp(&fakeClient{}, &fakeSSHRunner{})
	err := runErr(t, app, "update", "web")
	if err == nil || !strings.Contains(err.Error(), "at least one field") {
		t.Fatalf("expected field error, got: %v", err)
	}
}

func TestRunUpdate_MissingQuery(t *testing.T) {
	app, _, _ := newTestApp(&fakeClient{}, &fakeSSHRunner{})
	err := runErr(t, app, "update")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunUpdate_ClearTags(t *testing.T) {
	svc := &fakeClient{
		searchServers: []client.Server{{ID: 1, Name: "x", TailscaleIP: "100.64.0.1", SSHUser: "root"}},
		updated:       client.Server{ID: 1, Name: "x"},
	}
	app, _, _ := newTestApp(svc, &fakeSSHRunner{})
	run(t, app, "update", "--clear-tags", "--json", "x")
	if svc.updateRequest.Tags == nil || len(*svc.updateRequest.Tags) != 0 {
		t.Fatalf("expected empty tags, got %+v", svc.updateRequest.Tags)
	}
}

// ---------- rm ----------

func TestRunRemove_Text(t *testing.T) {
	svc := &fakeClient{
		searchServers: []client.Server{{ID: 3, Name: "old-server", TailscaleIP: "100.64.0.3", SSHUser: "root"}},
	}
	app, stdout, _ := newTestApp(svc, &fakeSSHRunner{})
	run(t, app, "rm", "old-server")
	if !strings.Contains(stdout.String(), "Deleted old-server") {
		t.Fatalf("unexpected output: %s", stdout.String())
	}
	if svc.deleteID != 3 {
		t.Fatalf("expected delete ID=3, got %d", svc.deleteID)
	}
}

func TestRunRemove_JSON(t *testing.T) {
	svc := &fakeClient{
		searchServers: []client.Server{{ID: 7, Name: "gone", TailscaleIP: "100.64.0.7", SSHUser: "root"}},
	}
	app, stdout, _ := newTestApp(svc, &fakeSSHRunner{})
	run(t, app, "rm", "--json", "gone")
	var parsed map[string]any
	json.Unmarshal(stdout.Bytes(), &parsed)
	if parsed["deleted"] != true {
		t.Fatalf("expected deleted=true: %s", stdout.String())
	}
}

func TestRunRemove_MissingQuery(t *testing.T) {
	app, _, _ := newTestApp(&fakeClient{}, &fakeSSHRunner{})
	err := runErr(t, app, "rm")
	if err == nil {
		t.Fatal("expected error")
	}
}

// ---------- command aliases ----------

func TestCommandAliases(t *testing.T) {
	svc := &fakeClient{listServers: []client.Server{{ID: 1, Name: "x"}}}
	for _, alias := range []string{"list", "ls"} {
		app, stdout, _ := newTestApp(svc, &fakeSSHRunner{})
		run(t, app, alias, "--json")
		if !strings.Contains(stdout.String(), `"servers"`) {
			t.Fatalf("%s alias broken", alias)
		}
	}

	svc2 := &fakeClient{searchServers: []client.Server{{ID: 1, Name: "x", TailscaleIP: "100.64.0.1", SSHUser: "root"}}}
	for _, alias := range []string{"rm", "delete", "remove"} {
		app, _, _ := newTestApp(svc2, &fakeSSHRunner{})
		run(t, app, alias, "--json", "x")
	}
}

// ---------- resolveServerQuery ----------

func TestResolve_ExactNameMatch(t *testing.T) {
	svc := &fakeClient{searchServers: []client.Server{
		{ID: 1, Name: "prod-db-1", TailscaleIP: "100.64.0.10", SSHUser: "root", Tags: []string{"prod"}},
		{ID: 2, Name: "prod", TailscaleIP: "100.64.0.11", SSHUser: "root", Tags: []string{"ops"}},
	}}
	app, _, _ := newTestApp(svc, &fakeSSHRunner{})
	server, err := app.resolveServerQuery(context.Background(), svc, "prod", false, false)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if server.ID != 2 {
		t.Fatalf("expected exact name match (ID=2), got %+v", server)
	}
}

func TestResolve_ExactIPMatch(t *testing.T) {
	svc := &fakeClient{searchServers: []client.Server{
		{ID: 1, Name: "a", TailscaleIP: "100.64.0.10", SSHUser: "root"},
		{ID: 2, Name: "b", TailscaleIP: "100.64.0.99", SSHUser: "root"},
	}}
	app, _, _ := newTestApp(svc, &fakeSSHRunner{})
	server, err := app.resolveServerQuery(context.Background(), svc, "100.64.0.99", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if server.ID != 2 {
		t.Fatalf("expected IP match, got ID=%d", server.ID)
	}
}

func TestResolve_SecondaryTagMatch(t *testing.T) {
	svc := &fakeClient{searchServers: []client.Server{
		{ID: 1, Name: "server-a", TailscaleIP: "100.64.0.1", SSHUser: "root", Tags: []string{"unique-tag"}},
		{ID: 2, Name: "server-b", TailscaleIP: "100.64.0.2", SSHUser: "root", Tags: []string{"other"}},
	}}
	app, _, _ := newTestApp(svc, &fakeSSHRunner{})
	server, err := app.resolveServerQuery(context.Background(), svc, "unique-tag", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if server.ID != 1 {
		t.Fatalf("expected tag match, got ID=%d", server.ID)
	}
}

func TestResolve_ByNumericID(t *testing.T) {
	svc := &fakeClient{listServers: []client.Server{
		{ID: 42, Name: "target", TailscaleIP: "100.64.0.42", SSHUser: "root"},
	}}
	app, _, _ := newTestApp(svc, &fakeSSHRunner{})
	server, err := app.resolveServerQuery(context.Background(), svc, "42", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if server.ID != 42 {
		t.Fatalf("expected ID=42, got %d", server.ID)
	}
}

func TestResolve_NumericIDNotFound(t *testing.T) {
	svc := &fakeClient{listServers: []client.Server{{ID: 1, Name: "only"}}}
	app, _, _ := newTestApp(svc, &fakeSSHRunner{})
	_, err := app.resolveServerQuery(context.Background(), svc, "999", false, false)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected not found, got: %v", err)
	}
}

func TestResolve_NoMatches(t *testing.T) {
	svc := &fakeClient{}
	app, _, _ := newTestApp(svc, &fakeSSHRunner{})
	_, err := app.resolveServerQuery(context.Background(), svc, "ghost", false, false)
	if err == nil || !strings.Contains(err.Error(), "no servers matched") {
		t.Fatalf("expected no match error, got: %v", err)
	}
}

func TestResolve_MultiplePrimaryMatches(t *testing.T) {
	svc := &fakeClient{searchServers: []client.Server{
		{ID: 1, Name: "dup", TailscaleIP: "100.64.0.1", SSHUser: "root"},
		{ID: 2, Name: "dup", TailscaleIP: "100.64.0.2", SSHUser: "root"},
	}}
	app, _, _ := newTestApp(svc, &fakeSSHRunner{})
	_, err := app.resolveServerQuery(context.Background(), svc, "dup", false, false)
	if err == nil || !strings.Contains(err.Error(), "matched 2 servers exactly") {
		t.Fatalf("expected ambiguity error, got: %v", err)
	}
}

func TestResolve_MultipleSecondaryMatches(t *testing.T) {
	svc := &fakeClient{searchServers: []client.Server{
		{ID: 1, Name: "a", TailscaleIP: "100.64.0.1", SSHUser: "root", Tags: []string{"shared-tag"}},
		{ID: 2, Name: "b", TailscaleIP: "100.64.0.2", SSHUser: "root", Tags: []string{"shared-tag"}},
	}}
	app, _, _ := newTestApp(svc, &fakeSSHRunner{})
	_, err := app.resolveServerQuery(context.Background(), svc, "shared-tag", false, false)
	if err == nil || !strings.Contains(err.Error(), "matched 2") {
		t.Fatalf("expected ambiguity error, got: %v", err)
	}
}

func TestResolve_FuzzyMultipleWithoutFirst(t *testing.T) {
	svc := &fakeClient{searchServers: []client.Server{
		{ID: 1, Name: "prod-db-1", TailscaleIP: "100.64.0.1", SSHUser: "root"},
		{ID: 2, Name: "prod-db-2", TailscaleIP: "100.64.0.2", SSHUser: "root"},
	}}
	app, _, _ := newTestApp(svc, &fakeSSHRunner{})
	_, err := app.resolveServerQuery(context.Background(), svc, "prod-db", false, false)
	if err == nil || !strings.Contains(err.Error(), "--first") {
		t.Fatalf("expected hint to use --first, got: %v", err)
	}
}

func TestResolve_FuzzyMultipleWithFirst(t *testing.T) {
	svc := &fakeClient{searchServers: []client.Server{
		{ID: 1, Name: "prod-db-1", TailscaleIP: "100.64.0.1", SSHUser: "root"},
		{ID: 2, Name: "prod-db-2", TailscaleIP: "100.64.0.2", SSHUser: "root"},
	}}
	app, _, _ := newTestApp(svc, &fakeSSHRunner{})
	server, err := app.resolveServerQuery(context.Background(), svc, "prod-db", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if server.ID != 1 {
		t.Fatalf("expected first match, got ID=%d", server.ID)
	}
}

func TestResolve_ExactOnlyRejectsFuzzy(t *testing.T) {
	svc := &fakeClient{searchServers: []client.Server{
		{ID: 1, Name: "prod-db-1", TailscaleIP: "100.64.0.1", SSHUser: "root"},
	}}
	app, _, _ := newTestApp(svc, &fakeSSHRunner{})
	_, err := app.resolveServerQuery(context.Background(), svc, "prod", true, false)
	if err == nil || !strings.Contains(err.Error(), "did not match") {
		t.Fatalf("expected exact-only rejection, got: %v", err)
	}
}

func TestResolve_EmptyQuery(t *testing.T) {
	app, _, _ := newTestApp(&fakeClient{}, &fakeSSHRunner{})
	_, err := app.resolveServerQuery(context.Background(), &fakeClient{}, "  ", false, false)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty error, got: %v", err)
	}
}

func TestResolve_CaseInsensitive(t *testing.T) {
	svc := &fakeClient{searchServers: []client.Server{
		{ID: 1, Name: "Prod-DB", TailscaleIP: "100.64.0.1", SSHUser: "root"},
	}}
	app, _, _ := newTestApp(svc, &fakeSSHRunner{})
	server, err := app.resolveServerQuery(context.Background(), svc, "prod-db", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if server.ID != 1 {
		t.Fatal("case-insensitive match failed")
	}
}

// ---------- fan-out ----------

func TestFanOutList_MergesMultipleClients(t *testing.T) {
	app := New(&fakeSSHRunner{}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	app.allClientsOverride = []namedClient{
		{name: "alpha", svc: &fakeClient{listServers: []client.Server{{ID: 1, Name: "a-server"}}}},
		{name: "beta", svc: &fakeClient{listServers: []client.Server{{ID: 2, Name: "b-server"}}}},
	}

	servers, err := app.fanOutList(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2 merged servers, got %d", len(servers))
	}
	names := map[string]bool{}
	for _, s := range servers {
		names[s.Name] = true
	}
	if !names["a-server"] || !names["b-server"] {
		t.Fatalf("missing servers: %v", names)
	}
}

func TestFanOutSearch_MergesResults(t *testing.T) {
	app := New(&fakeSSHRunner{}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	app.allClientsOverride = []namedClient{
		{name: "one", svc: &fakeClient{searchServers: []client.Server{{ID: 10, Name: "match-a"}}}},
		{name: "two", svc: &fakeClient{searchServers: []client.Server{{ID: 20, Name: "match-b"}}}},
	}

	servers, err := app.fanOutSearch(context.Background(), "match", nil, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 {
		t.Fatalf("expected 2, got %d", len(servers))
	}
}

func TestFanOut_PartialFailure(t *testing.T) {
	var stderr bytes.Buffer
	app := New(&fakeSSHRunner{}, nil, &bytes.Buffer{}, &stderr)
	app.allClientsOverride = []namedClient{
		{name: "good", svc: &fakeClient{listServers: []client.Server{{ID: 1, Name: "ok"}}}},
		{name: "bad", svc: &fakeClient{listErr: errors.New("connection refused")}},
	}

	servers, err := app.fanOutList(context.Background(), nil)
	if err != nil {
		t.Fatalf("should succeed with partial results: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 server from good backend, got %d", len(servers))
	}
	if !strings.Contains(stderr.String(), "warning") {
		t.Fatalf("expected warning on stderr: %s", stderr.String())
	}
}

func TestFanOut_AllFail(t *testing.T) {
	app := New(&fakeSSHRunner{}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	app.allClientsOverride = []namedClient{
		{name: "broken", svc: &fakeClient{listErr: errors.New("down")}},
	}

	_, err := app.fanOutList(context.Background(), nil)
	if err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("expected unreachable error, got: %v", err)
	}
}

func TestFanOut_SetsSourceField(t *testing.T) {
	app := New(&fakeSSHRunner{}, nil, &bytes.Buffer{}, &bytes.Buffer{})
	app.allClientsOverride = []namedClient{
		{name: "my-backend", svc: &fakeClient{listServers: []client.Server{{ID: 1, Name: "x"}}}},
	}

	servers, _ := app.fanOutList(context.Background(), nil)
	if servers[0].Source != "my-backend" {
		t.Fatalf("expected source=my-backend, got %q", servers[0].Source)
	}
}

// ---------- render ----------

func TestRenderServers_NoSourceColumn(t *testing.T) {
	app, stdout, _ := newTestApp(&fakeClient{}, &fakeSSHRunner{})
	app.renderServers([]client.Server{
		{ID: 1, Name: "x", TailscaleIP: "100.64.0.1", SSHUser: "root"},
	})
	if strings.Contains(stdout.String(), "SERVER") {
		t.Fatalf("should not show SERVER column without source: %s", stdout.String())
	}
}

func TestRenderServers_WithSourceColumn(t *testing.T) {
	app, stdout, _ := newTestApp(&fakeClient{}, &fakeSSHRunner{})
	app.renderServers([]client.Server{
		{ID: 1, Name: "x", TailscaleIP: "100.64.0.1", SSHUser: "root", Source: "alpha"},
		{ID: 2, Name: "y", TailscaleIP: "100.64.0.2", SSHUser: "root", Source: "beta"},
	})
	out := stdout.String()
	if !strings.Contains(out, "SERVER") {
		t.Fatalf("expected SERVER column: %s", out)
	}
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Fatalf("expected source names: %s", out)
	}
}

func TestRenderServers_Empty(t *testing.T) {
	app, stdout, _ := newTestApp(&fakeClient{}, &fakeSSHRunner{})
	app.renderServers(nil)
	if !strings.Contains(stdout.String(), "No servers found") {
		t.Fatalf("expected empty message: %s", stdout.String())
	}
}

func TestRenderMe(t *testing.T) {
	app, stdout, _ := newTestApp(&fakeClient{}, &fakeSSHRunner{})
	app.renderMe(client.MeResponse{
		LoginName: "test@example.com", DisplayName: "Test",
		NodeName: "test-node", Tailnet: "example.com",
	})
	out := stdout.String()
	if !strings.Contains(out, "test@example.com") || !strings.Contains(out, "example.com") {
		t.Fatalf("unexpected render: %s", out)
	}
}

func TestRenderMe_EmptyFields(t *testing.T) {
	app, stdout, _ := newTestApp(&fakeClient{}, &fakeSSHRunner{})
	app.renderMe(client.MeResponse{})
	if !strings.Contains(stdout.String(), "-") {
		t.Fatalf("expected dash fallback for empty fields: %s", stdout.String())
	}
}

// ---------- fakes ----------

type fakeClient struct {
	me            client.MeResponse
	meErr         error
	listServers   []client.Server
	listErr       error
	listTags      []string
	searchServers []client.Server
	searchErr     error
	searchLimit   int
	searchTags    []string
	added         client.Server
	addRequest    client.CreateServerRequest
	updated       client.Server
	updateRequest client.UpdateServerRequest
	deleteID      int64
}

func (f *fakeClient) Me(ctx context.Context) (client.MeResponse, error) {
	if f.meErr != nil {
		return client.MeResponse{}, f.meErr
	}
	return f.me, nil
}

func (f *fakeClient) ListServers(ctx context.Context, tags []string) ([]client.Server, error) {
	f.listTags = tags
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]client.Server(nil), f.listServers...), nil
}

func (f *fakeClient) SearchServers(ctx context.Context, query string, tags []string, limit int) ([]client.Server, error) {
	f.searchTags = tags
	f.searchLimit = limit
	if f.searchErr != nil {
		return nil, f.searchErr
	}
	return append([]client.Server(nil), f.searchServers...), nil
}

func (f *fakeClient) AddServer(ctx context.Context, request client.CreateServerRequest) (client.Server, error) {
	f.addRequest = request
	return f.added, nil
}

func (f *fakeClient) UpdateServer(ctx context.Context, id int64, request client.UpdateServerRequest) (client.Server, error) {
	f.updateRequest = request
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

