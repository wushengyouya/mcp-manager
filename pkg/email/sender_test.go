package email

import (
	"bytes"
	"errors"
	"testing"

	"gopkg.in/gomail.v2"
)

func TestNewSMTPSender(t *testing.T) {
	sender := NewSMTPSender("smtp.example.com", 587, "user", "pass")
	if sender == nil {
		t.Fatal("expected sender")
	}
}

func TestSMTPSenderSendBuildsMessage(t *testing.T) {
	var captured *gomail.Message
	sender := &SMTPSender{
		host:     "smtp.example.com",
		port:     587,
		username: "user",
		password: "pass",
		sendFunc: func(msg *gomail.Message) error {
			captured = msg
			return nil
		},
	}

	err := sender.Send("from@example.com", []string{"to1@example.com", "to2@example.com"}, "Test Subject", "hello body")
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if captured == nil {
		t.Fatal("expected captured message")
	}
	if got := captured.GetHeader("From"); len(got) != 1 || got[0] != "from@example.com" {
		t.Fatalf("unexpected From header: %#v", got)
	}
	if got := captured.GetHeader("To"); len(got) != 2 || got[0] != "to1@example.com" || got[1] != "to2@example.com" {
		t.Fatalf("unexpected To header: %#v", got)
	}
	if got := captured.GetHeader("Subject"); len(got) != 1 || got[0] != "Test Subject" {
		t.Fatalf("unexpected Subject header: %#v", got)
	}

	var buf bytes.Buffer
	if _, err := captured.WriteTo(&buf); err != nil {
		t.Fatalf("WriteTo() error = %v", err)
	}
	body := buf.String()
	if !bytes.Contains([]byte(body), []byte("hello body")) {
		t.Fatalf("expected body to contain payload, got %q", body)
	}
}

func TestSMTPSenderSendReturnsUnderlyingError(t *testing.T) {
	wantErr := errors.New("send failed")
	sender := &SMTPSender{
		sendFunc: func(msg *gomail.Message) error {
			return wantErr
		},
	}

	err := sender.Send("from@example.com", []string{"to@example.com"}, "Subject", "body")
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
}
