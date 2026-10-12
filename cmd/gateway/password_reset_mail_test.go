package main

import (
	"bufio"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestResetSMTPVerifiedTLSDeliveryAndNoDowngrade(t *testing.T) {
	certificateServer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	certificate := certificateServer.TLS.Certificates[0]
	roots := x509.NewCertPool()
	roots.AddCert(certificateServer.Certificate())
	certificateServer.Close()
	for _, mode := range []string{"starttls", "implicit"} {
		t.Run(mode, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			captured := make(chan string, 1)
			failures := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					failures <- err
					return
				}
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				serverTLS := &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
				if mode == "implicit" {
					conn = tls.Server(conn, serverTLS)
				}
				reader := bufio.NewReader(conn)
				fmt.Fprint(conn, "220 localhost SMTP\r\n")
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						failures <- err
						return
					}
					switch {
					case strings.HasPrefix(line, "EHLO"):
						if _, ok := conn.(*tls.Conn); ok {
							fmt.Fprint(conn, "250-localhost\r\n250 AUTH PLAIN\r\n")
						} else {
							fmt.Fprint(conn, "250-localhost\r\n250 STARTTLS\r\n")
						}
					case strings.HasPrefix(line, "STARTTLS"):
						fmt.Fprint(conn, "220 Upgrade\r\n")
						conn = tls.Server(conn, serverTLS)
						reader = bufio.NewReader(conn)
					case strings.HasPrefix(line, "AUTH PLAIN"):
						if _, ok := conn.(*tls.Conn); !ok {
							failures <- fmt.Errorf("credentials sent before TLS")
							return
						}
						fmt.Fprint(conn, "235 Authenticated\r\n")
					case strings.HasPrefix(line, "MAIL FROM:") || strings.HasPrefix(line, "RCPT TO:"):
						fmt.Fprint(conn, "250 OK\r\n")
					case line == "DATA\r\n":
						fmt.Fprint(conn, "354 Send message\r\n")
						var message strings.Builder
						for {
							line, err = reader.ReadString('\n')
							if err != nil {
								failures <- err
								return
							}
							if line == ".\r\n" {
								break
							}
							message.WriteString(line)
						}
						captured <- message.String()
						fmt.Fprint(conn, "250 Queued\r\n")
					case line == "QUIT\r\n":
						fmt.Fprint(conn, "221 Bye\r\n")
						return
					default:
						failures <- fmt.Errorf("unexpected SMTP command")
						return
					}
				}
			}()
			base, _ := url.Parse("https://gateway.example")
			token := strings.Repeat("a", 64)
			message := passwordResetMessage(base, "torii@example.com", "admin@example.com", token)
			err = sendResetSMTP(listener.Addr().String(), "127.0.0.1", mode, "relay-user", "synthetic-password", "torii@example.com", "admin@example.com", message, &tls.Config{ServerName: "127.0.0.1", RootCAs: roots, MinVersion: tls.VersionTLS12})
			if err != nil {
				t.Fatal(err)
			}
			select {
			case got := <-captured:
				if !strings.Contains(got, "https://gateway.example/reset-password#"+token) || !strings.Contains(got, "30 minutes") || strings.Contains(got, "synthetic-password") {
					t.Fatal("unsafe reset message")
				}
			case err := <-failures:
				t.Fatal(err)
			case <-time.After(2 * time.Second):
				t.Fatal("email missing")
			}
		})
	}
	t.Run("reject plaintext relay", func(t *testing.T) {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		commands := make(chan string, 1)
		go func() {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(3 * time.Second))
			fmt.Fprint(conn, "220 SMTP\r\n")
			reader := bufio.NewReader(conn)
			reader.ReadString('\n')
			fmt.Fprint(conn, "250 localhost\r\n")
			line, _ := reader.ReadString('\n')
			commands <- line
		}()
		err = sendResetSMTP(listener.Addr().String(), "127.0.0.1", "starttls", "relay-user", "synthetic-password", "torii@example.com", "admin@example.com", "message", &tls.Config{ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12})
		if err == nil {
			t.Fatal("TLS downgrade accepted")
		}
		select {
		case command := <-commands:
			if strings.Contains(command, "AUTH") || strings.Contains(command, "MAIL") {
				t.Fatal("credentials/mail sent before TLS")
			}
		case <-time.After(4 * time.Second):
			t.Fatal("client did not close")
		}
	})
}
