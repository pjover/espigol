// Package mapper translates between sqlc row structs and domain types.
package mapper

import (
	"database/sql"
	"time"
)

const dateLayout = "2006-01-02"

func FormatTimestamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func ParseTimestamp(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

func FormatDate(t time.Time) string { return t.UTC().Format(dateLayout) }

func ParseDate(s string) (time.Time, error) {
	return time.Parse(dateLayout, s)
}

func FormatNullableTimestamp(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: FormatTimestamp(*t), Valid: true}
}

func ParseNullableTimestamp(ns sql.NullString) (*time.Time, error) {
	if !ns.Valid {
		return nil, nil
	}
	t, err := ParseTimestamp(ns.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}
