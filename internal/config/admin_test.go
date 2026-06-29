package config

import "testing"

func TestLoad_AdminEmail_Default(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil { t.Fatal(err) }
	if cfg.Admin.Email != "admin@espigol" {
		t.Errorf("Admin.Email default = %q", cfg.Admin.Email)
	}
}

func TestLoad_AdminEmail_EnvOverride(t *testing.T) {
	t.Setenv("ESPIGOL_ADMIN_EMAIL", "boss@coop.cat")
	cfg, err := Load(t.TempDir())
	if err != nil { t.Fatal(err) }
	if cfg.Admin.Email != "boss@coop.cat" {
		t.Errorf("Admin.Email = %q", cfg.Admin.Email)
	}
}
