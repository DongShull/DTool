package utiles

import (
	"reflect"
	"testing"

	dockerTypes "github.com/docker/docker/api/types"
)

func TestSplitImageRef(t *testing.T) {
	cases := []struct{ in, name, tag string }{
		{"nginx", "nginx", "<none>"},
		{"nginx:latest", "nginx", "latest"},
		{"registry.example.com:5000/app:v1.2.3", "registry.example.com:5000/app", "v1.2.3"},
		{"linuxserver/qbittorrent:4.4.5", "linuxserver/qbittorrent", "4.4.5"},
		{"sha256:abc", "sha256", "abc"},
		{"ghcr.io/a/b@sha256:deadbeef", "ghcr.io/a/b", "sha256:deadbeef"},
		{"", "", "<none>"},
	}
	for _, c := range cases {
		name, tag := SplitImageRef(c.in)
		if name != c.name || tag != c.tag {
			t.Errorf("SplitImageRef(%q) = (%q,%q), want (%q,%q)", c.in, name, tag, c.name, c.tag)
		}
	}
}

func TestFormatPorts(t *testing.T) {
	ports := []dockerTypes.Port{
		{IP: "0.0.0.0", PrivatePort: 3000, PublicPort: 13000, Type: "tcp"},
		{IP: "::", PrivatePort: 80, PublicPort: 48209, Type: "tcp"},
		{PrivatePort: 1688, Type: "tcp"},
	}
	got := FormatPorts(ports)
	exp := []string{"0.0.0.0:13000→3000/tcp", "[::]:48209→80/tcp", "1688/tcp"}
	if !reflect.DeepEqual(got, exp) {
		t.Fatalf("FormatPorts = %#v, want %#v", got, exp)
	}
	if len(FormatPorts(nil)) != 0 {
		t.Fatalf("空端口列表应返回空")
	}
}

func TestHealthFromStatus(t *testing.T) {
	cases := map[string]string{
		"Up 5 hours (healthy)":             "healthy",
		"Up 2 minutes (unhealthy)":         "unhealthy",
		"Up 10 seconds (health: starting)": "starting",
		"Up 27 hours":                      "",
		"Exited (0) 2 days ago":            "",
		"Up 3 seconds (health: starting) ": "starting",
	}
	for status, want := range cases {
		if got := HealthFromStatus(status); got != want {
			t.Errorf("HealthFromStatus(%q) = %q, want %q", status, got, want)
		}
	}
}

func TestMaskEnv(t *testing.T) {
	env := []string{
		"PATH=/usr/bin",
		"secretKey=abc123",
		"MYSQL_ROOT_PASSWORD=hunter2",
		"API_TOKEN=xyz",
		"WITHOUT_EQUALS",
		"LOG_LEVEL=debug",
	}
	got := maskEnv(env)
	exp := []string{
		"PATH=/usr/bin",
		"secretKey=******",
		"MYSQL_ROOT_PASSWORD=******",
		"API_TOKEN=******",
		"WITHOUT_EQUALS",
		"LOG_LEVEL=debug",
	}
	if !reflect.DeepEqual(got, exp) {
		t.Fatalf("maskEnv = %#v\nwant %#v", got, exp)
	}
}

func TestCleanLogLine(t *testing.T) {
	cases := map[string]string{
		"\x012026-01-01 hello\r": "2026-01-01 hello",
		"\x02stderr line":        "stderr line",
		"plain":                  "plain",
	}
	for in, want := range cases {
		if got := cleanLogLine(in); got != want {
			t.Errorf("cleanLogLine(%q) = %q, want %q", in, got, want)
		}
	}
}
