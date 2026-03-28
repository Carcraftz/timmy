package store

import (
	"strings"
	"testing"
)

// ---------- NormalizeCreateInput ----------

func TestNormalizeCreateInput_Valid(t *testing.T) {
	input, err := NormalizeCreateInput(CreateServerInput{
		Name: "  prod-db ", TailscaleIP: " 100.64.0.10 ", SSHUser: "root", Tags: []string{"Prod", "prod", "DB"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input.Name != "prod-db" {
		t.Fatalf("name not trimmed: %q", input.Name)
	}
	if input.TailscaleIP != "100.64.0.10" {
		t.Fatalf("ip not normalized: %q", input.TailscaleIP)
	}
	if input.SSHUser != "root" {
		t.Fatalf("ssh_user: %q", input.SSHUser)
	}
	if len(input.Tags) != 2 {
		t.Fatalf("tags not deduped: %v", input.Tags)
	}
}

func TestNormalizeCreateInput_MissingName(t *testing.T) {
	_, err := NormalizeCreateInput(CreateServerInput{
		Name: "  ", TailscaleIP: "100.64.0.1", SSHUser: "root",
	})
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("expected name error, got: %v", err)
	}
}

func TestNormalizeCreateInput_MissingIP(t *testing.T) {
	_, err := NormalizeCreateInput(CreateServerInput{
		Name: "x", TailscaleIP: "", SSHUser: "root",
	})
	if err == nil || !strings.Contains(err.Error(), "tailscale_ip") {
		t.Fatalf("expected ip error, got: %v", err)
	}
}

func TestNormalizeCreateInput_InvalidIP(t *testing.T) {
	_, err := NormalizeCreateInput(CreateServerInput{
		Name: "x", TailscaleIP: "not-an-ip", SSHUser: "root",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid ip error, got: %v", err)
	}
}

func TestNormalizeCreateInput_MissingSSHUser(t *testing.T) {
	_, err := NormalizeCreateInput(CreateServerInput{
		Name: "x", TailscaleIP: "100.64.0.1", SSHUser: "",
	})
	if err == nil || !strings.Contains(err.Error(), "ssh_user") {
		t.Fatalf("expected ssh_user error, got: %v", err)
	}
}

func TestNormalizeCreateInput_InvalidSSHUser(t *testing.T) {
	_, err := NormalizeCreateInput(CreateServerInput{
		Name: "x", TailscaleIP: "100.64.0.1", SSHUser: "root user",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("expected invalid ssh_user error, got: %v", err)
	}
}

func TestNormalizeCreateInput_IPv6(t *testing.T) {
	input, err := NormalizeCreateInput(CreateServerInput{
		Name: "v6", TailscaleIP: "fd7a:115c:a1e0::1", SSHUser: "root",
	})
	if err != nil {
		t.Fatalf("unexpected error for IPv6: %v", err)
	}
	if input.TailscaleIP != "fd7a:115c:a1e0::1" {
		t.Fatalf("unexpected normalized IPv6: %q", input.TailscaleIP)
	}
}

func TestNormalizeCreateInput_EmptyTags(t *testing.T) {
	input, err := NormalizeCreateInput(CreateServerInput{
		Name: "x", TailscaleIP: "100.64.0.1", SSHUser: "root", Tags: nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Tags == nil || len(input.Tags) != 0 {
		t.Fatalf("expected empty slice, got %v", input.Tags)
	}
}

// ---------- NormalizeUpdateInput ----------

func TestNormalizeUpdateInput_AllFields(t *testing.T) {
	name, ip, user := "new-name", "100.64.0.99", "deploy"
	tags := []string{"A", "b", "a"}
	input, err := NormalizeUpdateInput(UpdateServerInput{
		Name: &name, TailscaleIP: &ip, SSHUser: &user, Tags: &tags,
	})
	if err != nil {
		t.Fatal(err)
	}
	if *input.Name != "new-name" {
		t.Fatalf("name: %q", *input.Name)
	}
	if *input.TailscaleIP != "100.64.0.99" {
		t.Fatalf("ip: %q", *input.TailscaleIP)
	}
	if *input.SSHUser != "deploy" {
		t.Fatalf("user: %q", *input.SSHUser)
	}
	if len(*input.Tags) != 2 {
		t.Fatalf("tags not deduped: %v", *input.Tags)
	}
}

func TestNormalizeUpdateInput_NoFields(t *testing.T) {
	input, err := NormalizeUpdateInput(UpdateServerInput{})
	if err != nil {
		t.Fatal(err)
	}
	if input.Name != nil || input.TailscaleIP != nil || input.SSHUser != nil || input.Tags != nil {
		t.Fatalf("expected all nil: %+v", input)
	}
}

func TestNormalizeUpdateInput_EmptyName(t *testing.T) {
	name := "  "
	_, err := NormalizeUpdateInput(UpdateServerInput{Name: &name})
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty name error, got: %v", err)
	}
}

func TestNormalizeUpdateInput_InvalidIP(t *testing.T) {
	ip := "garbage"
	_, err := NormalizeUpdateInput(UpdateServerInput{TailscaleIP: &ip})
	if err == nil {
		t.Fatal("expected error for invalid IP")
	}
}

func TestNormalizeUpdateInput_InvalidSSHUser(t *testing.T) {
	user := "bad user!"
	_, err := NormalizeUpdateInput(UpdateServerInput{SSHUser: &user})
	if err == nil {
		t.Fatal("expected error for invalid SSH user")
	}
}

func TestNormalizeUpdateInput_ClearTags(t *testing.T) {
	empty := []string{}
	input, err := NormalizeUpdateInput(UpdateServerInput{Tags: &empty})
	if err != nil {
		t.Fatal(err)
	}
	if input.Tags == nil || len(*input.Tags) != 0 {
		t.Fatalf("expected empty tags, got %v", input.Tags)
	}
}

// ---------- NormalizeTags ----------

func TestNormalizeTags_Nil(t *testing.T) {
	tags, err := NormalizeTags(nil)
	if err != nil {
		t.Fatal(err)
	}
	if tags == nil || len(tags) != 0 {
		t.Fatalf("expected empty slice, got %v", tags)
	}
}

func TestNormalizeTags_Empty(t *testing.T) {
	tags, err := NormalizeTags([]string{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 0 {
		t.Fatalf("expected empty, got %v", tags)
	}
}

func TestNormalizeTags_Lowercase(t *testing.T) {
	tags, err := NormalizeTags([]string{"Prod", "US-East"})
	if err != nil {
		t.Fatal(err)
	}
	if tags[0] != "prod" || tags[1] != "us-east" {
		t.Fatalf("not lowered: %v", tags)
	}
}

func TestNormalizeTags_Dedup(t *testing.T) {
	tags, err := NormalizeTags([]string{"prod", "PROD", "  prod  "})
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0] != "prod" {
		t.Fatalf("not deduped: %v", tags)
	}
}

func TestNormalizeTags_Sorted(t *testing.T) {
	tags, err := NormalizeTags([]string{"zebra", "alpha", "mid"})
	if err != nil {
		t.Fatal(err)
	}
	if tags[0] != "alpha" || tags[1] != "mid" || tags[2] != "zebra" {
		t.Fatalf("not sorted: %v", tags)
	}
}

func TestNormalizeTags_EmptyValueRejected(t *testing.T) {
	_, err := NormalizeTags([]string{"good", "  ", "ok"})
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty tag error, got: %v", err)
	}
}

// ---------- SSH user validation edge cases ----------

func TestSSHUserValidChars(t *testing.T) {
	valid := []string{"root", "deploy_user", "web-server", "user.name", "USER123"}
	for _, u := range valid {
		_, err := NormalizeCreateInput(CreateServerInput{
			Name: "x", TailscaleIP: "100.64.0.1", SSHUser: u,
		})
		if err != nil {
			t.Errorf("expected %q to be valid, got: %v", u, err)
		}
	}
}

func TestSSHUserInvalidChars(t *testing.T) {
	invalid := []string{"root user", "root@host", "user/name", "user;cmd", "user$var"}
	for _, u := range invalid {
		_, err := NormalizeCreateInput(CreateServerInput{
			Name: "x", TailscaleIP: "100.64.0.1", SSHUser: u,
		})
		if err == nil {
			t.Errorf("expected %q to be invalid", u)
		}
	}
}

// ---------- IP normalization ----------

func TestNormalizeIP_Trims(t *testing.T) {
	input, _ := NormalizeCreateInput(CreateServerInput{
		Name: "x", TailscaleIP: "  100.64.0.1  ", SSHUser: "root",
	})
	if input.TailscaleIP != "100.64.0.1" {
		t.Fatalf("not trimmed: %q", input.TailscaleIP)
	}
}
