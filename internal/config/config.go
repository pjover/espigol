// Package config resolves the espigol home directory and loads settings.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// ResolveHome returns the espigol home directory: $ESPIGOL_HOME when set,
// otherwise <user-home>/.config/espigol.
func ResolveHome() (string, error) {
	if h := os.Getenv("ESPIGOL_HOME"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving user home: %w", err)
	}
	return filepath.Join(home, ".config", "espigol"), nil
}

// Config holds the resolved espigol settings.
type Config struct {
	Home         string
	DBPath       string
	BusinessName string
	OutputDir    string
	BackupDir    string
	ImportDir    string
	LogoPath     string
	Server       struct {
		Port int
	}
	OAuth struct {
		ClientID     string
		ClientSecret string
		RedirectURL  string
	}
	Admin struct {
		Email string
	}
}

// EnsureHome creates the espigol home directory tree (home/, reports/,
// backups/, import/) and writes a default config.yaml if one is not already present.
// It is idempotent and safe to call on an already-initialised home.
func EnsureHome(home string) error {
	for _, dir := range []string{
		home,
		filepath.Join(home, "reports"),
		filepath.Join(home, "backups"),
		filepath.Join(home, "import"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	cfgPath := filepath.Join(home, "config.yaml")
	if _, err := os.Stat(cfgPath); err == nil {
		return nil // already present — don't overwrite
	}
	return os.WriteFile(cfgPath, defaultConfigYAML(home), 0o600)
}

func defaultConfigYAML(_ string) []byte {
	return []byte(`# Espígol configuration — edit as needed.
# All keys can be overridden with ESPIGOL_<KEY> environment variables
# (e.g. ESPIGOL_SERVER_PORT=9090, ESPIGOL_ADMIN_EMAIL=admin@example.org).
#
# Paths below are relative to $ESPIGOL_HOME or absolute if you prefer.

business:
  name: "Cooperativa d'Estellencs"

server:
  port: 8080

output:
  dir: "reports"

backup:
  dir: "backups"

logo:
  path: "logo.png"

oauth:
  client_id: ""
  client_secret: ""
  redirect_url: ""

admin:
  email: "admin@espigol"
`)
}

// Load reads <home>/config.yaml if present, applies defaults, and allows
// environment overrides (prefix ESPIGOL_, nested keys joined with "_").
// A missing config file is not an error.
func Load(home string) (*Config, error) {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(home)

	v.SetEnvPrefix("ESPIGOL")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	v.SetDefault("business.name", "Cooperativa d'Estellencs")
	v.SetDefault("server.port", 8080)
	v.SetDefault("output.dir", filepath.Join(home, "reports"))
	v.SetDefault("backup.dir", filepath.Join(home, "backups"))
	v.SetDefault("logo.path", filepath.Join(home, "logo.png"))
	v.SetDefault("oauth.client_id", "")
	v.SetDefault("oauth.client_secret", "")
	v.SetDefault("oauth.redirect_url", "")
	v.SetDefault("admin.email", "admin@espigol")

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	outputDir := resolvePath(home, v.GetString("output.dir"), "reports")
	backupDir := resolvePath(home, v.GetString("backup.dir"), "backups")
	logoPath := resolvePath(home, v.GetString("logo.path"), "logo.png")

	cfg := &Config{
		Home:         home,
		DBPath:       filepath.Join(home, "espigol.db"),
		BusinessName: v.GetString("business.name"),
		OutputDir:    outputDir,
		BackupDir:    backupDir,
		ImportDir:    filepath.Join(home, "import"),
		LogoPath:     logoPath,
	}
	cfg.Server.Port = v.GetInt("server.port")
	cfg.OAuth.ClientID = v.GetString("oauth.client_id")
	cfg.OAuth.ClientSecret = v.GetString("oauth.client_secret")
	cfg.OAuth.RedirectURL = v.GetString("oauth.redirect_url")
	cfg.Admin.Email = v.GetString("admin.email")
	return cfg, nil
}

// resolvePath resolves a config path value against home.
// Empty/omitted → home/rel (default). Relative → home/val. Absolute → val.
func resolvePath(home, val, rel string) string {
	if val == "" {
		return filepath.Join(home, rel)
	}
	if filepath.IsAbs(val) {
		return val
	}
	return filepath.Join(home, val)
}
