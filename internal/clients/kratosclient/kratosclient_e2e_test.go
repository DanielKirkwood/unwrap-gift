//go:build e2e

// This file's test is gated behind the "e2e" build tag because it needs a
// running Docker daemon: `task test:e2e` (or `go test -tags=e2e ./...`)
// runs it; plain `go test ./...` does not.
package kratosclient_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcnetwork "github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/DanielKirkwood/unwrap-gift/internal/clients/kratosclient"
	"github.com/DanielKirkwood/unwrap-gift/internal/config"
)

const (
	kratosImage = "oryd/kratos:v1.3.1"
	kratosDSN   = "postgres://kratos:kratos@postgres-kratos:5432/kratos?sslmode=disable&max_conns=20&max_idle_conns=4"

	containerStartupTimeout  = 60 * time.Second
	verificationCodeTimeout  = 30 * time.Second
	verificationPollInterval = 500 * time.Millisecond
)

// TestRegistrationAndVerification_E2E spins up Postgres, Kratos, and
// mailslurper directly via testcontainers-go (not
// deploy/kratos/docker-compose.kratos.yml's compose orchestration — see
// deploy/kratos/README.md for why), but reuses that same directory's
// kratos.yml/identity.schema.json unmodified, so the Kratos configuration
// itself is never duplicated. It drives Kratos's native (non-browser)
// self-service API — matching how a backend/mobile client, not our own
// cookie-based AuthenticationMiddleware, would integrate — making this a
// regression test for deploy/kratos's config, not for our own HTTP
// handlers: it would have caught the smtp:// vs smtps:// courier bug fixed
// earlier in kratos.yml.
func TestRegistrationAndVerification_E2E(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	nw, err := tcnetwork.New(ctx)
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	testcontainers.CleanupNetwork(t, nw)

	startPostgres(ctx, t, nw.Name)
	runKratosMigrate(ctx, t, nw.Name)
	publicURL, adminURL := startKratos(ctx, t, nw.Name)
	mailAPIURL := startMailslurper(ctx, t, nw.Name)

	client, err := kratosclient.New(config.KratosConfig{PublicURL: publicURL, AdminURL: adminURL}, true)
	if err != nil {
		t.Fatalf("kratosclient.New: %v", err)
	}

	const email = "e2e-verify@example.test"

	identityID := registerNative(ctx, t, publicURL, email, "correct-horse-battery-staple-42")

	// The identity schema marks the email trait `verification: {via:
	// email}`, so Kratos auto-sends a verification email as a
	// registration side effect — independent of, and racing against, the
	// one our own startVerificationFlow call below triggers. Both emails
	// go to the same address, so waitForVerificationCode disambiguates by
	// flow ID (embedded in the email's verification link) rather than by
	// arrival order, which isn't guaranteed.
	flowID := startVerificationFlow(ctx, t, publicURL, email)
	code := waitForVerificationCode(ctx, t, mailAPIURL, email, flowID, verificationCodeTimeout)
	submitVerificationCode(ctx, t, publicURL, flowID, code)

	identity, err := client.GetIdentity(ctx, identityID)
	if err != nil {
		t.Fatalf("GetIdentity: %v", err)
	}
	if len(identity.VerifiableAddresses) == 0 || !identity.VerifiableAddresses[0].Verified {
		t.Fatalf("identity not verified after submitting code: %+v", identity.VerifiableAddresses)
	}
}

func startPostgres(ctx context.Context, t *testing.T, networkName string) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image: "postgres:17",
		Env: map[string]string{
			"POSTGRES_USER":     "kratos",
			"POSTGRES_PASSWORD": "kratos",
			"POSTGRES_DB":       "kratos",
		},
		ExposedPorts:   []string{"5432/tcp"},
		Networks:       []string{networkName},
		NetworkAliases: map[string][]string{networkName: {"postgres-kratos"}},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(containerStartupTimeout),
	}

	c, err := testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true},
	)
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
}

func runKratosMigrate(ctx context.Context, t *testing.T, networkName string) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:      kratosImage,
		Env:        map[string]string{"DSN": kratosDSN},
		Networks:   []string{networkName},
		Files:      kratosConfigFiles(t),
		Cmd:        []string{"-c", "/etc/config/kratos/kratos.yml", "migrate", "sql", "-e", "--yes"},
		WaitingFor: wait.ForExit().WithExitTimeout(containerStartupTimeout),
	}

	c, err := testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true},
	)
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatalf("run kratos migrate: %v", err)
	}

	state, err := c.State(ctx)
	if err != nil {
		t.Fatalf("kratos migrate state: %v", err)
	}
	if state.ExitCode != 0 {
		t.Fatalf("kratos migrate exited %d, want 0", state.ExitCode)
	}
}

func startKratos(ctx context.Context, t *testing.T, networkName string) (string, string) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:          kratosImage,
		Env:            map[string]string{"DSN": kratosDSN},
		ExposedPorts:   []string{"4433/tcp", "4434/tcp"},
		Networks:       []string{networkName},
		NetworkAliases: map[string][]string{networkName: {"kratos"}},
		Files:          kratosConfigFiles(t),
		Cmd:            []string{"serve", "-c", "/etc/config/kratos/kratos.yml", "--dev", "--watch-courier"},
		WaitingFor: wait.ForHTTP("/health/ready").
			WithPort("4433/tcp").
			WithStartupTimeout(containerStartupTimeout),
	}

	c, err := testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true},
	)
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatalf("start kratos: %v", err)
	}

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("kratos host: %v", err)
	}

	publicPort, err := c.MappedPort(ctx, "4433/tcp")
	if err != nil {
		t.Fatalf("kratos public mapped port: %v", err)
	}

	adminPort, err := c.MappedPort(ctx, "4434/tcp")
	if err != nil {
		t.Fatalf("kratos admin mapped port: %v", err)
	}

	return fmt.Sprintf("http://%s", net.JoinHostPort(host, publicPort.Port())),
		fmt.Sprintf("http://%s", net.JoinHostPort(host, adminPort.Port()))
}

func startMailslurper(ctx context.Context, t *testing.T, networkName string) string {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:          "oryd/mailslurper:latest-smtps",
		ExposedPorts:   []string{"1025/tcp", "4436/tcp", "4437/tcp"},
		Networks:       []string{networkName},
		NetworkAliases: map[string][]string{networkName: {"mailslurper"}},
		WaitingFor: wait.ForHTTP("/").
			WithPort("4436/tcp").
			WithStartupTimeout(containerStartupTimeout),
	}

	c, err := testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true},
	)
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatalf("start mailslurper: %v", err)
	}

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("mailslurper host: %v", err)
	}

	apiPort, err := c.MappedPort(ctx, "4437/tcp")
	if err != nil {
		t.Fatalf("mailslurper api mapped port: %v", err)
	}

	return fmt.Sprintf("http://%s", net.JoinHostPort(host, apiPort.Port()))
}

// kratosConfigFiles returns the same kratos.yml/identity.schema.json this
// repo's local dev docker-compose.kratos.yml mounts, so the Kratos
// configuration itself has exactly one source of truth.
func kratosConfigFiles(t *testing.T) []testcontainers.ContainerFile {
	t.Helper()

	// This file lives at internal/clients/kratosclient/, three levels
	// below the repo root.
	root := filepath.Join("..", "..", "..")

	return []testcontainers.ContainerFile{
		{
			HostFilePath:      filepath.Join(root, "deploy", "kratos", "kratos.yml"),
			ContainerFilePath: "/etc/config/kratos/kratos.yml",
			FileMode:          0o644,
		},
		{
			HostFilePath:      filepath.Join(root, "deploy", "kratos", "identity.schema.json"),
			ContainerFilePath: "/etc/config/kratos/identity.schema.json",
			FileMode:          0o644,
		},
	}
}

type flowResponse struct {
	ID string `json:"id"`
}

type registrationResult struct {
	SessionToken string `json:"session_token"`
	Session      struct {
		Identity struct {
			ID string `json:"id"`
		} `json:"identity"`
	} `json:"session"`
}

// registerNative drives Kratos's native (type: api) registration flow —
// no CSRF token or cookie jar needed, unlike the browser flow — and
// returns the newly created identity's ID.
func registerNative(ctx context.Context, t *testing.T, publicURL, email, password string) string {
	t.Helper()

	var flow flowResponse
	doJSON(ctx, t, http.MethodGet, publicURL+"/self-service/registration/api", nil, &flow)

	var result registrationResult
	doJSON(ctx, t, http.MethodPost, fmt.Sprintf("%s/self-service/registration?flow=%s", publicURL, flow.ID),
		map[string]any{
			"method":   "password",
			"password": password,
			"traits":   map[string]any{"email": email},
		}, &result)

	if result.SessionToken == "" || result.Session.Identity.ID == "" {
		t.Fatalf("registration did not return a session: %+v", result)
	}

	return result.Session.Identity.ID
}

// startVerificationFlow begins a native verification flow and requests a
// code be sent to email, returning the flow ID that code must be submitted
// against.
func startVerificationFlow(ctx context.Context, t *testing.T, publicURL, email string) string {
	t.Helper()

	var flow flowResponse
	doJSON(ctx, t, http.MethodGet, publicURL+"/self-service/verification/api", nil, &flow)

	doJSON(ctx, t, http.MethodPost, fmt.Sprintf("%s/self-service/verification?flow=%s", publicURL, flow.ID),
		map[string]any{"method": "code", "email": email}, nil)

	return flow.ID
}

func submitVerificationCode(ctx context.Context, t *testing.T, publicURL, flowID, code string) {
	t.Helper()

	doJSON(ctx, t, http.MethodPost, fmt.Sprintf("%s/self-service/verification?flow=%s", publicURL, flowID),
		map[string]any{"method": "code", "code": code}, nil)
}

type mailItem struct {
	ToAddresses []string `json:"toAddresses"`
	Body        string   `json:"body"`
}

type mailResponse struct {
	MailItems []mailItem `json:"mailItems"`
}

// waitForVerificationCode polls mailslurper's REST API until a message
// addressed to toEmail arrives whose body embeds flowID — Kratos's
// verification email always includes a link of the form
// ".../verification?code=<code>&flow=<flowID>", so matching on that
// string is how this disambiguates from the *other* verification email
// registration's identity-schema hook sends to the same address, racing
// against this one, tied to a different flow ID. It then extracts the
// matching email's 6-digit code from the body.
func waitForVerificationCode(
	ctx context.Context,
	t *testing.T,
	mailAPIURL, toEmail, flowID string,
	timeout time.Duration,
) string {
	t.Helper()

	codeRe := regexp.MustCompile(`code:\s*\n*\s*(\d{6})`)

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var mail mailResponse
		doJSON(ctx, t, http.MethodGet, mailAPIURL+"/mail", nil, &mail)

		for _, item := range mail.MailItems {
			if !slices.Contains(item.ToAddresses, toEmail) || !strings.Contains(item.Body, flowID) {
				continue
			}
			if m := codeRe.FindStringSubmatch(item.Body); m != nil {
				return m[1]
			}
		}

		time.Sleep(verificationPollInterval)
	}

	t.Fatalf("no verification email for %s matching flow %s arrived within %s", toEmail, flowID, timeout)
	return ""
}

// doJSON performs an HTTP request with an optional JSON body, and decodes
// the JSON response into out (skipped when out is nil). It fails the test
// immediately on any transport error, non-2xx status, or decode error.
func doJSON(ctx context.Context, t *testing.T, method, url string, body, out any) {
	t.Helper()

	var reqBody io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request body for %s %s: %v", method, url, err)
		}
		reqBody = bytes.NewReader(payload)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, url, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body from %s %s: %v", method, url, err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		t.Fatalf("%s %s returned %d: %s", method, url, resp.StatusCode, raw)
	}
	if out == nil {
		return
	}
	if unmarshalErr := json.Unmarshal(raw, out); unmarshalErr != nil {
		t.Fatalf("decode response from %s %s: %v (body: %s)", method, url, unmarshalErr, raw)
	}
}
