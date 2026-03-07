package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type OpenAIConfig struct {
	APIKey  string
	BaseURL string
}

type Config struct {
	DBPath          string
	DefaultProvider string
	DefaultModel    string
	Timeout         string
	OpenAI          OpenAIConfig
}

func Default() Config {
	home, _ := os.UserHomeDir()
	base := filepath.Join(home, ".zai")
	return Config{
		DBPath:          filepath.Join(base, "traces.db"),
		DefaultProvider: "openai",
		DefaultModel:    "gpt-4o-mini",
		Timeout:         "30s",
		OpenAI: OpenAIConfig{
			BaseURL: "https://api.openai.com",
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		path = DefaultConfigPath()
	}
	if b, err := os.ReadFile(path); err == nil {
		parseSimpleYAML(&cfg, string(b))
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}

	overrideFromEnv(&cfg)
	if err := ensureParentDir(cfg.DBPath); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func DefaultConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".zai", "config.yaml")
}

func (c Config) TimeoutDuration() time.Duration {
	d, err := time.ParseDuration(c.Timeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

func parseSimpleYAML(cfg *Config, content string) {
	section := ""
	s := bufio.NewScanner(strings.NewReader(content))
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasSuffix(line, ":") {
			section = strings.TrimSuffix(line, ":")
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
		switch section {
		case "openai":
			switch key {
			case "api_key":
				cfg.OpenAI.APIKey = val
			case "base_url":
				cfg.OpenAI.BaseURL = val
			}
		default:
			switch key {
			case "db_path":
				cfg.DBPath = expandHome(val)
			case "default_provider":
				cfg.DefaultProvider = val
			case "default_model":
				cfg.DefaultModel = val
			case "timeout":
				cfg.Timeout = val
			}
		}
	}
}

func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, strings.TrimPrefix(path, "~/"))
}

func overrideFromEnv(cfg *Config) {
	setIf := func(env string, dst *string) {
		v := strings.TrimSpace(os.Getenv(env))
		if v != "" {
			*dst = v
		}
	}
	setIf("ZAI_DB_PATH", &cfg.DBPath)
	setIf("ZAI_PROVIDER", &cfg.DefaultProvider)
	setIf("ZAI_MODEL", &cfg.DefaultModel)
	setIf("ZAI_TIMEOUT", &cfg.Timeout)
	setIf("OPENAI_API_KEY", &cfg.OpenAI.APIKey)
	setIf("ZAI_OPENAI_API_KEY", &cfg.OpenAI.APIKey)
	setIf("OPENAI_BASE_URL", &cfg.OpenAI.BaseURL)
	setIf("ZAI_OPENAI_BASE_URL", &cfg.OpenAI.BaseURL)
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	return nil
}
