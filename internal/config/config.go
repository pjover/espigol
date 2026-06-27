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
	LogoPath     string
	Server       struct {
		Port int
	}
	OAuth struct {
		ClientID     string
		ClientSecret string
	}
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

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if !errors.As(err, &notFound) {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	cfg := &Config{
		Home:         home,
		DBPath:       filepath.Join(home, "espigol.db"),
		BusinessName: v.GetString("business.name"),
		OutputDir:    v.GetString("output.dir"),
		BackupDir:    v.GetString("backup.dir"),
		LogoPath:     v.GetString("logo.path"),
	}
	cfg.Server.Port = v.GetInt("server.port")
	cfg.OAuth.ClientID = v.GetString("oauth.client_id")
	cfg.OAuth.ClientSecret = v.GetString("oauth.client_secret")
	return cfg, nil
}
