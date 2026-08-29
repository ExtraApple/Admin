package mailadapter_test

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	mailadapter "admin/internal/identity/adapters/mail"
	platformconfig "admin/internal/platform/config"
)

func TestSenderUsesConfiguredSMTPAndSendsVerificationTokenSynchronously(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen smtp test server: %v", err)
	}
	defer listener.Close()
	message := make(chan string, 1)
	serverErr := make(chan error, 1)
	go serveSMTPTestConnection(listener, message, serverErr)

	address := listener.Addr().String()
	host, portText, _ := net.SplitHostPort(address)
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatalf("parse smtp test port: %v", err)
	}
	sender, err := mailadapter.NewSender(platformconfig.SMTPConfig{Host: host, Port: port, Username: "mailer", Password: "secret", From: "no-reply@example.test", TimeoutSeconds: 2, TLSMode: "disabled"})
	if err != nil {
		t.Fatalf("NewSender() error = %v", err)
	}
	if err := sender.SendVerification(context.Background(), "user@example.test", "verification-token"); err != nil {
		t.Fatalf("SendVerification() error = %v", err)
	}
	select {
	case err := <-serverErr:
		t.Fatalf("smtp server error: %v", err)
	case raw := <-message:
		if !strings.Contains(raw, "user@example.test") || !strings.Contains(raw, "verification-tok") {
			t.Fatalf("SMTP message = %s", raw)
		}
	case <-time.After(time.Second):
		t.Fatal("SMTP test server did not receive message")
	}
}

func serveSMTPTestConnection(listener net.Listener, message chan<- string, serverErr chan<- error) {
	connection, err := listener.Accept()
	if err != nil {
		serverErr <- err
		return
	}
	defer connection.Close()
	reader := bufio.NewReader(connection)
	write := func(value string) error {
		_, err := connection.Write([]byte(value + "\r\n"))
		return err
	}
	if err := write("220 smtp.test ESMTP"); err != nil {
		serverErr <- err
		return
	}
	var data strings.Builder
	inData := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			serverErr <- err
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if inData {
			if line == "." {
				inData = false
				if err := write("250 2.0.0 accepted"); err != nil {
					serverErr <- err
					return
				}
				message <- data.String()
				continue
			}
			data.WriteString(line)
			data.WriteString("\n")
			continue
		}
		switch {
		case strings.HasPrefix(strings.ToUpper(line), "EHLO"), strings.HasPrefix(strings.ToUpper(line), "HELO"):
			if _, err := connection.Write([]byte("250-smtp.test\r\n250-AUTH PLAIN LOGIN\r\n250 OK\r\n")); err != nil {
				serverErr <- err
				return
			}
		case strings.HasPrefix(strings.ToUpper(line), "AUTH"):
			if err := write("235 2.7.0 authenticated"); err != nil {
				serverErr <- err
				return
			}
		case strings.HasPrefix(strings.ToUpper(line), "MAIL FROM"), strings.HasPrefix(strings.ToUpper(line), "RCPT TO"):
			if err := write("250 2.1.0 ok"); err != nil {
				serverErr <- err
				return
			}
		case strings.EqualFold(line, "DATA"):
			inData = true
			if err := write("354 end with <CRLF>.<CRLF>"); err != nil {
				serverErr <- err
				return
			}
		case strings.EqualFold(line, "QUIT"):
			_ = write("221 2.0.0 bye")
			return
		default:
			if err := write("250 2.0.0 ok"); err != nil {
				serverErr <- err
				return
			}
		}
	}
}
