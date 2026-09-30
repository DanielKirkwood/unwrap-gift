//go:build e2e

// This file's test is gated behind the "e2e" build tag because it needs a
// running Docker daemon: `task test:e2e` (or `go test -tags=e2e ./...`)
// runs it; plain `go test ./...` does not.
package kratosclient_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
	kratos "github.com/ory/kratos-client-go/v26"
	"github.com/testcontainers/testcontainers-go"
	tcnetwork "github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/DanielKirkwood/unwrap-gift/internal/clients/kratosclient"
	"github.com/DanielKirkwood/unwrap-gift/internal/config"
)

const (
	kratosImage = "oryd/kratos:v1.3.1"
	kratosDSN   = "postgres://kratos:kratos@postgres-kratos:5432/kratos?sslmode=disable&max_conns=20&max_idle_conns=4"

	containerStartupTimeout = 60 * time.Second
	verificationCodeTimeout = 30 * time.Second
	testWebhookSecret       = "dev-courier-webhook-secret-not-secure"
)

var loginCodeRe = regexp.MustCompile(`code is:\s*(\d{6})`)

// webhookStandInScript is a minimal HTTP server standing in for
// unwrap-gift's own /webhooks/kratos/sms handler: it checks the shared
// secret header and prints each accepted request body to stdout
// (container logs), which pollWebhookLogsForCode scans for the delivered
// code. It's plain Python (needing no build step) rather than Go, since
// testcontainers has no built-in way to run an inline Go HTTP handler as
// its own container without first building an image.
const webhookStandInScript = `
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

SECRET = "` + testWebhookSecret + `"

class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        body = self.rfile.read(length)
        if self.headers.get("X-Courier-Webhook-Secret") != SECRET:
            self.send_response(401)
            self.end_headers()
            return
        print("WEBHOOK_BODY:" + body.decode(), flush=True)
        self.send_response(204)
        self.end_headers()

    def log_message(self, fmt, *args):
        pass

HTTPServer(("0.0.0.0", 8082), Handler).serve_forever()
`

// TestPhoneCodeLogin_E2E spins up Postgres and Kratos directly via
// testcontainers-go (not deploy/kratos/docker-compose.kratos.yml's compose
// orchestration — see deploy/kratos/README.md for why), but reuses that
// same directory's kratos.yml/identity.schema.json/login_code.sms.jsonnet
// unmodified, so the Kratos configuration itself is never duplicated. It
// proves the full admin-create -> code-login -> courier-webhook-delivery
// path: an organiser-provisioned identity (no self-service registration
// ever run) completes a native (non-browser) code login once the SMS
// login code — captured by a stand-in webhook container, reached by Kratos
// via host.docker.internal exactly as kratos.yml itself points at in real
// local dev — is submitted back.
//
// The stand-in webhook runs as its own container on the shared test
// network (not a host-side httptest.Server) so host.docker.internal can be
// mapped straight to it: see startKratos's HostConfigModifier comment for
// why a host-side listener doesn't work here.
func TestPhoneCodeLogin_E2E(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	nw, err := tcnetwork.New(ctx)
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	testcontainers.CleanupNetwork(t, nw)

	startPostgres(ctx, t, nw.Name)
	runKratosMigrate(ctx, t, nw.Name)
	webhookContainer, webhookIP := startWebhookStandIn(ctx, t, nw.Name)

	publicURL, adminURL, kratosContainer := startKratos(ctx, t, nw.Name, webhookIP)

	client, err := kratosclient.New(config.KratosConfig{PublicURL: publicURL, AdminURL: adminURL}, true)
	if err != nil {
		t.Fatalf("kratosclient.New: %v", err)
	}

	const phone = "+12025550123"
	identity, err := client.CreateIdentity(ctx, map[string]any{
		"phone": phone, "full_name": "Test Participant",
	}, "")
	if err != nil {
		var apiErr *kratos.GenericOpenAPIError
		if errors.As(err, &apiErr) {
			t.Fatalf("CreateIdentity: %v\nbody: %s", err, apiErr.Body())
		}
		t.Fatalf("CreateIdentity: %v", err)
	}

	flowID := startNativeLoginFlow(ctx, t, publicURL)
	submitLoginIdentifier(ctx, t, publicURL, flowID, phone)

	code, ok := pollWebhookLogsForCode(ctx, t, webhookContainer, verificationCodeTimeout)
	if !ok {
		logs, _ := kratosContainer.Logs(ctx)
		if logs != nil {
			raw, _ := io.ReadAll(logs)
			t.Logf("kratos container logs:\n%s", raw)
		}
		t.Fatal("no SMS delivered to stand-in webhook within timeout")
	}

	loggedInIdentityID := submitLoginCode(ctx, t, publicURL, flowID, phone, code)
	if loggedInIdentityID != identity.Id {
		t.Fatalf("logged-in identity = %s, want %s", loggedInIdentityID, identity.Id)
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

// startWebhookStandIn starts the stand-in courier webhook container (see
// webhookStandInScript) on networkName, returning it and its IP address on
// that network.
func startWebhookStandIn(ctx context.Context, t *testing.T, networkName string) (testcontainers.Container, string) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        "python:3.12-slim",
		Networks:     []string{networkName},
		ExposedPorts: []string{"8082/tcp"},
		Files: []testcontainers.ContainerFile{
			{
				Reader:            strings.NewReader(webhookStandInScript),
				ContainerFilePath: "/webhook.py",
				FileMode:          0o644,
			},
		},
		Cmd:        []string{"python3", "/webhook.py"},
		WaitingFor: wait.ForListeningPort("8082/tcp").WithStartupTimeout(containerStartupTimeout),
	}

	c, err := testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true},
	)
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatalf("start webhook stand-in: %v", err)
	}

	ip, err := c.ContainerIP(ctx)
	if err != nil {
		t.Fatalf("webhook stand-in container IP: %v", err)
	}

	return c, ip
}

// pollWebhookLogsForCode polls webhookContainer's logs until loginCodeRe
// finds a delivered login code, or timeout elapses. Polling the
// container's own stdout (rather than a live Go channel) is what lets the
// webhook run as a container instead of a host-side listener — see
// startKratos's HostConfigModifier comment. It searches the whole log
// text directly rather than first isolating each WEBHOOK_BODY-prefixed
// line, since webhookStandInScript prints the (pretty-printed,
// multi-line) request body as-is — matching loginCodeRe across the raw
// log avoids needing a multi-line-aware wrapper regex.
func pollWebhookLogsForCode(
	ctx context.Context,
	t *testing.T,
	webhookContainer testcontainers.Container,
	timeout time.Duration,
) (string, bool) {
	t.Helper()

	const pollInterval = 500 * time.Millisecond

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		logs, err := webhookContainer.Logs(ctx)
		if err == nil {
			raw, _ := io.ReadAll(logs)
			_ = logs.Close()

			if m := loginCodeRe.FindStringSubmatch(string(raw)); m != nil {
				return m[1], true
			}
		}

		time.Sleep(pollInterval)
	}

	return "", false
}

func startKratos(
	ctx context.Context,
	t *testing.T,
	networkName string,
	webhookIP string,
) (string, string, testcontainers.Container) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:          kratosImage,
		Env:            map[string]string{"DSN": kratosDSN},
		ExposedPorts:   []string{"4433/tcp", "4434/tcp"},
		Networks:       []string{networkName},
		NetworkAliases: map[string][]string{networkName: {"kratos"}},
		Files:          kratosConfigFiles(t),
		Cmd:            []string{"serve", "-c", "/etc/config/kratos/kratos.yml", "--dev", "--watch-courier"},
		HostConfigModifier: func(hc *container.HostConfig) {
			// kratos.yml's courier SMS channel hardcodes
			// host.docker.internal:8082 — unwrap-gift's own hidden port in
			// real local dev, where the app runs on the host, not in a
			// container. Docker's "host-gateway" ExtraHosts value is the
			// portable way to make that resolve (and is what
			// docker-compose.kratos.yml uses for real local dev), but on
			// OrbStack specifically it resolves to a special address in
			// the reserved 0.0.0.0/8 range, which Kratos's courier-channel
			// HTTP client hard-refuses to dial ("prohibited IP address...
			// denied by: 0.0.0.0/8") — confirmed empirically, and not
			// something clients.http.disallow_private_ip_ranges (set
			// false in kratos.yml) controls. Mapping host.docker.internal
			// straight to the stand-in webhook container's own (ordinary,
			// non-reserved) IP address sidesteps that, while keeping
			// kratos.yml itself byte-for-byte unmodified.
			hc.ExtraHosts = []string{"host.docker.internal:" + webhookIP}
		},
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
		fmt.Sprintf("http://%s", net.JoinHostPort(host, adminPort.Port())), c
}

// kratosConfigFiles returns the same kratos.yml/identity.schema.json/
// login_code.sms.jsonnet this repo's local dev docker-compose.kratos.yml
// mounts, so the Kratos configuration itself has exactly one source of
// truth (see deploy/kratos/docker-compose.kratos.yml).
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
		{
			HostFilePath:      filepath.Join(root, "deploy", "kratos", "login_code.sms.jsonnet"),
			ContainerFilePath: "/etc/config/kratos/login_code.sms.jsonnet",
			FileMode:          0o644,
		},
	}
}

type flowResponse struct {
	ID string `json:"id"`
}

type loginResult struct {
	SessionToken string `json:"session_token"`
	Session      struct {
		Identity struct {
			ID string `json:"id"`
		} `json:"identity"`
	} `json:"session"`
}

// startNativeLoginFlow begins a native (type: api) login flow, returning
// its flow ID.
func startNativeLoginFlow(ctx context.Context, t *testing.T, publicURL string) string {
	t.Helper()

	var flow flowResponse
	doJSON(ctx, t, http.MethodGet, publicURL+"/self-service/login/api", nil, &flow)

	return flow.ID
}

// submitLoginIdentifier submits phone as the code method's identifier,
// triggering Kratos to generate and send a login code via the courier
// webhook. Unlike every other step in this flow, Kratos's native API
// intentionally responds 400 (not 2xx) here, carrying the updated —
// still in-progress, not erroneous — login flow as its body; that 400 is
// how a native client is told "the flow isn't complete yet, submit the
// code next", so this doesn't use doJSON's fatal-on-non-2xx check.
func submitLoginIdentifier(ctx context.Context, t *testing.T, publicURL, flowID, phone string) {
	t.Helper()

	payload, err := json.Marshal(map[string]any{"method": "code", "identifier": phone})
	if err != nil {
		t.Fatalf("marshal login identifier request: %v", err)
	}

	url := fmt.Sprintf("%s/self-service/login?flow=%s", publicURL, flowID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build POST %s: %v", url, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s returned %d, want 200 or 400: %s", url, resp.StatusCode, raw)
	}
}

// submitLoginCode completes the login flow by submitting code, returning
// the resulting session's identity ID. Kratos's native code-method login
// flow requires identifier to be resent alongside code on this second
// step — omitting it (as the plan's External Documentation summary of
// UpdateLoginFlowWithCodeMethod implied) fails with "Property identifier
// is missing", confirmed empirically here.
func submitLoginCode(ctx context.Context, t *testing.T, publicURL, flowID, phone, code string) string {
	t.Helper()

	var result loginResult
	doJSON(ctx, t, http.MethodPost, fmt.Sprintf("%s/self-service/login?flow=%s", publicURL, flowID),
		map[string]any{"method": "code", "identifier": phone, "code": code}, &result)

	if result.SessionToken == "" || result.Session.Identity.ID == "" {
		t.Fatalf("login did not return a session: %+v", result)
	}

	return result.Session.Identity.ID
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
