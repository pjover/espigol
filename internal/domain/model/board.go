package model

import "fmt"

type BoardAuthorization struct {
	partnerID   int
	scopeKind   ScopeKind
	sectionCode string
}

// NewBoardAuthorization builds an authorization for a non-Soci scope a board
// member may edit on the web. Only COMMON and SECTION are valid; sectionCode is
// set iff scopeKind == SECTION.
func NewBoardAuthorization(partnerID int, scopeKind ScopeKind, sectionCode string) (BoardAuthorization, error) {
	if partnerID < 0 {
		return BoardAuthorization{}, fmt.Errorf("partnerID must be >= 0, got %d", partnerID)
	}
	switch scopeKind {
	case ScopeCommon:
		if sectionCode != "" {
			return BoardAuthorization{}, fmt.Errorf("COMMON authorization must not carry a section code")
		}
	case ScopeSection:
		if sectionCode == "" {
			return BoardAuthorization{}, fmt.Errorf("SECTION authorization requires a section code")
		}
	default:
		return BoardAuthorization{}, fmt.Errorf("board authorization scope must be COMMON or SECTION, got %q", scopeKind)
	}
	return BoardAuthorization{partnerID, scopeKind, sectionCode}, nil
}

func (a BoardAuthorization) PartnerID() int       { return a.partnerID }
func (a BoardAuthorization) ScopeKind() ScopeKind { return a.scopeKind }
func (a BoardAuthorization) SectionCode() string  { return a.sectionCode }
