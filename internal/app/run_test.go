package app

import "testing"

func TestParseMode(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want RunMode
	}{
		{"no args -> tui", []string{}, ModeTUI},
		{"--server -> server", []string{"--server"}, ModeServer},
		{"-server -> server", []string{"-server"}, ModeServer},
		{"--version -> version", []string{"--version"}, ModeVersion},
		{"--version takes priority over --server", []string{"--version", "--server"}, ModeVersion},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ParseMode(c.args); got != c.want {
				t.Errorf("ParseMode(%v) = %v, want %v", c.args, got, c.want)
			}
		})
	}
}
