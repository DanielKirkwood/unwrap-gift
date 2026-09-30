// Package smsclient wraps seven.io's Go SDK (sms77api) behind the
// Send(ctx, to, body) method internal/api's SMSSender interface expects
// (see Phase 2's plan, .claude/PRPs/plans/secret-santa-sms-auth.plan.md) —
// api never imports this package directly, per ARCHITECTURE.md's
// dependency-direction rule.
package smsclient

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/seven-io/go-client/sms77api"

	"github.com/DanielKirkwood/unwrap-gift/internal/config"
)

// Sender is the narrow seam smsclient.Client sends through, satisfied in
// production by *sms77api.SmsResource (via New) and by a fake in tests.
// It exists because sms77api's HTTP transport and base URL
// ("https://gateway.seven.io/api/...") are both unexported and hardcoded
// inside the SDK — unlike kratosclient/ketoclient, there is no way to
// point the real SDK at a local httptest.Server, so this interface is
// the substitution point instead. Not to be confused with
// internal/api.SMSSender (Phase 2), a differently-shaped interface one
// layer up that *Client itself implements.
type Sender interface {
	JsonContext(ctx context.Context, p sms77api.SmsBaseParams) (*sms77api.SmsResponse, error)
}

// Client sends SMS messages via seven.io, or — when the sms feature is
// disabled — logs the message instead of sending it. Unlike
// kratosclient/ketoclient's "nil is disabled" convention, Client is
// NEVER nil: Phase 2's Kratos courier webhook and Phase 7's draw
// notification pipeline both call Send unconditionally, and a swallowed
// login code or draw notification would make local dev untestable. This
// is a deliberate, documented deviation from ARCHITECTURE.md's default
// convention — see New.
type Client struct {
	// Sender is the seven.io SMS-sending seam Send calls through when
	// Enabled is true. Exported so tests can substitute a fake directly,
	// mirroring kratosclient.Client's exported Public/Admin fields.
	Sender Sender
	// Enabled reports whether Send calls seven.io (true) or logs the
	// message instead (false).
	Enabled bool
	// From is the configured SEVEN_SENDER_ID, sent as every message's
	// From field.
	From string
	// Logger receives the "sms disabled" log line when Enabled is false.
	// Must be non-nil whenever Client is constructed via New.
	Logger *slog.Logger
}

// New builds a Client from cfg. When enabled is false, cfg is ignored and
// the returned Client is still non-nil — Send logs instead of sending.
// logger is required in both branches (see Client.Logger).
func New(cfg config.SMSConfig, enabled bool, logger *slog.Logger) (*Client, error) {
	if !enabled {
		return &Client{Enabled: false, Logger: logger}, nil
	}

	api := sms77api.New(sms77api.Options{ApiKey: cfg.APIKey})

	return &Client{Sender: api.Sms, Enabled: true, From: cfg.SenderID, Logger: logger}, nil
}

// Send implements api.SMSSender (internal/api/courier.go, Phase 2; and
// later Phase 7's notification pipeline) by sending body to the to phone
// number. When Enabled is false, it logs the message via slog at Info
// level instead of calling seven.io — no network call happens at all in
// that branch, which is what keeps local dev (and CI) from ever sending
// a real SMS.
func (c *Client) Send(ctx context.Context, to, body string) error {
	if !c.Enabled {
		c.Logger.InfoContext(ctx, "sms disabled: logging instead of sending", "to", to, "body", body)
		return nil
	}

	resp, err := c.Sender.JsonContext(ctx, sms77api.SmsBaseParams{To: to, From: c.From, Text: body})
	if err != nil {
		return fmt.Errorf("smsclient: send: %w", err)
	}

	return checkSmsResponse(resp)
}

// checkSmsResponse inspects each per-recipient result for failure,
// falling back to the top-level status when seven.io returns no
// per-message detail at all. See this plan's GOTCHA on seven.io's
// undocumented success-detection shape — this has not been confirmed
// against a live account.
func checkSmsResponse(resp *sms77api.SmsResponse) error {
	if len(resp.Messages) == 0 {
		if resp.Success != sms77api.StatusCodeSuccess && resp.Success != sms77api.StatusCodeSuccessPartial {
			return fmt.Errorf("smsclient: send: seven.io returned status %q with no per-message detail", resp.Success)
		}
		return nil
	}

	for _, m := range resp.Messages {
		if !m.Success {
			detail := "unknown error"
			if m.ErrorText != nil {
				detail = *m.ErrorText
			}
			return fmt.Errorf("smsclient: send: seven.io rejected message to %s: %s", m.Recipient, detail)
		}
	}

	return nil
}
