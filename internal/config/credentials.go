package config

import (
	"errors"
	"github.com/Apartment38/torii-gateway/internal/state"
	"strconv"
	"strings"
)

// VisitSecrets is the complete inventory of configuration credential fields.
func (c *Config) VisitSecrets(fn func(path, service, purpose string, ref *SecretRef) error) error {
	for i := range c.Providers {
		p := &c.Providers[i]
		base := "providers/" + strconv.Itoa(i) + "/"
		for _, v := range []struct {
			name string
			ref  *SecretRef
		}{{"bearer_token", p.BearerToken}, {"management_token", p.ManagementToken}} {
			if v.ref != nil {
				if err := fn(base+v.name, p.ID, v.name, v.ref); err != nil {
					return err
				}
			}
		}
		if p.Tracking != nil && p.Tracking.Token != nil {
			if err := fn(base+"tracking/token", p.ID, "tracking_token", p.Tracking.Token); err != nil {
				return err
			}
		}
		for name, ref := range p.Headers {
			if err := fn(base+"headers/"+name, p.ID, "header:"+name, &ref); err != nil {
				return err
			}
			p.Headers[name] = ref
		}
	}
	if c.Router.Classifier.APIKey != nil {
		if err := fn("router/classifier/api_key", "classifier", "api_key", c.Router.Classifier.APIKey); err != nil {
			return err
		}
	}
	for i := range c.Intelligence.Sources {
		s := &c.Intelligence.Sources[i]
		if s.APIKey != nil {
			if err := fn("intelligence/sources/"+strconv.Itoa(i)+"/api_key", "benchmark:"+s.ID, "api_key", s.APIKey); err != nil {
				return err
			}
		}
	}
	return nil
}
func (c *Config) BindSecrets(db *state.Database) error {
	c.Credentials = db
	return c.VisitSecrets(func(_, _, _ string, r *SecretRef) error {
		if r.Credential == "" || r.Env != "" || r.File != "" || r.Value != "" {
			return errors.New("persisted credentials must use database references; add credentials through the authenticated console")
		}
		if strings.HasPrefix(r.Credential, "__") || state.IsGatewayAPIKeyID(r.Credential) {
			return errors.New("system credentials cannot be used as provider credentials")
		}
		r.Bind(db)
		return nil
	})
}

// StoreSubmittedCredentials is used only behind authenticated management APIs.
// Existing references are ownership-checked; direct input is never serialized.
func StoreSubmittedCredentials(c *Config, db *state.Database) error {
	c.Credentials = db
	return c.VisitSecrets(func(_, service, purpose string, r *SecretRef) error {
		if strings.HasPrefix(r.Credential, "__") || state.IsGatewayAPIKeyID(r.Credential) {
			return errors.New("system credentials cannot be used as provider credentials")
		}
		if r.Credential == "" {
			if r.Env != "" || r.File != "" {
				return errors.New("use stored credential references or a directly submitted value; environment and file references are not supported in configuration")
			}
			if err := r.Validate(); err != nil {
				return err
			}
			id := state.NewSecretID()
			if err := db.Put(id, state.Owner, service, purpose, []byte(r.Value)); err != nil {
				return err
			}
			*r = SecretRef{Credential: id}
		}
		if _, err := db.Get(r.Credential, state.Owner); err != nil {
			return errors.New("credential reference is unavailable or not owned by this administrator")
		}
		r.Bind(db)
		return nil
	})
}
