package app

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zhouze/zai-xray/internal/config"
	"github.com/zhouze/zai-xray/internal/render"
)

func RunDoctor(ctx context.Context, cfg config.Config) []render.DoctorCheck {
	checks := make([]render.DoctorCheck, 0, 5)

	if strings.TrimSpace(cfg.OpenAI.APIKey) == "" {
		checks = append(checks, render.DoctorCheck{Name: "openai_api_key", Status: "WARN", Message: "OPENAI_API_KEY is not configured"})
	} else {
		checks = append(checks, render.DoctorCheck{Name: "openai_api_key", Status: "PASS", Message: "OPENAI_API_KEY detected"})
	}

	cfgPath := config.DefaultConfigPath()
	if _, err := os.Stat(cfgPath); err == nil {
		checks = append(checks, render.DoctorCheck{Name: "config_file", Status: "PASS", Message: "config file exists: " + cfgPath})
	} else if os.IsNotExist(err) {
		checks = append(checks, render.DoctorCheck{Name: "config_file", Status: "WARN", Message: "config file not found: " + cfgPath})
	} else {
		checks = append(checks, render.DoctorCheck{Name: "config_file", Status: "FAIL", Message: err.Error()})
	}

	if err := checkDBPath(ctx, cfg.DBPath); err != nil {
		checks = append(checks, render.DoctorCheck{Name: "db_path", Status: "FAIL", Message: err.Error()})
	} else {
		checks = append(checks, render.DoctorCheck{Name: "db_path", Status: "PASS", Message: "db path writable: " + cfg.DBPath})
	}

	networkStatus, networkMsg := checkNetwork(ctx, cfg.OpenAI.BaseURL)
	checks = append(checks, render.DoctorCheck{Name: "network_openai", Status: networkStatus, Message: networkMsg})

	return checks
}

func checkDBPath(ctx context.Context, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create db directory failed: %w", err)
	}
	f, err := os.CreateTemp(dir, "zai-doctor-*.tmp")
	if err != nil {
		return fmt.Errorf("db directory not writable: %w", err)
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)

	cmd := exec.CommandContext(ctx, "sqlite3", path, "SELECT 1;")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sqlite check failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func checkNetwork(ctx context.Context, rawURL string) (string, string) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "FAIL", "invalid OPENAI_BASE_URL"
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		if u.Scheme == "https" {
			host += ":443"
		} else {
			host += ":80"
		}
	}
	d := net.Dialer{Timeout: 2 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return "WARN", "network check failed: " + err.Error()
	}
	_ = conn.Close()
	return "PASS", "reachable: " + host
}
