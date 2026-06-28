package model

import "fmt"

type Section struct {
	code         string
	label        string
	active       bool
	displayOrder int
}

func NewSection(code, label string, active bool, displayOrder int) (Section, error) {
	if code == "" {
		return Section{}, fmt.Errorf("section code must not be empty")
	}
	if label == "" {
		return Section{}, fmt.Errorf("section label must not be empty")
	}
	return Section{code, label, active, displayOrder}, nil
}

func (s Section) Code() string      { return s.code }
func (s Section) Label() string     { return s.label }
func (s Section) Active() bool       { return s.active }
func (s Section) DisplayOrder() int { return s.displayOrder }

type PartnerSection struct {
	partnerID   int
	sectionCode string
}

func NewPartnerSection(partnerID int, sectionCode string) (PartnerSection, error) {
	if partnerID < 0 {
		return PartnerSection{}, fmt.Errorf("partnerID must be >= 0, got %d", partnerID)
	}
	if sectionCode == "" {
		return PartnerSection{}, fmt.Errorf("sectionCode must not be empty")
	}
	return PartnerSection{partnerID, sectionCode}, nil
}

func (m PartnerSection) PartnerID() int      { return m.partnerID }
func (m PartnerSection) SectionCode() string { return m.sectionCode }
