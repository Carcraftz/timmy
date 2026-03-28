package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"timmy/cli/internal/client"
)

func (a *App) renderMe(me client.MeResponse) error {
	_, err := fmt.Fprintf(
		a.stdout,
		"Login:\t%s\nDisplay:\t%s\nNode:\t%s\nTailnet:\t%s\n",
		fallback(me.LoginName),
		fallback(me.DisplayName),
		fallback(me.NodeName),
		fallback(me.Tailnet),
	)
	return err
}

func (a *App) renderServers(servers []client.Server) error {
	if len(servers) == 0 {
		_, err := fmt.Fprintln(a.stdout, "No servers found.")
		return err
	}

	showSource := false
	for _, s := range servers {
		if s.Source != "" {
			showSource = true
			break
		}
	}

	tw := tabwriter.NewWriter(a.stdout, 0, 0, 2, ' ', 0)

	if showSource {
		if _, err := fmt.Fprintln(tw, "SERVER\tID\tNAME\tTAILSCALE IP\tSSH USER\tTAGS"); err != nil {
			return err
		}
		for _, server := range servers {
			if _, err := fmt.Fprintf(
				tw,
				"%s\t%d\t%s\t%s\t%s\t%s\n",
				server.Source,
				server.ID,
				server.Name,
				server.TailscaleIP,
				server.SSHUser,
				strings.Join(server.Tags, ","),
			); err != nil {
				return err
			}
		}
	} else {
		if _, err := fmt.Fprintln(tw, "ID\tNAME\tTAILSCALE IP\tSSH USER\tTAGS"); err != nil {
			return err
		}
		for _, server := range servers {
			if _, err := fmt.Fprintf(
				tw,
				"%d\t%s\t%s\t%s\t%s\n",
				server.ID,
				server.Name,
				server.TailscaleIP,
				server.SSHUser,
				strings.Join(server.Tags, ","),
			); err != nil {
				return err
			}
		}
	}

	return tw.Flush()
}

func (a *App) writeJSON(payload any) error {
	encoder := json.NewEncoder(a.stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(payload)
}

func fallback(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}
	return value
}
