// Package auth provides the socis server's session store, authenticator
// (Google OAuth or dev-login), and the RequireAuth middleware.
package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"time"

	"github.com/pjover/espigol/internal/adapters/persistence/sqlc"
	"github.com/pjover/espigol/internal/domain/ports"
)

const sessionTTL = 30 * 24 * time.Hour

// Session is an authenticated session.
type Session struct {
	Token     string
	PartnerID int
	Email     string
	ExpiresAt time.Time
}

// SessionStore persists sessions in SQLite.
type SessionStore struct {
	q     *sqlc.Queries
	clock ports.Clock
}

func NewSessionStore(q *sqlc.Queries, clock ports.Clock) *SessionStore {
	return &SessionStore{q: q, clock: clock}
}

// Create issues a new session token for the partner with a 30-day TTL.
func (s *SessionStore) Create(ctx context.Context, partnerID int, email string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	now := s.clock.Now().UTC()
	err := s.q.InsertSession(ctx, sqlc.InsertSessionParams{
		Token:     token,
		PartnerID: int64(partnerID),
		Email:     email,
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(sessionTTL).Format(time.RFC3339),
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// Get returns the session for a token if it exists and has not expired.
func (s *SessionStore) Get(ctx context.Context, token string) (Session, bool, error) {
	row, err := s.q.GetSession(ctx, token)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, err
	}
	exp, err := time.Parse(time.RFC3339, row.ExpiresAt)
	if err != nil {
		return Session{}, false, err
	}
	if !exp.After(s.clock.Now().UTC()) {
		return Session{}, false, nil // expired
	}
	return Session{Token: row.Token, PartnerID: int(row.PartnerID), Email: row.Email, ExpiresAt: exp}, true, nil
}

// Delete removes a session (logout).
func (s *SessionStore) Delete(ctx context.Context, token string) error {
	return s.q.DeleteSession(ctx, token)
}
