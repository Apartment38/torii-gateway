package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/Apartment38/torii-gateway/internal/config"
)

func (r *handlerRuntime) adminAccount(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	a, err := r.credentials.Admin()
	if err != nil {
		writeConfigError(w, 503, errors.New("administrator unavailable"))
		return
	}
	switch req.Method {
	case http.MethodGet:
		writeJSON(w, 200, map[string]string{"username": a.Username, "email": a.Email})
	case http.MethodPut:
		var input struct {
			Username        string `json:"username"`
			Email           string `json:"email"`
			Password        string `json:"password"`
			CurrentPassword string `json:"current_password"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil {
			writeConfigError(w, 400, errors.New("invalid administrator request"))
			return
		}
		clone := req.Clone(req.Context())
		clone.Header = clone.Header.Clone()
		clone.SetBasicAuth(a.Username, input.CurrentPassword)
		if _, err = r.adminBasic.Authenticate(clone); err != nil {
			writeConfigError(w, 401, errors.New("administrator authentication failed"))
			return
		}
		if err = r.credentials.UpdateAdmin(input.Username, input.Email, input.Password, input.CurrentPassword); err != nil {
			writeConfigError(w, 400, err)
			return
		}
		writeJSON(w, 200, map[string]string{"message": "Administrator updated. Sign in again."})
	default:
		w.Header().Set("Allow", "GET, PUT")
		w.WriteHeader(405)
	}
}

func (r *handlerRuntime) passwordResetAPI(server config.ServerConfig) http.HandlerFunc {
	sender := r.resetSender
	if sender == nil {
		sender, _ = passwordResetSenderFromEnv()
	}
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if req.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			w.WriteHeader(405)
			return
		}
		if !sessionOrigin(req, server) {
			writeConfigError(w, 403, errors.New("cross-origin reset request denied"))
			return
		}
		var input struct {
			Email    string `json:"email"`
			Token    string `json:"token"`
			Password string `json:"password"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 4096))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil {
			writeConfigError(w, 400, errors.New("invalid password reset request"))
			return
		}
		if req.URL.Path == "/gateway/v1/password-reset/confirm" {
			r.resetMu.Lock()
			allowed := time.Now().After(r.resetAttemptAfter)
			if allowed {
				r.resetAttemptAfter = time.Now().Add(6 * time.Second)
			}
			r.resetMu.Unlock()
			if !allowed {
				w.Header().Set("Retry-After", "6")
				writeConfigError(w, 429, errors.New("wait before retrying"))
				return
			}
			if err := r.credentials.ResetAdminPassword(input.Token, input.Password); err != nil {
				writeConfigError(w, 400, err)
				return
			}
			writeJSON(w, 200, map[string]string{"message": "Password changed. Sign in with your new password."})
			return
		}
		// Every address gets the same immediate response, including disabled delivery.
		// One in-flight worker and a global cooldown bound anonymous resource use.
		r.resetMu.Lock()
		allowed := !r.resetBusy && time.Now().After(r.resetRequestAfter)
		if allowed {
			r.resetBusy = true
			r.resetRequestAfter = time.Now().Add(10 * time.Second)
		}
		r.resetMu.Unlock()
		if allowed {
			go func() {
				defer func() { r.resetMu.Lock(); r.resetBusy = false; r.resetMu.Unlock() }()
				if sender == nil {
					return
				}
				token, email, err := r.credentials.IssueAdminPasswordReset(input.Email)
				if err != nil {
					return
				}
				if err = sender(email, token); err != nil {
					log.Print("Administrator password reset email delivery failed; check SMTP configuration and connectivity.")
				}
			}()
		}
		writeJSON(w, 202, map[string]string{"message": "If this email matches the administrator account and email delivery is configured, a password reset link will be sent. Please check your inbox."})
	}
}
