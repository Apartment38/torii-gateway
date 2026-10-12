package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"time"
)

// Opt-in integration: Torii never imports or rewrites the CLI's credential files.
type CodexAppServerConfig struct{ Path, Home string }

func (c *Codex) ConfigureAppServer(cfg CodexAppServerConfig) { c.appServer = cfg }

func (c *Codex) appServerQuota(ctx context.Context, credential CodexCredential) (map[string]any, error) {
	if credential.AccountID == "" || credential.Email == "" {
		return nil, errors.New("CLI quota user/workspace identity cannot be verified")
	}
	cmd := exec.CommandContext(ctx, c.appServer.Path, "app-server")
	cmd.WaitDelay = time.Second
	cmd.Env = os.Environ()
	if c.appServer.Home != "" {
		env := make([]string, 0, len(cmd.Env))
		for _, v := range cmd.Env {
			if len(v) < 11 || v[:11] != "CODEX_HOME=" {
				env = append(env, v)
			}
		}
		cmd.Env = append(env, "CODEX_HOME="+c.appServer.Home)
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, errors.New("CLI unavailable")
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, errors.New("CLI unavailable")
	}
	if cmd.Start() != nil {
		return nil, errors.New("CLI unavailable")
	}
	stopClose := context.AfterFunc(ctx, func() { out.Close(); in.Close() })
	defer stopClose()
	defer func() { in.Close(); out.Close(); cmd.Process.Kill(); cmd.Wait() }()
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	send := func(v any) error { return json.NewEncoder(in).Encode(v) }
	read := func(id int) (map[string]any, error) {
		for n := 0; n < 256 && scanner.Scan(); n++ {
			var envelope struct {
				ID     *int            `json:"id"`
				Result map[string]any  `json:"result"`
				Error  json.RawMessage `json:"error"`
				Method string          `json:"method"`
			}
			dec := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
			dec.UseNumber()
			if dec.Decode(&envelope) != nil {
				return nil, errors.New("invalid CLI JSON-RPC")
			}
			// account/rateLimits/updated is a notification, not the requested response.
			// Read the full account snapshot; never attach an unverified notification.
			if envelope.ID == nil || *envelope.ID != id {
				continue
			}
			if len(envelope.Error) > 0 && string(envelope.Error) != "null" {
				return nil, errors.New("CLI quota request failed")
			}
			if envelope.Result == nil {
				return nil, errors.New("CLI response missing result")
			}
			return envelope.Result, nil
		}
		return nil, errors.New("CLI quota unavailable or timed out")
	}
	if send(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]string{"name": "torii-gateway", "version": "1.0"}}}) != nil {
		return nil, errors.New("CLI initialization failed")
	}
	if _, err = read(1); err != nil {
		return nil, err
	}
	if send(map[string]any{"method": "initialized", "params": map[string]any{}}) != nil {
		return nil, errors.New("CLI initialization failed")
	}
	if send(map[string]any{"id": 2, "method": "account/read", "params": map[string]bool{"refreshToken": false}}) != nil {
		return nil, errors.New("CLI account read failed")
	}
	account, err := read(2)
	if err != nil {
		return nil, err
	}
	identity := object(account["account"])
	if textValue(identity["type"]) != "chatgpt" || credential.Email != "" && textValue(identity["email"]) != credential.Email {
		return nil, errors.New("CLI authentication context mismatch")
	}
	if send(map[string]any{"id": 3, "method": "account/rateLimits/read", "params": map[string]any{}}) != nil {
		return nil, errors.New("CLI quota read failed")
	}
	payload, err := read(3)
	if err != nil {
		return nil, err
	}
	// Current official protocol supplies accountId. Older versions lacking it
	// cannot prove workspace identity and deliberately fall back to direct polling.
	if textValue(payload["accountId"]) != credential.AccountID {
		return nil, errors.New("CLI quota account mismatch or unverified")
	}
	latest, err := c.load(c.id)
	if err != nil || latest.AccountID != credential.AccountID || latest.Subject != credential.Subject {
		return nil, errors.New("Torii account changed during CLI read")
	}
	return payload, nil
}
