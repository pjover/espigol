package model

import (
	"fmt"
	"time"
)

type Partner struct {
	id          int
	name        string
	surname     string
	vatCode     string
	email       string
	mobile      string
	partnerType PartnerType
	riaNumber   int
	addedOn     time.Time
	boardMember bool
}

func NewPartner(id int, name, surname, vatCode, email, mobile string, pt PartnerType,
	riaNumber int, addedOn time.Time, boardMember bool) (Partner, error) {
	if id < 0 {
		return Partner{}, fmt.Errorf("partner id must be >= 0, got %d", id)
	}
	if riaNumber < 0 {
		return Partner{}, fmt.Errorf("riaNumber must be >= 0, got %d", riaNumber)
	}
	return Partner{id, name, surname, vatCode, email, mobile, pt, riaNumber, addedOn, boardMember}, nil
}

func (p Partner) ID() int                  { return p.id }
func (p Partner) Name() string             { return p.name }
func (p Partner) Surname() string          { return p.surname }
func (p Partner) VatCode() string          { return p.vatCode }
func (p Partner) Email() string            { return p.email }
func (p Partner) Mobile() string           { return p.mobile }
func (p Partner) PartnerType() PartnerType { return p.partnerType }
func (p Partner) RiaNumber() int           { return p.riaNumber }
func (p Partner) AddedOn() time.Time       { return p.addedOn }
func (p Partner) BoardMember() bool        { return p.boardMember }

func (p Partner) WithBoardMember(b bool) Partner {
	p.boardMember = b
	return p
}
