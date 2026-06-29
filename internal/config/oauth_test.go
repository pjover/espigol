package config

import "testing"

func TestLoad_OAuthRedirectURL(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ESPIGOL_OAUTH_REDIRECT_URL", "https://espigol.example/oauth2/callback")
	cfg, err := Load(home)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OAuth.RedirectURL != "https://espigol.example/oauth2/callback" {
		t.Errorf("RedirectURL = %q", cfg.OAuth.RedirectURL)
	}
}

func TestLoad_OAuthRedirectURL_DefaultsEmpty(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.OAuth.RedirectURL != "" {
		t.Errorf("RedirectURL default = %q, want empty", cfg.OAuth.RedirectURL)
	}
}
