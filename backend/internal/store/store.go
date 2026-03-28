package store

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"slices"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("store: not found")
	ErrConflict = errors.New("store: conflict")

	sshUserPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

type Server struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	TailscaleIP string    `json:"tailscale_ip"`
	SSHUser     string    `json:"ssh_user"`
	Tags        []string  `json:"tags"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	CreatedBy   string    `json:"created_by"`
	UpdatedBy   string    `json:"updated_by"`
}

type CreateServerInput struct {
	Name        string   `json:"name"`
	TailscaleIP string   `json:"tailscale_ip"`
	SSHUser     string   `json:"ssh_user"`
	Tags        []string `json:"tags"`
}

type UpdateServerInput struct {
	Name        *string   `json:"name,omitempty"`
	TailscaleIP *string   `json:"tailscale_ip,omitempty"`
	SSHUser     *string   `json:"ssh_user,omitempty"`
	Tags        *[]string `json:"tags,omitempty"`
}

type Store interface {
	EnsureTailnet(ctx context.Context, name string) (int64, error)
	ListServers(ctx context.Context, tailnetID int64, tags []string) ([]Server, error)
	SearchServers(ctx context.Context, tailnetID int64, query string, tags []string, limit int) ([]Server, error)
	CreateServer(ctx context.Context, tailnetID int64, actor string, input CreateServerInput) (Server, error)
	UpdateServer(ctx context.Context, tailnetID int64, id int64, actor string, input UpdateServerInput) (Server, error)
	DeleteServer(ctx context.Context, tailnetID int64, id int64) error
}

func NormalizeCreateInput(input CreateServerInput) (CreateServerInput, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return CreateServerInput{}, errors.New("name is required")
	}

	ip, err := normalizeIP(input.TailscaleIP)
	if err != nil {
		return CreateServerInput{}, err
	}

	sshUser, err := normalizeSSHUser(input.SSHUser)
	if err != nil {
		return CreateServerInput{}, err
	}

	tags, err := NormalizeTags(input.Tags)
	if err != nil {
		return CreateServerInput{}, err
	}

	return CreateServerInput{
		Name:        name,
		TailscaleIP: ip,
		SSHUser:     sshUser,
		Tags:        tags,
	}, nil
}

func NormalizeUpdateInput(input UpdateServerInput) (UpdateServerInput, error) {
	var normalized UpdateServerInput

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return UpdateServerInput{}, errors.New("name cannot be empty")
		}
		normalized.Name = &name
	}

	if input.TailscaleIP != nil {
		ip, err := normalizeIP(*input.TailscaleIP)
		if err != nil {
			return UpdateServerInput{}, err
		}
		normalized.TailscaleIP = &ip
	}

	if input.SSHUser != nil {
		sshUser, err := normalizeSSHUser(*input.SSHUser)
		if err != nil {
			return UpdateServerInput{}, err
		}
		normalized.SSHUser = &sshUser
	}

	if input.Tags != nil {
		tags, err := NormalizeTags(*input.Tags)
		if err != nil {
			return UpdateServerInput{}, err
		}
		normalized.Tags = &tags
	}

	return normalized, nil
}

func NormalizeTags(tags []string) ([]string, error) {
	if len(tags) == 0 {
		return []string{}, nil
	}

	seen := make(map[string]struct{}, len(tags))
	normalized := make([]string, 0, len(tags))

	for _, tag := range tags {
		value := strings.ToLower(strings.TrimSpace(tag))
		if value == "" {
			return nil, errors.New("tags cannot contain empty values")
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}

	slices.Sort(normalized)
	return normalized, nil
}

func normalizeIP(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", errors.New("tailscale_ip is required")
	}

	ip := net.ParseIP(trimmed)
	if ip == nil {
		return "", fmt.Errorf("invalid tailscale_ip %q", value)
	}

	return ip.String(), nil
}

func normalizeSSHUser(value string) (string, error) {
	user := strings.TrimSpace(value)
	if user == "" {
		return "", errors.New("ssh_user is required")
	}
	if !sshUserPattern.MatchString(user) {
		return "", fmt.Errorf("invalid ssh_user %q", value)
	}
	return user, nil
}
