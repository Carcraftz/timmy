package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	APIURL string `json:"api_url"`
}

func Load() (Config, error) {
	cfg := Config{
		APIURL: "https://timmyd",
	}

	path, err := configPath()
	if err != nil {
		return Config{}, err
	}

	if path != "" {
		file, err := os.ReadFile(path)
		if err == nil {
			var fromFile Config
			if err := json.Unmarshal(file, &fromFile); err != nil {
				return Config{}, err
			}
			if strings.TrimSpace(fromFile.APIURL) != "" {
				cfg.APIURL = strings.TrimSpace(fromFile.APIURL)
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return Config{}, err
		}
	}

	if value := strings.TrimSpace(os.Getenv("TIMMY_API_URL")); value != "" {
		cfg.APIURL = value
	}

	return cfg, nil
}

func configPath() (string, error) {
	if value := strings.TrimSpace(os.Getenv("TIMMY_CONFIG")); value != "" {
		return value, nil
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "timmy", "config.json"), nil
}
