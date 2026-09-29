//go:build e2e

// This file's test is gated behind the "e2e" build tag because it needs a
// running Docker daemon: `task test:e2e` (or `go test -tags=e2e ./...`)
// runs it; plain `go test ./...` does not.
package ketoclient_test

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"testing"
	"time"

	keto "github.com/ory/keto-client-go/v26"
	"github.com/testcontainers/testcontainers-go"
	tcnetwork "github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/DanielKirkwood/unwrap-gift/internal/clients/ketoclient"
	"github.com/DanielKirkwood/unwrap-gift/internal/config"
)

const (
	ketoImage = "oryd/keto:v26.2.0"
	ketoDSN   = "postgres://keto:keto@postgres-keto:5432/keto?sslmode=disable&max_conns=20&max_idle_conns=4"

	containerStartupTimeout = 60 * time.Second
)

// TestCheckPermission_E2E spins up Postgres and Keto directly via
// testcontainers-go (not deploy/keto/docker-compose.keto.yml's compose
// orchestration — see deploy/keto/README.md for why), but reuses that same
// directory's keto.yml/identities.ts unmodified, so the Keto configuration
// itself is never duplicated. It seeds the same two relation tuples
// cmd/db.go's seedKeto creates, then asserts AuthorizationMiddleware's real
// dependency — CheckPermission — allows the seeded identity and denies an
// arbitrary other one, making this a regression test for deploy/keto's
// config and identities.ts's OPL logic, not just for a mocked HTTP
// response.
func TestCheckPermission_E2E(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	nw, err := tcnetwork.New(ctx)
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	testcontainers.CleanupNetwork(t, nw)

	startPostgres(ctx, t, nw.Name)
	runKetoMigrate(ctx, t, nw.Name)
	readURL, writeURL := startKeto(ctx, t, nw.Name)

	client, err := ketoclient.New(config.KetoConfig{ReadURL: readURL, WriteURL: writeURL}, true)
	if err != nil {
		t.Fatalf("ketoclient.New: %v", err)
	}

	const adminIdentityID = "e2e-admin-identity"
	const otherIdentityID = "e2e-other-identity"

	if seedErr := client.CreateRelationTuple(
		ctx,
		"Role",
		"admin",
		"members",
		ptr(adminIdentityID),
		nil,
	); seedErr != nil {
		t.Fatalf("seed Role:admin#members: %v", seedErr)
	}

	managerRole := &keto.SubjectSet{Namespace: "Role", Object: "admin", Relation: "members"}
	if seedErr := client.CreateRelationTuple(ctx, "Identities", "admin", "managers", nil, managerRole); seedErr != nil {
		t.Fatalf("seed Identities:admin#managers: %v", seedErr)
	}

	allowed, err := client.CheckPermission(ctx, "Identities", "admin", "manage", adminIdentityID)
	if err != nil {
		t.Fatalf("CheckPermission(admin) error: %v", err)
	}
	if !allowed {
		t.Error("CheckPermission(admin) = false, want true")
	}

	denied, err := client.CheckPermission(ctx, "Identities", "admin", "manage", otherIdentityID)
	if err != nil {
		t.Fatalf("CheckPermission(other) error: %v", err)
	}
	if denied {
		t.Error("CheckPermission(other) = true, want false")
	}
}

func ptr(s string) *string { return &s }

func startPostgres(ctx context.Context, t *testing.T, networkName string) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image: "postgres:17",
		Env: map[string]string{
			"POSTGRES_USER":     "keto",
			"POSTGRES_PASSWORD": "keto",
			"POSTGRES_DB":       "keto",
		},
		ExposedPorts:   []string{"5432/tcp"},
		Networks:       []string{networkName},
		NetworkAliases: map[string][]string{networkName: {"postgres-keto"}},
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

func runKetoMigrate(ctx context.Context, t *testing.T, networkName string) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:      ketoImage,
		Env:        map[string]string{"DSN": ketoDSN},
		Networks:   []string{networkName},
		Files:      ketoConfigFiles(t),
		Cmd:        []string{"-c", "/etc/config/keto/keto.yml", "migrate", "up", "-y"},
		WaitingFor: wait.ForExit().WithExitTimeout(containerStartupTimeout),
	}

	c, err := testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true},
	)
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatalf("run keto migrate: %v", err)
	}

	state, err := c.State(ctx)
	if err != nil {
		t.Fatalf("keto migrate state: %v", err)
	}
	if state.ExitCode != 0 {
		t.Fatalf("keto migrate exited %d, want 0", state.ExitCode)
	}
}

func startKeto(ctx context.Context, t *testing.T, networkName string) (string, string) {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:          ketoImage,
		Env:            map[string]string{"DSN": ketoDSN},
		ExposedPorts:   []string{"4466/tcp", "4467/tcp"},
		Networks:       []string{networkName},
		NetworkAliases: map[string][]string{networkName: {"keto"}},
		Files:          ketoConfigFiles(t),
		Cmd:            []string{"serve", "-c", "/etc/config/keto/keto.yml"},
		WaitingFor: wait.ForHTTP("/health/ready").
			WithPort("4466/tcp").
			WithStartupTimeout(containerStartupTimeout),
	}

	c, err := testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true},
	)
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatalf("start keto: %v", err)
	}

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("keto host: %v", err)
	}

	readPort, err := c.MappedPort(ctx, "4466/tcp")
	if err != nil {
		t.Fatalf("keto read mapped port: %v", err)
	}

	writePort, err := c.MappedPort(ctx, "4467/tcp")
	if err != nil {
		t.Fatalf("keto write mapped port: %v", err)
	}

	return fmt.Sprintf("http://%s", net.JoinHostPort(host, readPort.Port())),
		fmt.Sprintf("http://%s", net.JoinHostPort(host, writePort.Port()))
}

// ketoConfigFiles returns the same keto.yml/identities.ts this repo's local
// dev docker-compose.keto.yml mounts, so the Keto configuration itself has
// exactly one source of truth.
func ketoConfigFiles(t *testing.T) []testcontainers.ContainerFile {
	t.Helper()

	// This file lives at internal/clients/ketoclient/, three levels below
	// the repo root.
	root := filepath.Join("..", "..", "..")

	return []testcontainers.ContainerFile{
		{
			HostFilePath:      filepath.Join(root, "deploy", "keto", "keto.yml"),
			ContainerFilePath: "/etc/config/keto/keto.yml",
			FileMode:          0o644,
		},
		{
			HostFilePath:      filepath.Join(root, "deploy", "keto", "identities.ts"),
			ContainerFilePath: "/etc/config/keto/identities.ts",
			FileMode:          0o644,
		},
	}
}
