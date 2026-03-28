package config

import (
	"errors"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL   string
	TailnetName   string
	Hostname      string
	StateDir      string
	ListenAddr    string
	UseTLS        bool
	AuthKey       string
	ClientSecret  string
	AdvertiseTags []string
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL:   strings.TrimSpace(os.Getenv("DATABASE_URL")),
		TailnetName:   envOrDefault("TIMMY_TAILNET_NAME", "default"),
		Hostname:      envOrDefault("TIMMY_HOSTNAME", "timmyd"),
		StateDir:      envOrDefault("TIMMY_STATE_DIR", ".data/tsnet"),
		ListenAddr:    envOrDefault("TIMMY_LISTEN_ADDR", ":443"),
		UseTLS:        envBool("TIMMY_USE_TLS", true),
		AuthKey:       strings.TrimSpace(os.Getenv("TS_AUTHKEY")),
		ClientSecret:  strings.TrimSpace(os.Getenv("TS_CLIENT_SECRET")),
		AdvertiseTags: splitCSV(os.Getenv("TS_ADVERTISE_TAGS")),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	if cfg.ClientSecret != "" && len(cfg.AdvertiseTags) == 0 {
		return Config{}, errors.New("TS_ADVERTISE_TAGS is required when TS_CLIENT_SECRET is set")
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	raw := strings.Split(value, ",")
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
