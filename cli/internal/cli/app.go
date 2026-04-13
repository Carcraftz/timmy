package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"timmy/cli/internal/client"
	"timmy/cli/internal/config"
	"timmy/cli/internal/ssh"
)

type App struct {
	sshRunner  ssh.Runner
	stdout     io.Writer
	stderr     io.Writer
	httpClient *http.Client

	lazyClient       client.Service
	allClientsOverride []namedClient // testing only: overrides allClients()
}

type namedClient struct {
	name string
	svc  client.Service
}

func New(sshRunner ssh.Runner, httpClient *http.Client, stdout, stderr io.Writer) *App {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &App{
		sshRunner:  sshRunner,
		stdout:     stdout,
		stderr:     stderr,
		httpClient: httpClient,
	}
}

func (a *App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		a.printUsage()
		return errors.New("command is required")
	}

	switch args[0] {
	case "init":
		return a.runInit(args[1:])
	case "servers":
		return a.runServers()
	case "use":
		return a.runUse(args[1:])
	case "uninit":
		return a.runUninit(args[1:])
	case "help", "-h", "--help":
		a.printUsage()
		return nil

	// Fan-out: query all configured servers
	case "ls", "list":
		return a.runList(ctx, args[1:])
	case "search":
		return a.runSearch(ctx, args[1:])
	case "ssh":
		return a.runSSH(ctx, args[1:])

	// Active server only
	case "me":
		svc, err := a.getClient()
		if err != nil {
			return err
		}
		return a.runMe(ctx, svc, args[1:])
	case "add":
		svc, err := a.getClient()
		if err != nil {
			return err
		}
		return a.runAdd(ctx, svc, args[1:])
	case "update":
		svc, err := a.getClient()
		if err != nil {
			return err
		}
		return a.runUpdate(ctx, svc, args[1:])
	case "rm", "delete", "remove":
		svc, err := a.getClient()
		if err != nil {
			return err
		}
		return a.runRemove(ctx, svc, args[1:])
	default:
		a.printUsage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// ---------- client resolution ----------

func (a *App) getClient() (client.Service, error) {
	if a.lazyClient != nil {
		return a.lazyClient, nil
	}

	apiURL, err := config.ActiveURL()
	if err != nil {
		return nil, err
	}

	c, err := client.NewHTTPClient(apiURL, a.httpClient)
	if err != nil {
		return nil, err
	}
	a.lazyClient = c
	return c, nil
}

func (a *App) allClients() ([]namedClient, error) {
	if a.allClientsOverride != nil {
		return a.allClientsOverride, nil
	}
	if a.lazyClient != nil {
		return []namedClient{{name: "", svc: a.lazyClient}}, nil
	}

	store, err := config.LoadStore()
	if err != nil {
		return nil, err
	}
	if len(store.Servers) == 0 {
		return nil, errors.New("timmy is not initialized -- run: timmy init <server-url>")
	}

	out := make([]namedClient, 0, len(store.Servers))
	for _, entry := range store.Servers {
		c, err := client.NewHTTPClient(entry.URL, a.httpClient)
		if err != nil {
			return nil, fmt.Errorf("server %s: %w", entry.Name, err)
		}
		out = append(out, namedClient{name: entry.Name, svc: c})
	}
	return out, nil
}

// ---------- fan-out helpers ----------

type fanOutResult struct {
	servers []client.Server
	source  string
	err     error
}

func (a *App) fanOutList(ctx context.Context, tags []string) ([]client.Server, error) {
	clients, err := a.allClients()
	if err != nil {
		return nil, err
	}

	ch := make(chan fanOutResult, len(clients))
	for _, nc := range clients {
		go func(nc namedClient) {
			servers, err := nc.svc.ListServers(ctx, tags)
			ch <- fanOutResult{servers: servers, source: nc.name, err: err}
		}(nc)
	}
	return a.collectFanOut(ch, len(clients))
}

func (a *App) fanOutSearch(ctx context.Context, query string, tags []string, limit int) ([]client.Server, error) {
	clients, err := a.allClients()
	if err != nil {
		return nil, err
	}

	ch := make(chan fanOutResult, len(clients))
	for _, nc := range clients {
		go func(nc namedClient) {
			servers, err := nc.svc.SearchServers(ctx, query, tags, limit)
			ch <- fanOutResult{servers: servers, source: nc.name, err: err}
		}(nc)
	}
	return a.collectFanOut(ch, len(clients))
}

func (a *App) collectFanOut(ch <-chan fanOutResult, count int) ([]client.Server, error) {
	var all []client.Server
	var errs []string
	for n := 0; n < count; n++ {
		r := <-ch
		if r.err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.source, r.err))
			continue
		}
		for j := range r.servers {
			r.servers[j].Source = r.source
		}
		all = append(all, r.servers...)
	}

	for _, e := range errs {
		_, _ = fmt.Fprintf(a.stderr, "warning: %s\n", e)
	}

	if len(all) == 0 && len(errs) > 0 {
		return nil, fmt.Errorf("all servers unreachable")
	}

	return all, nil
}

// ---------- init / servers / use / uninit ----------

func (a *App) runInit(args []string) error {
	fs := a.newFlagSet("init")
	var name string
	fs.StringVar(&name, "name", "", "friendly name for this server (defaults to hostname)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: timmy init <server-url> [--name NAME]")
	}

	rawURL := fs.Arg(0)

	// Validate server is reachable before adding to config
	testClient, err := client.NewHTTPClient(rawURL, a.httpClient)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := testClient.Me(ctx); err != nil {
		return fmt.Errorf("server %s is not reachable: %w", rawURL, err)
	}

	if err := config.AddServer(name, rawURL); err != nil {
		return err
	}

	store, err := config.LoadStore()
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(a.stdout, "Server added. %d server(s) configured, active: %s\n", len(store.Servers), store.Active)
	return nil
}

func (a *App) runServers() error {
	store, err := config.LoadStore()
	if err != nil {
		return err
	}

	if len(store.Servers) == 0 {
		_, _ = fmt.Fprintln(a.stdout, "No servers configured. Run: timmy init <server-url>")
		return nil
	}

	for _, s := range store.Servers {
		marker := "  "
		if s.Name == store.Active {
			marker = "* "
		}
		_, _ = fmt.Fprintf(a.stdout, "%s%s  %s\n", marker, s.Name, s.URL)
	}
	return nil
}

func (a *App) runUse(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: timmy use <server-name>")
	}
	name := strings.TrimSpace(args[0])
	if err := config.SetActive(name); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(a.stdout, "Switched to %s\n", name)
	return nil
}

func (a *App) runUninit(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: timmy uninit <server-name>")
	}
	name := strings.TrimSpace(args[0])
	if err := config.RemoveServer(name); err != nil {
		return err
	}
	_, _ = fmt.Fprintf(a.stdout, "Removed %s\n", name)
	return nil
}

// ---------- fan-out commands ----------

func (a *App) runList(ctx context.Context, args []string) error {
	fs := a.newFlagSet("ls")
	var (
		tags    stringSliceFlag
		jsonOut bool
	)
	fs.Var(&tags, "tag", "filter by tag (repeatable)")
	fs.BoolVar(&jsonOut, "json", false, "print machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("ls does not accept positional arguments")
	}

	servers, err := a.fanOutList(ctx, tags.Values())
	if err != nil {
		return err
	}

	if jsonOut {
		// Ensure we return empty array instead of null for JSON
		if servers == nil {
			servers = []client.Server{}
		}
		return a.writeJSON(map[string]any{"servers": servers})
	}
	return a.renderServers(servers)
}

func (a *App) runSearch(ctx context.Context, args []string) error {
	fs := a.newFlagSet("search")
	var (
		tags    stringSliceFlag
		jsonOut bool
		limit   int
	)
	fs.Var(&tags, "tag", "filter by tag (repeatable)")
	fs.BoolVar(&jsonOut, "json", false, "print machine-readable output")
	fs.IntVar(&limit, "limit", 50, "maximum servers to return")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("search requires a single query argument")
	}

	servers, err := a.fanOutSearch(ctx, fs.Arg(0), tags.Values(), limit)
	if err != nil {
		return err
	}

	if jsonOut {
		// Ensure we return empty array instead of null for JSON
		if servers == nil {
			servers = []client.Server{}
		}
		return a.writeJSON(map[string]any{
			"query":   fs.Arg(0),
			"servers": servers,
		})
	}
	return a.renderServers(servers)
}

func (a *App) runSSH(ctx context.Context, args []string) error {
	fs := a.newFlagSet("ssh")
	var (
		exact bool
		first bool
	)
	fs.BoolVar(&exact, "exact", false, "require an exact server match")
	fs.BoolVar(&first, "first", false, "connect to the first fuzzy match")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("ssh requires a single query argument")
	}

	server, err := a.resolveServerQueryAll(ctx, fs.Arg(0), exact, first)
	if err != nil {
		return err
	}

	return a.sshRunner.Run(ctx, fmt.Sprintf("%s@%s", server.SSHUser, server.TailscaleIP))
}

// ---------- active-server commands ----------

func (a *App) runMe(ctx context.Context, svc client.Service, args []string) error {
	fs := a.newFlagSet("me")
	var jsonOut bool
	fs.BoolVar(&jsonOut, "json", false, "print machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	me, err := svc.Me(ctx)
	if err != nil {
		return err
	}

	if jsonOut {
		return a.writeJSON(me)
	}
	return a.renderMe(me)
}

func (a *App) runAdd(ctx context.Context, svc client.Service, args []string) error {
	fs := a.newFlagSet("add")
	var (
		name    string
		ip      string
		sshUser string
		tags    stringSliceFlag
		jsonOut bool
	)
	fs.StringVar(&name, "name", "", "server name")
	fs.StringVar(&ip, "ip", "", "tailscale IP")
	fs.StringVar(&sshUser, "user", "root", "ssh username")
	fs.Var(&tags, "tag", "tag to add (repeatable)")
	fs.BoolVar(&jsonOut, "json", false, "print machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if strings.TrimSpace(name) == "" || strings.TrimSpace(ip) == "" {
		return errors.New("add requires --name and --ip")
	}

	server, err := svc.AddServer(ctx, client.CreateServerRequest{
		Name:        name,
		TailscaleIP: ip,
		SSHUser:     sshUser,
		Tags:        tags.Values(),
	})
	if err != nil {
		return err
	}

	if jsonOut {
		return a.writeJSON(server)
	}
	return a.renderServers([]client.Server{server})
}

func (a *App) runUpdate(ctx context.Context, svc client.Service, args []string) error {
	// Reorder args: extract positional arg and put flags first
	// This allows both "update 1 --tag foo" and "update --tag foo 1" to work
	args = reorderArgsForFlags(args)

	fs := a.newFlagSet("update")
	var (
		name      string
		ip        string
		sshUser   string
		tags      stringSliceFlag
		clearTags bool
		jsonOut   bool
	)
	fs.StringVar(&name, "name", "", "new server name")
	fs.StringVar(&ip, "ip", "", "new tailscale IP")
	fs.StringVar(&sshUser, "user", "", "new ssh username")
	fs.Var(&tags, "tag", "replace tags with this set (repeatable)")
	fs.BoolVar(&clearTags, "clear-tags", false, "remove all tags")
	fs.BoolVar(&jsonOut, "json", false, "print machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("update requires <id|query>")
	}

	request := client.UpdateServerRequest{}
	if strings.TrimSpace(name) != "" {
		request.Name = ptr(name)
	}
	if strings.TrimSpace(ip) != "" {
		request.TailscaleIP = ptr(ip)
	}
	if strings.TrimSpace(sshUser) != "" {
		request.SSHUser = ptr(sshUser)
	}
	if clearTags {
		empty := []string{}
		request.Tags = &empty
	} else if len(tags) > 0 {
		values := tags.Values()
		request.Tags = &values
	}

	if request.Name == nil && request.TailscaleIP == nil && request.SSHUser == nil && request.Tags == nil {
		return errors.New("update requires at least one field to change")
	}

	server, err := a.resolveServerQuery(ctx, svc, fs.Arg(0), true, false)
	if err != nil {
		return err
	}

	updated, err := svc.UpdateServer(ctx, server.ID, request)
	if err != nil {
		return err
	}

	if jsonOut {
		return a.writeJSON(updated)
	}
	return a.renderServers([]client.Server{updated})
}

func (a *App) runRemove(ctx context.Context, svc client.Service, args []string) error {
	// Reorder args to handle "rm 1 --json" and "rm --json 1"
	args = reorderArgsForFlags(args)

	fs := a.newFlagSet("rm")
	var jsonOut bool
	fs.BoolVar(&jsonOut, "json", false, "print machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("rm requires <id|query>")
	}

	server, err := a.resolveServerQuery(ctx, svc, fs.Arg(0), true, false)
	if err != nil {
		return err
	}

	if err := svc.DeleteServer(ctx, server.ID); err != nil {
		return err
	}

	if jsonOut {
		return a.writeJSON(map[string]any{"deleted": true, "id": server.ID, "name": server.Name})
	}

	_, err = fmt.Fprintf(a.stdout, "Deleted %s (%d)\n", server.Name, server.ID)
	return err
}

// ---------- server resolution ----------

// resolveServerQuery resolves a query against a single backend.
func (a *App) resolveServerQuery(ctx context.Context, svc client.Service, query string, exactOnly, allowFirst bool) (client.Server, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return client.Server{}, errors.New("query cannot be empty")
	}

	if id, err := strconv.ParseInt(query, 10, 64); err == nil && id > 0 {
		servers, err := svc.ListServers(ctx, nil)
		if err != nil {
			return client.Server{}, err
		}
		for _, server := range servers {
			if server.ID == id {
				return server, nil
			}
		}
		return client.Server{}, fmt.Errorf("server %d not found", id)
	}

	servers, err := svc.SearchServers(ctx, query, nil, 25)
	if err != nil {
		return client.Server{}, err
	}
	return resolveFromCandidates(query, servers, exactOnly, allowFirst)
}

// resolveServerQueryAll fans out search across all backends then resolves.
func (a *App) resolveServerQueryAll(ctx context.Context, query string, exactOnly, allowFirst bool) (client.Server, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return client.Server{}, errors.New("query cannot be empty")
	}

	servers, err := a.fanOutSearch(ctx, query, nil, 25)
	if err != nil {
		return client.Server{}, err
	}
	return resolveFromCandidates(query, servers, exactOnly, allowFirst)
}

func resolveFromCandidates(query string, servers []client.Server, exactOnly, allowFirst bool) (client.Server, error) {
	if len(servers) == 0 {
		return client.Server{}, fmt.Errorf("no servers matched %q", query)
	}

	primaryMatches, secondaryMatches := exactServerMatches(query, servers)
	switch {
	case len(primaryMatches) == 1:
		return primaryMatches[0], nil
	case len(primaryMatches) > 1:
		return client.Server{}, fmt.Errorf("query %q matched %d servers exactly", query, len(primaryMatches))
	case len(secondaryMatches) == 1:
		return secondaryMatches[0], nil
	case len(secondaryMatches) > 1:
		return client.Server{}, fmt.Errorf("query %q matched %d servers exactly", query, len(secondaryMatches))
	case exactOnly:
		return client.Server{}, fmt.Errorf("query %q did not match any server exactly", query)
	case len(servers) == 1:
		return servers[0], nil
	case allowFirst:
		return servers[0], nil
	default:
		return client.Server{}, fmt.Errorf("query %q matched %d servers; refine the query or pass --first", query, len(servers))
	}
}

func exactServerMatches(query string, servers []client.Server) ([]client.Server, []client.Server) {
	primary := make([]client.Server, 0, len(servers))
	secondary := make([]client.Server, 0, len(servers))
	for _, server := range servers {
		switch exactServerMatchKind(query, server) {
		case exactMatchPrimary:
			primary = append(primary, server)
		case exactMatchSecondary:
			secondary = append(secondary, server)
		}
	}
	return primary, secondary
}

type exactMatchKind int

const (
	exactMatchNone exactMatchKind = iota
	exactMatchPrimary
	exactMatchSecondary
)

func exactServerMatchKind(query string, server client.Server) exactMatchKind {
	if strings.EqualFold(server.Name, query) {
		return exactMatchPrimary
	}
	if strings.EqualFold(server.TailscaleIP, query) {
		return exactMatchPrimary
	}
	if strings.EqualFold(server.SSHUser, query) {
		return exactMatchSecondary
	}
	for _, tag := range server.Tags {
		if strings.EqualFold(tag, query) {
			return exactMatchSecondary
		}
	}
	return exactMatchNone
}

// ---------- utilities ----------

// reorderArgsForFlags reorders arguments so flags come before positional args.
// This allows commands like "update 1 --tag foo" to work the same as "update --tag foo 1".
// Go's flag package stops parsing at the first non-flag argument, so we need to
// move positional arguments to the end.
func reorderArgsForFlags(args []string) []string {
	var flags []string
	var positional []string

	i := 0
	for i < len(args) {
		arg := args[i]
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
			// Check if this flag takes a value (not a boolean flag)
			// Flags like --tag, --name, --ip, --user take values
			if !strings.Contains(arg, "=") && i+1 < len(args) {
				// Peek at next arg - if it doesn't start with -, it's likely the value
				next := args[i+1]
				if !strings.HasPrefix(next, "-") {
					// Check if this is a known boolean flag
					flagName := strings.TrimLeft(arg, "-")
					if flagName != "json" && flagName != "clear-tags" && flagName != "exact" && flagName != "first" {
						flags = append(flags, next)
						i++
					}
				}
			}
		} else {
			positional = append(positional, arg)
		}
		i++
	}

	return append(flags, positional...)
}

func (a *App) newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	return fs
}

func (a *App) printUsage() {
	_, _ = fmt.Fprintln(a.stderr, `Usage:
  timmy init <server-url> [--name NAME]   Register a timmy server
  timmy servers                           List configured servers
  timmy use <server-name>                 Switch active server
  timmy uninit <server-name>              Remove a server

  timmy me [--json]                                           (active server)
  timmy add --name NAME --ip IP [--user root] [--tag T]...    (active server)
  timmy ls [--tag TAG]... [--json]                            (all servers)
  timmy search QUERY [--tag TAG]... [--limit N] [--json]      (all servers)
  timmy ssh QUERY [--exact] [--first]                         (all servers)
  timmy update <id|query> [--name N] [--ip I] [--tag T]...    (active server)
  timmy rm <id|query> [--json]                                (active server)`)
}

func ptr[T any](value T) *T {
	return &value
}

type stringSliceFlag []string

func (f *stringSliceFlag) String() string {
	return strings.Join(*f, ",")
}

func (f *stringSliceFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

func (f stringSliceFlag) Values() []string {
	if len(f) == 0 {
		return nil
	}
	values := make([]string, len(f))
	copy(values, f)
	return values
}
