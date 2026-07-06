package model

import "fmt"

type ScopeKind string

const (
	ScopeCommon  ScopeKind = "COMMON"
	ScopeSection ScopeKind = "SECTION"
	ScopePartner ScopeKind = "PARTNER"
)

func ParseScopeKind(s string) (ScopeKind, error) {
	switch ScopeKind(s) {
	case ScopeCommon, ScopeSection, ScopePartner:
		return ScopeKind(s), nil
	default:
		return "", fmt.Errorf("unknown ScopeKind: %q", s)
	}
}

type WindowState string

const (
	WindowDraft  WindowState = "DRAFT"
	WindowOpen   WindowState = "OPEN"
	WindowClosed WindowState = "CLOSED"
)

func ParseWindowState(s string) (WindowState, error) {
	switch WindowState(s) {
	case WindowDraft, WindowOpen, WindowClosed:
		return WindowState(s), nil
	default:
		return "", fmt.Errorf("unknown WindowState: %q", s)
	}
}

type ExpenseCategory string

const (
	CategoryCurrent    ExpenseCategory = "CURRENT"
	CategoryInvestment ExpenseCategory = "INVESTMENT"
)

func ParseExpenseCategory(s string) (ExpenseCategory, error) {
	switch ExpenseCategory(s) {
	case CategoryCurrent, CategoryInvestment:
		return ExpenseCategory(s), nil
	default:
		return "", fmt.Errorf("unknown ExpenseCategory: %q", s)
	}
}

type PartnerType string

const (
	Productor    PartnerType = "Productor"
	Patrocinador PartnerType = "Patrocinador"
	Collaborador PartnerType = "Col·laborador"
)

func ParsePartnerType(s string) (PartnerType, error) {
	switch PartnerType(s) {
	case Productor, Patrocinador, Collaborador:
		return PartnerType(s), nil
	default:
		return "", fmt.Errorf("unknown PartnerType: %q", s)
	}
}

type AuditKind string

const (
	AuditLogin            AuditKind = "LOGIN"
	AuditForecastCreated  AuditKind = "FORECAST_CREATED"
	AuditForecastEdited   AuditKind = "FORECAST_EDITED"
	AuditForecastDeleted  AuditKind = "FORECAST_DELETED"
	AuditWindowOpened     AuditKind = "WINDOW_OPENED"
	AuditWindowEdited     AuditKind = "WINDOW_EDITED"
	AuditWindowClosed     AuditKind = "WINDOW_CLOSED"
	AuditWindowAutoClosed AuditKind = "WINDOW_AUTO_CLOSED"
	AuditReportGenerated  AuditKind = "REPORT_GENERATED"
	AuditPartnerCreated   AuditKind = "PARTNER_CREATED"
	AuditPartnerEdited    AuditKind = "PARTNER_EDITED"
	AuditNotificationSent AuditKind = "NOTIFICATION_SENT"
	AuditMigration        AuditKind = "MIGRATION"
	AuditSectionSaved     AuditKind = "SECTION_SAVED"
	AuditTaxonomySaved    AuditKind = "TAXONOMY_SAVED"
	AuditTaxonomyDeleted  AuditKind = "TAXONOMY_DELETED"
	AuditBoardAuthChanged AuditKind = "BOARD_AUTH_CHANGED"
)

func ParseAuditKind(s string) (AuditKind, error) {
	switch AuditKind(s) {
	case AuditLogin, AuditForecastCreated, AuditForecastEdited, AuditForecastDeleted,
		AuditWindowOpened, AuditWindowClosed, AuditWindowAutoClosed, AuditReportGenerated,
		AuditPartnerCreated, AuditPartnerEdited, AuditNotificationSent, AuditMigration,
		AuditSectionSaved, AuditTaxonomySaved, AuditTaxonomyDeleted, AuditBoardAuthChanged:
		return AuditKind(s), nil
	default:
		return "", fmt.Errorf("unknown AuditKind: %q", s)
	}
}
