package app

import "flag"

// RunMode selects which driving adapter the binary launches.
type RunMode int

const (
	// ModeTUI launches the admin terminal UI (default).
	ModeTUI RunMode = iota
	// ModeServer launches the socis HTTP server.
	ModeServer
	// ModeVersion prints the binary version and exits.
	ModeVersion
)

// ParseMode returns ModeVersion when --version is present (checked first, so it
// takes priority over any other flag), ModeServer when --server is present,
// else ModeTUI. Unknown flags are ignored so future flags do not break dispatch.
func ParseMode(args []string) RunMode {
	fs := flag.NewFlagSet("espigol", flag.ContinueOnError)
	fs.SetOutput(nil)
	server := fs.Bool("server", false, "run the HTTP server instead of the TUI")
	versionFlag := fs.Bool("version", false, "print the version and exit")
	_ = fs.Parse(args)
	if *versionFlag {
		return ModeVersion
	}
	if *server {
		return ModeServer
	}
	return ModeTUI
}
