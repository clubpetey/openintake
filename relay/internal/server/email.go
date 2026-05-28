package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"strconv"

	"intake/internal/auth/emailcode"
)

// emailStartRequest is the body of POST /v1/intake/auth/email/start.
type emailStartRequest struct {
	Email string `json:"email"`
}

// emailVerifyRequest is the body of POST /v1/intake/auth/email/verify.
type emailVerifyRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

// emailVerifyResponse is the success body of POST /v1/intake/auth/email/verify.
type emailVerifyResponse struct {
	Token     string          `json:"token"`
	ExpiresAt string          `json:"expires_at"`
	User      emailVerifyUser `json:"user"`
}

type emailVerifyUser struct {
	Email    string `json:"email"`
	Verified bool   `json:"verified"`
}

// emailStartHandler handles POST /v1/intake/auth/email/start.
// Mounted UNAUTH at the top of registerIntakeRoutes.
func emailStartHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.EmailService == nil {
			writeError(w, http.StatusNotFound, "not_found", "email auth not enabled")
			return
		}

		var req emailStartRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
			return
		}
		addr, err := mail.ParseAddress(req.Email)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid email address")
			return
		}
		email := addr.Address // normalized: bare RFC 5321 address, no display-name

		retry, ierr := deps.EmailService.IssueAndSend(r.Context(), email)
		switch {
		case errors.Is(ierr, emailcode.ErrRateLimited):
			// Anti-enumeration: generic body, detail only via Retry-After header.
			seconds := int(retry.Seconds())
			if seconds < 1 {
				seconds = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(seconds))
			writeError(w, http.StatusTooManyRequests, "rate_limited", "too many codes requested for this email; retry later")
			return
		case errors.Is(ierr, ErrSMTP):
			// Log the underlying detail server-side; the body stays generic.
			if deps.Logger != nil {
				deps.Logger.Error("email start: smtp send failed", "email_redacted", redactEmail(email), "err", ierr.Error())
			}
			writeError(w, http.StatusBadGateway, "smtp_error", "could not send email")
			return
		case ierr != nil:
			// Defensive — should not happen.
			writeError(w, http.StatusInternalServerError, "internal", "internal error")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"message_sent": true})
	}
}

// emailVerifyHandler handles POST /v1/intake/auth/email/verify.
func emailVerifyHandler(deps Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if deps.EmailService == nil {
			writeError(w, http.StatusNotFound, "not_found", "email auth not enabled")
			return
		}

		var req emailVerifyRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
			return
		}
		if req.Email == "" || req.Code == "" {
			writeError(w, http.StatusBadRequest, "bad_request", "email and code are required")
			return
		}
		addr, err := mail.ParseAddress(req.Email)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad_request", "invalid email address")
			return
		}
		email := addr.Address // normalized: bare RFC 5321 address, no display-name

		token, expiresAt, verr := deps.EmailService.VerifyAndMint(email, req.Code)
		if verr != nil {
			// Anti-enumeration: generic 401 regardless of "not found"/"expired"/"used".
			writeError(w, http.StatusUnauthorized, "invalid_code", "the provided code is not valid")
			return
		}

		writeJSON(w, http.StatusOK, emailVerifyResponse{
			Token:     token,
			ExpiresAt: expiresAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
			User: emailVerifyUser{
				Email:    email,
				Verified: true,
			},
		})
	}
}

// redactEmail returns the email with the local-part redacted, suitable for
// log lines. "user@example.com" → "u***@example.com".
func redactEmail(email string) string {
	at := -1
	for i := 0; i < len(email); i++ {
		if email[i] == '@' {
			at = i
			break
		}
	}
	if at < 1 {
		return "***"
	}
	return string(email[0]) + "***" + email[at:]
}
