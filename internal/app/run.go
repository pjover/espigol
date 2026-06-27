package app

import "flag"

// RunMode selects which driving adapter the binary launches.
type RunMode int

const (
	// ModeTUI launches the admin terminal UI (default).
	ModeTUI RunMode = iota
	// ModeServer launches the socis HTTP server.
	ModeServer
)

// ParseMode returns ModeServer when the --server flag is present, else ModeTUI.
// Unknown flags are ignored so future flags do not break dispatch.
func ParseMode(args []string) RunMode {
	fs := flag.NewFlagSet("espigol", flag.ContinueOnError)
	fs.SetOutput(nil)
	server := fs.Bool("server", false, "run the HTTP server instead of the TUI")
	_ = fs.Parse(args)
	if *server {
		return ModeServer
	}
	return ModeTUI
}
