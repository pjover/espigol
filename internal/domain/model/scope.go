package model

import "fmt"

// ExpenseScope is the scope of a forecast: COMMON, a specific SECTION, or PARTNER.
// sectionCode is set iff kind == ScopeSection.
type ExpenseScope struct {
	kind        ScopeKind
	sectionCode string
}

func NewCommonScope() ExpenseScope  { return ExpenseScope{kind: ScopeCommon} }
func NewPartnerScope() ExpenseScope { return ExpenseScope{kind: ScopePartner} }

func NewSectionScope(code string) (ExpenseScope, error) {
	if code == "" {
		return ExpenseScope{}, fmt.Errorf("section scope requires a non-empty section code")
	}
	return ExpenseScope{kind: ScopeSection, sectionCode: code}, nil
}

// NewScope builds a scope from a kind and an optional section code, enforcing
// that the section code is present iff the kind is SECTION.
func NewScope(kind ScopeKind, sectionCode string) (ExpenseScope, error) {
	switch kind {
	case ScopeSection:
		return NewSectionScope(sectionCode)
	case ScopeCommon, ScopePartner:
		if sectionCode != "" {
			return ExpenseScope{}, fmt.Errorf("scope %s must not carry a section code", kind)
		}
		return ExpenseScope{kind: kind}, nil
	default:
		return ExpenseScope{}, fmt.Errorf("unknown ScopeKind: %q", kind)
	}
}

func (s ExpenseScope) Kind() ScopeKind     { return s.kind }
func (s ExpenseScope) SectionCode() string { return s.sectionCode }
