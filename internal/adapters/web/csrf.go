package web

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
)

// csrfKey is a process-random key derived once at startup.
var (
	csrfKey     []byte
	csrfKeyOnce sync.Once
)

func getCSRFKey() []byte {
	csrfKeyOnce.Do(func() {
		csrfKey = make([]byte, 32)
		if _, err := rand.Read(csrfKey); err != nil {
			panic("web: generating CSRF key: " + err.Error())
		}
	})
	return csrfKey
}

// csrfToken derives a per-session CSRF token deterministically from the session token.
// Uses HMAC-SHA256 with the process-random key.
func csrfToken(sessionToken string) string {
	mac := hmac.New(sha256.New, getCSRFKey())
	mac.Write([]byte(sessionToken))
	return hex.EncodeToString(mac.Sum(nil))
}

// verifyCSRF checks that the "csrf" form field matches the expected token for the session.
// Returns true if the token is valid.
func verifyCSRF(r *http.Request, sessionToken string) bool {
	if err := r.ParseForm(); err != nil {
		return false
	}
	got := r.FormValue("csrf")
	expected := csrfToken(sessionToken)
	return hmac.Equal([]byte(got), []byte(expected))
}
