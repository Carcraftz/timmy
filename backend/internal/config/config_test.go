package config

import (
	"strings"
	"testing"
)

func TestLoad_Valid(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/timmy")
	t.Setenv("TIMMY_TAILNET_NAME", "mynet")
	t.Setenv("TIMMY_HOSTNAME", "myhost")
	t.Setenv("TIMMY_STATE_DIR", "/data/ts")
	t.Setenv("TIMMY_LISTEN_ADDR", ":8080")
	t.Setenv("TIMMY_USE_TLS", "false")
	t.Setenv("TS_AUTHKEY", "tskey-auth-xxx")
	t.Setenv("TS_CLIENT_SECRET", "")
	t.Setenv("TS_ADVERTISE_TAGS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DatabaseURL != "postgres://localhost/timmy" {
		t.Fatalf("database url: %q", cfg.DatabaseURL)
	}
	if cfg.TailnetName != "mynet" {
		t.Fatalf("tailnet: %q", cfg.TailnetName)
	}
	if cfg.Hostname != "myhost" {
		t.Fatalf("hostname: %q", cfg.Hostname)
	}
	if cfg.StateDir != "/data/ts" {
		t.Fatalf("state dir: %q", cfg.StateDir)
	}
	if cfg.ListenAddr != ":8080" {
		t.Fatalf("listen addr: %q", cfg.ListenAddr)
	}
	if cfg.UseTLS != false {
		t.Fatal("expected UseTLS=false")
	}
	if cfg.AuthKey != "tskey-auth-xxx" {
		t.Fatalf("auth key: %q", cfg.AuthKey)
	}
}

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/timmy")
	t.Setenv("TIMMY_TAILNET_NAME", "")
	t.Setenv("TIMMY_HOSTNAME", "")
	t.Setenv("TIMMY_STATE_DIR", "")
	t.Setenv("TIMMY_LISTEN_ADDR", "")
	t.Setenv("TIMMY_USE_TLS", "")
	t.Setenv("TS_AUTHKEY", "")
	t.Setenv("TS_CLIENT_SECRET", "")
	t.Setenv("TS_ADVERTISE_TAGS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TailnetName != "default" {
		t.Fatalf("expected default tailnet, got %q", cfg.TailnetName)
	}
	if cfg.Hostname != "timmyd" {
		t.Fatalf("expected default hostname, got %q", cfg.Hostname)
	}
	if cfg.StateDir != ".data/tsnet" {
		t.Fatalf("expected default state dir, got %q", cfg.StateDir)
	}
	if cfg.ListenAddr != ":443" {
		t.Fatalf("expected default listen addr, got %q", cfg.ListenAddr)
	}
	if cfg.UseTLS != true {
		t.Fatal("expected default UseTLS=true")
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("TS_CLIENT_SECRET", "")
	t.Setenv("TS_ADVERTISE_TAGS", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("expected DATABASE_URL error, got: %v", err)
	}
}

func TestLoad_ClientSecretWithoutTags(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/timmy")
	t.Setenv("TS_CLIENT_SECRET", "some-secret")
	t.Setenv("TS_ADVERTISE_TAGS", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "TS_ADVERTISE_TAGS") {
		t.Fatalf("expected tags error, got: %v", err)
	}
}

func TestLoad_ClientSecretWithTags(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/timmy")
	t.Setenv("TS_CLIENT_SECRET", "some-secret")
	t.Setenv("TS_ADVERTISE_TAGS", "tag:server,tag:prod")
	t.Setenv("TIMMY_USE_TLS", "")
	t.Setenv("TS_AUTHKEY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AdvertiseTags) != 2 || cfg.AdvertiseTags[0] != "tag:server" || cfg.AdvertiseTags[1] != "tag:prod" {
		t.Fatalf("unexpected tags: %v", cfg.AdvertiseTags)
	}
}

// ---------- envBool ----------

func TestEnvBool_TrueValues(t *testing.T) {
	for _, v := range []string{"1", "true", "yes", "on", "TRUE", "Yes", " ON "} {
		t.Setenv("TEST_BOOL", v)
		if !envBool("TEST_BOOL", false) {
			t.Errorf("expected true for %q", v)
		}
	}
}

func TestEnvBool_FalseValues(t *testing.T) {
	for _, v := range []string{"0", "false", "no", "off", "FALSE", "No"} {
		t.Setenv("TEST_BOOL", v)
		if envBool("TEST_BOOL", true) {
			t.Errorf("expected false for %q", v)
		}
	}
}

func TestEnvBool_Fallback(t *testing.T) {
	t.Setenv("TEST_BOOL", "")
	if !envBool("TEST_BOOL", true) {
		t.Fatal("expected true fallback")
	}
	if envBool("TEST_BOOL", false) {
		t.Fatal("expected false fallback")
	}
}

func TestEnvBool_UnrecognizedFallback(t *testing.T) {
	t.Setenv("TEST_BOOL", "maybe")
	if !envBool("TEST_BOOL", true) {
		t.Fatal("expected fallback for unrecognized value")
	}
}

// ---------- splitCSV ----------

func TestSplitCSV_Valid(t *testing.T) {
	result := splitCSV(" a , b , c ")
	if len(result) != 3 || result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Fatalf("unexpected: %v", result)
	}
}

func TestSplitCSV_Empty(t *testing.T) {
	if result := splitCSV(""); result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
	if result := splitCSV("   "); result != nil {
		t.Fatalf("expected nil for whitespace, got %v", result)
	}
}

func TestSplitCSV_SkipsEmptyEntries(t *testing.T) {
	result := splitCSV("a,,b,  ,c")
	if len(result) != 3 {
		t.Fatalf("expected 3 entries, got %v", result)
	}
}
