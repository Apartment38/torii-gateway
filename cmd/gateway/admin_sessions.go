package main

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/Apartment38/torii-gateway/internal/api"
	"github.com/Apartment38/torii-gateway/internal/config"
	"github.com/Apartment38/torii-gateway/internal/state"
)

const adminCookie = "torii_admin_session"

type sessionAuthenticator struct {
	db     *state.Database
	basic  api.ClientAuthenticator
	server config.ServerConfig
}

func secureRequest(r *http.Request, server config.ServerConfig) bool {
	if r.TLS != nil {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	for _, cidr := range server.TrustedProxies {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil && network.Contains(ip) {
			return r.Header.Get("X-Forwarded-Proto") == "https"
		}
	}
	return false
}
func sessionOrigin(r *http.Request, server config.ServerConfig) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	u, err := url.Parse(origin)
	scheme := "http"
	if secureRequest(r, server) {
		scheme = "https"
	}
	return err == nil && u.Scheme == scheme && u.Host == r.Host && u.User == nil && u.RawQuery == "" && u.Fragment == "" && u.Path == ""
}
func (a sessionAuthenticator) Authenticate(r *http.Request) (*api.ClientIdentity, error) {
	if r.Header.Get("Authorization") != "" {
		return a.basic.Authenticate(r)
	}
	cookie, err := r.Cookie(adminCookie)
	if err != nil {
		return nil, api.ErrUnauthorized
	}
	session, err := a.db.AdminSession(cookie.Value)
	if err != nil {
		return nil, api.ErrUnauthorized
	}
	if !sessionOrigin(r, a.server) {
		return nil, api.ErrUnauthorized
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(session.CSRF)) != 1 {
			return nil, api.ErrUnauthorized
		}
	}
	return &api.ClientIdentity{ID: state.Owner}, nil
}
func (r *handlerRuntime) adminSessionAPI(server config.ServerConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !sessionOrigin(req, server) {
			writeConfigError(w, 403, errors.New("cross-origin session request denied"))
			return
		}
		switch req.Method {
		case http.MethodPost:
			var input struct {
				Username string `json:"username"`
				Password string `json:"password"`
			}
			decoder := json.NewDecoder(http.MaxBytesReader(w, req.Body, 4096))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&input) != nil || input.Username == "" || input.Password == "" {
				writeConfigError(w, 400, errors.New("provide administrator username and password"))
				return
			}
			clone := req.Clone(req.Context())
			clone.Header = clone.Header.Clone()
			clone.SetBasicAuth(input.Username, input.Password)
			if _, err := r.adminBasic.Authenticate(clone); err != nil {
				writeConfigError(w, 401, errors.New("administrator authentication failed"))
				return
			}
			ttl := 7 * 24 * time.Hour
			if server.AdminSessionTTL != "" {
				ttl, _ = time.ParseDuration(server.AdminSessionTTL)
			}
			token, session, err := r.credentials.NewAdminSession(ttl)
			if err != nil {
				writeConfigError(w, 503, errors.New("session storage unavailable"))
				return
			}
			http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: token, Path: "/", Expires: session.ExpiresAt, MaxAge: int(ttl.Seconds()), HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secureRequest(req, server)})
			writeJSON(w, 201, session)
		case http.MethodGet, http.MethodDelete:
			cookie, err := req.Cookie(adminCookie)
			if err != nil {
				writeConfigError(w, 401, errors.New("sign in required"))
				return
			}
			session, err := r.credentials.AdminSession(cookie.Value)
			if err != nil {
				writeConfigError(w, 401, errors.New("session expired; sign in again"))
				return
			}
			if req.Method == http.MethodGet {
				writeJSON(w, 200, session)
				return
			}
			if subtle.ConstantTimeCompare([]byte(req.Header.Get("X-CSRF-Token")), []byte(session.CSRF)) != 1 {
				writeConfigError(w, 403, errors.New("CSRF token required"))
				return
			}
			if err = r.credentials.DeleteAdminSession(cookie.Value); err != nil {
				writeConfigError(w, 503, errors.New("cannot revoke session"))
				return
			}
			http.SetCookie(w, &http.Cookie{Name: adminCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteLaxMode, Secure: secureRequest(req, server)})
			w.WriteHeader(204)
		default:
			w.WriteHeader(405)
		}
	}
}
