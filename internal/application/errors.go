package application

import "errors"

var (
	ErrYearExists         = errors.New("submission window already exists for that year")
	ErrNoPriorYear        = errors.New("no prior year to copy taxonomy and limits from")
	ErrWindowNotFound     = errors.New("submission window not found")
	ErrWrongState         = errors.New("operation not allowed in the window's current state")
	ErrDeadlinePassed     = errors.New("deadline must be in the future to open the window")
	ErrIncompleteTaxonomy = errors.New("taxonomy must define at least one CURRENT and one INVESTMENT type")
	ErrAnotherWindowOpen  = errors.New("another submission window is already open")
)
