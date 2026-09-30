package smsclient_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/seven-io/go-client/sms77api"

	"github.com/DanielKirkwood/unwrap-gift/internal/clients/smsclient"
	"github.com/DanielKirkwood/unwrap-gift/internal/config"
)

type fakeSender struct {
	resp                    *sms77api.SmsResponse
	err                     error
	called                  bool
	gotTo, gotFrom, gotText string
}

//nolint:revive,staticcheck // name must match smsclient.Sender's JsonContext exactly, which itself mirrors the seven.io SDK's own method name.
func (f *fakeSender) JsonContext(_ context.Context, p sms77api.SmsBaseParams) (*sms77api.SmsResponse, error) {
	f.called = true
	f.gotTo, f.gotFrom, f.gotText = p.To, p.From, p.Text
	return f.resp, f.err
}

func testLogger(buf *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, nil))
}

func TestNewDisabled(t *testing.T) {
	t.Parallel()

	client, err := smsclient.New(config.SMSConfig{}, false, slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if client == nil {
		t.Fatal("New() = nil, want non-nil (sms client is never nil — see Client's doc comment)")
	}
	if client.Enabled {
		t.Error("Enabled = true, want false")
	}
}

func TestNewEnabled(t *testing.T) {
	t.Parallel()

	cfg := config.SMSConfig{APIKey: "test-key", SenderID: "TestSender"}

	client, err := smsclient.New(cfg, true, slog.Default())
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	if !client.Enabled {
		t.Error("Enabled = false, want true")
	}
	if client.Sender == nil {
		t.Error("Sender = nil, want non-nil")
	}
	if client.From != "TestSender" {
		t.Errorf("From = %q, want %q", client.From, "TestSender")
	}
}

func TestClient_Send_Disabled_LogsInsteadOfSending(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	client := &smsclient.Client{Enabled: false, Logger: testLogger(&buf)}

	if err := client.Send(t.Context(), "+15550001234", "your code is 123456"); err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}

	got := buf.String()
	if !strings.Contains(got, "+15550001234") || !strings.Contains(got, "your code is 123456") {
		t.Errorf("log output = %q, want it to contain the to/body", got)
	}
	// client.Sender is nil here; if Send incorrectly fell through to call
	// it, this test would panic rather than reach this assertion.
}

func TestClient_Send_Success(t *testing.T) {
	t.Parallel()

	fake := &fakeSender{resp: &sms77api.SmsResponse{
		Success:  sms77api.StatusCodeSuccess,
		Messages: []sms77api.SmsResponseMessage{{Recipient: "+15550001234", Success: true}},
	}}
	client := &smsclient.Client{Sender: fake, Enabled: true, From: "TestSender", Logger: slog.Default()}

	if err := client.Send(t.Context(), "+15550001234", "hello"); err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}
	if !fake.called {
		t.Error("fakeSender was not called")
	}
	if fake.gotTo != "+15550001234" || fake.gotFrom != "TestSender" || fake.gotText != "hello" {
		t.Errorf(
			"JsonContext params = (%q, %q, %q), want (+15550001234, TestSender, hello)",
			fake.gotTo,
			fake.gotFrom,
			fake.gotText,
		)
	}
}

func TestClient_Send_MessageRejected(t *testing.T) {
	t.Parallel()

	errText := "invalid recipient"
	fake := &fakeSender{resp: &sms77api.SmsResponse{
		Success: sms77api.StatusCodeErrorUnknown,
		Messages: []sms77api.SmsResponseMessage{
			{Recipient: "+15550001234", Success: false, ErrorText: &errText},
		},
	}}
	client := &smsclient.Client{Sender: fake, Enabled: true, Logger: slog.Default()}

	err := client.Send(t.Context(), "+15550001234", "hello")
	if err == nil || !strings.Contains(err.Error(), errText) {
		t.Errorf("Send() error = %v, want it to contain %q", err, errText)
	}
}

func TestClient_Send_TransportError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	fake := &fakeSender{err: boom}
	client := &smsclient.Client{Sender: fake, Enabled: true, Logger: slog.Default()}

	err := client.Send(t.Context(), "+15550001234", "hello")
	if !errors.Is(err, boom) {
		t.Errorf("Send() error = %v, want errors.Is(err, boom)", err)
	}
}
