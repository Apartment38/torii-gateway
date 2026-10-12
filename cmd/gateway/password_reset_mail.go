package main

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"os"
	"strings"
	"time"
)

// Links use an operator-configured HTTPS origin, never the request Host header.
func passwordResetSenderFromEnv() (func(string, string) error, error) {
	base, err := url.Parse(os.Getenv("TORII_PUBLIC_URL"))
	if err != nil || base.Scheme != "https" || base.Hostname() == "" || base.User != nil || base.RawQuery != "" || base.ForceQuery || base.Fragment != "" || (base.Path != "" && base.Path != "/") {
		return nil, errors.New("TORII_PUBLIC_URL must be an HTTPS origin")
	}
	address := os.Getenv("TORII_SMTP_ADDR")
	host, _, err := net.SplitHostPort(address)
	if err != nil || host == "" {
		return nil, errors.New("TORII_SMTP_ADDR must be host:port")
	}
	from := os.Getenv("TORII_SMTP_FROM")
	parsed, err := mail.ParseAddress(from)
	if err != nil || parsed.Address != from || strings.ContainsAny(from, "\r\n") {
		return nil, errors.New("TORII_SMTP_FROM must be an email address")
	}
	mode := os.Getenv("TORII_SMTP_TLS")
	if mode == "" {
		mode = "starttls"
	}
	if mode != "starttls" && mode != "implicit" {
		return nil, errors.New("TORII_SMTP_TLS must be starttls or implicit")
	}
	username := os.Getenv("TORII_SMTP_USERNAME")
	password := os.Getenv("TORII_SMTP_PASSWORD")
	if path := os.Getenv("TORII_SMTP_PASSWORD_FILE"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, errors.New("cannot read SMTP password file")
		}
		password = strings.TrimRight(string(data), "\r\n")
	}
	if (username == "") != (password == "") {
		return nil, errors.New("configure both SMTP username and password")
	}
	return func(email, token string) error {
		message := passwordResetMessage(base, from, email, token)
		return sendResetSMTP(address, host, mode, username, password, from, email, message, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	}, nil
}

func passwordResetMessage(base *url.URL, from, email, token string) string {
	link := *base
	link.Path = "/reset-password"
	link.Fragment = token
	// Tokens stay in URL fragments, out of access logs and Referer headers.
	return fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: Reset your Torii administrator password\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\nUse this link to choose a new password within 30 minutes:\r\n%s\r\n\r\nThe link can be used once. If you did not request this, ignore this email.\r\n", from, email, link.String())
}

func sendResetSMTP(address, host, mode, username, password, from, email, message string, tlsConfig *tls.Config) error {
	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err = conn.SetDeadline(time.Now().Add(20 * time.Second)); err != nil {
		return err
	}
	if mode == "implicit" {
		conn = tls.Client(conn, tlsConfig)
	}
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()
	if mode == "starttls" {
		if err = client.StartTLS(tlsConfig); err != nil {
			return err
		}
	}
	if username != "" {
		if err = client.Auth(smtp.PlainAuth("", username, password, host)); err != nil {
			return err
		}
	}
	if err = client.Mail(from); err != nil {
		return err
	}
	if err = client.Rcpt(email); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err = writer.Write([]byte(message)); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
