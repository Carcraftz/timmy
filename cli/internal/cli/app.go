package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"

	"timmy/cli/internal/client"
	"timmy/cli/internal/ssh"
)

type App struct {
	client client.Service
	ssh    ssh.Runner
	stdout io.Writer
	stderr io.Writer
}

func New(service client.Service, sshRunner ssh.Runner, stdout, stderr io.Writer) *App {
	return &App{
		client: service,
		ssh:    sshRunner,
		stdout: stdout,
		stderr: stderr,
	}
}

func (a *App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		a.printUsage()
		return errors.New("command is required")
	}

	switch args[0] {
	case "me":
		return a.runMe(ctx, args[1:])
	case "add":
		return a.runAdd(ctx, args[1:])
	case "ls", "list":
		return a.runList(ctx, args[1:])
	case "search":
		return a.runSearch(ctx, args[1:])
	case "ssh":
		return a.runSSH(ctx, args[1:])
	case "update":
		return a.runUpdate(ctx, args[1:])
	case "rm", "delete", "remove":
		return a.runRemove(ctx, args[1:])
	case "help", "-h", "--help":
		a.printUsage()
		return nil
	default:
		a.printUsage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func (a *App) runMe(ctx context.Context, args []string) error {
	fs := a.newFlagSet("me")
	var jsonOut bool
	fs.BoolVar(&jsonOut, "json", false, "print machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}

	me, err := a.client.Me(ctx)
	if err != nil {
		return err
	}

	if jsonOut {
		return a.writeJSON(me)
	}
	return a.renderMe(me)
}

func (a *App) runAdd(ctx context.Context, args []string) error {
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

	server, err := a.client.AddServer(ctx, client.CreateServerRequest{
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

	servers, err := a.client.ListServers(ctx, tags.Values())
	if err != nil {
		return err
	}

	if jsonOut {
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

	servers, err := a.client.SearchServers(ctx, fs.Arg(0), tags.Values(), limit)
	if err != nil {
		return err
	}

	if jsonOut {
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

	server, err := a.resolveServerQuery(ctx, fs.Arg(0), exact, first)
	if err != nil {
		return err
	}

	return a.ssh.Run(ctx, fmt.Sprintf("%s@%s", server.SSHUser, server.TailscaleIP))
}

func (a *App) runUpdate(ctx context.Context, args []string) error {
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

	server, err := a.resolveServerQuery(ctx, fs.Arg(0), true, false)
	if err != nil {
		return err
	}

	updated, err := a.client.UpdateServer(ctx, server.ID, request)
	if err != nil {
		return err
	}

	if jsonOut {
		return a.writeJSON(updated)
	}
	return a.renderServers([]client.Server{updated})
}

func (a *App) runRemove(ctx context.Context, args []string) error {
	fs := a.newFlagSet("rm")
	var jsonOut bool
	fs.BoolVar(&jsonOut, "json", false, "print machine-readable output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("rm requires <id|query>")
	}

	server, err := a.resolveServerQuery(ctx, fs.Arg(0), true, false)
	if err != nil {
		return err
	}

	if err := a.client.DeleteServer(ctx, server.ID); err != nil {
		return err
	}

	if jsonOut {
		return a.writeJSON(map[string]any{"deleted": true, "id": server.ID, "name": server.Name})
	}

	_, err = fmt.Fprintf(a.stdout, "Deleted %s (%d)\n", server.Name, server.ID)
	return err
}

func (a *App) resolveServerQuery(ctx context.Context, query string, exactOnly, allowFirst bool) (client.Server, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return client.Server{}, errors.New("query cannot be empty")
	}

	if id, err := strconv.ParseInt(query, 10, 64); err == nil && id > 0 {
		servers, err := a.client.ListServers(ctx, nil)
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

	servers, err := a.client.SearchServers(ctx, query, nil, 25)
	if err != nil {
		return client.Server{}, err
	}
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

func (a *App) newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.stderr)
	return fs
}

func (a *App) printUsage() {
	_, _ = fmt.Fprintln(a.stderr, `Usage:
  timmy me [--json]
  timmy add --name NAME --ip TAILSCALE_IP [--user root] [--tag TAG]... [--json]
  timmy ls [--tag TAG]... [--json]
  timmy search QUERY [--tag TAG]... [--limit N] [--json]
  timmy ssh QUERY [--exact] [--first]
  timmy update <id|query> [--name NAME] [--ip TAILSCALE_IP] [--user USER] [--tag TAG]... [--clear-tags] [--json]
  timmy rm <id|query> [--json]`)
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
