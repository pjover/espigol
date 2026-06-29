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
	ErrNoOpenWindow       = errors.New("no submission window is currently open")
	ErrWindowNotOpen      = errors.New("el termini ja ha finalitzat, contacta amb el Consell Rector")
	ErrForbidden          = errors.New("not authorized to act on this forecast scope")
	ErrForecastNotFound   = errors.New("forecast not found")

	ErrPartnerExists       = errors.New("a partner with that id already exists")
	ErrPartnerNotFound     = errors.New("partner not found")
	ErrEmailTaken          = errors.New("email address already in use by another partner")
	ErrInvalidPartnerType  = errors.New("invalid partner type")
	ErrSectionNotFound     = errors.New("section not found")
)
