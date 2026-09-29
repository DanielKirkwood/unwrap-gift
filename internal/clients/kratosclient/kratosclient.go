// Package kratosclient wraps Ory Kratos's public (session) and admin
// (identity) SDK clients behind the small interfaces internal/api declares
// (SessionValidator, IdentityAdmin) — api never imports this package
// directly, per ARCHITECTURE.md's dependency-direction rule.
package kratosclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	kratos "github.com/ory/kratos-client-go/v26"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/DanielKirkwood/unwrap-gift/internal/config"
)

// identitySchemaID is the identity JSON schema every identity this client
// creates or updates is validated against. It must match the schema id
// configured in deploy/kratos/kratos.yml.
const identitySchemaID = "default"

// ErrIdentityNotFound is returned by Client's identity methods when Kratos
// reports no identity exists for the given lookup. app maps it to a 404
// Problem via the Adapter it builds for the hidden router.
var ErrIdentityNotFound = errors.New("kratosclient: identity not found")

// Client holds the public and admin Kratos API clients. A nil *Client means
// the kratos feature is disabled; app only assigns a *Client into an
// api.SessionValidator/api.IdentityAdmin field once it knows the feature is
// enabled, so no code here needs to be nil-receiver-safe (contrast with
// db.Store, which is used unconditionally).
type Client struct {
	// Public is the session-validation and self-service-flow client, used
	// by ToSession.
	Public *kratos.APIClient
	// Admin is the identity-CRUD client, used by the
	// Create/List/Get/Update/DeleteIdentity methods.
	Admin *kratos.APIClient
}

// New builds a Client from cfg, with both the public and admin SDK clients'
// transports instrumented via otelhttp. It returns (nil, nil) when enabled
// is false, mirroring db.New/otelclient.New's disabled-is-nil convention.
//
//nolint:nilnil // deliberate: nil is the "kratos feature disabled" state, not an error.
func New(cfg config.KratosConfig, enabled bool) (*Client, error) {
	if !enabled {
		return nil, nil
	}

	return &Client{
		Public: newAPIClient(cfg.PublicURL),
		Admin:  newAPIClient(cfg.AdminURL),
	}, nil
}

func newAPIClient(url string) *kratos.APIClient {
	cfg := kratos.NewConfiguration()
	cfg.Servers = kratos.ServerConfigurations{{URL: url}}
	cfg.HTTPClient = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

	return kratos.NewAPIClient(cfg)
}

// ToSession implements api.SessionValidator by exchanging cookieHeader (the
// incoming request's raw Cookie header) for the session it identifies. An
// invalid or expired session comes back from Kratos as a non-2xx response,
// which is wrapped and returned as a plain error — unlike the identity
// methods below, there is no sentinel for "no session", since callers only
// need to know that validation failed.
func (c *Client) ToSession(ctx context.Context, cookieHeader string) (*kratos.Session, error) {
	session, resp, err := c.Public.FrontendAPI.ToSession(ctx).Cookie(cookieHeader).Execute()
	closeBody(resp)

	if err != nil {
		return nil, fmt.Errorf("kratosclient: to session: %w", err)
	}

	return session, nil
}

// CreateIdentity implements api.IdentityAdmin. password may be empty, in
// which case the identity is created with no password credential.
func (c *Client) CreateIdentity(ctx context.Context, traits map[string]any, password string) (*kratos.Identity, error) {
	body := kratos.NewCreateIdentityBody(identitySchemaID, traits)
	if password != "" {
		body.Credentials = &kratos.IdentityWithCredentials{
			Password: &kratos.IdentityWithCredentialsPassword{
				Config: &kratos.IdentityWithCredentialsPasswordConfig{Password: &password},
			},
		}
	}

	identity, resp, err := c.Admin.IdentityAPI.CreateIdentity(ctx).CreateIdentityBody(*body).Execute()
	closeBody(resp)

	if err != nil {
		return nil, mapIdentityErr("create identity", resp, err)
	}

	return identity, nil
}

// ListIdentities implements api.IdentityAdmin. It takes no pagination or
// filter parameters, so it returns exactly what Kratos's admin identities
// endpoint gives back for an unparameterized request — there is no way for
// a caller to page through additional results.
func (c *Client) ListIdentities(ctx context.Context) ([]kratos.Identity, error) {
	identities, resp, err := c.Admin.IdentityAPI.ListIdentities(ctx).Execute()
	closeBody(resp)

	if err != nil {
		return nil, mapIdentityErr("list identities", resp, err)
	}

	return identities, nil
}

// GetIdentity implements api.IdentityAdmin. It returns ErrIdentityNotFound,
// rather than the underlying Kratos error, when id does not match any
// identity (a 404 response).
func (c *Client) GetIdentity(ctx context.Context, id string) (*kratos.Identity, error) {
	identity, resp, err := c.Admin.IdentityAPI.GetIdentity(ctx, id).Execute()
	closeBody(resp)

	if err != nil {
		return nil, mapIdentityErr("get identity", resp, err)
	}

	return identity, nil
}

// UpdateIdentity implements api.IdentityAdmin, replacing the identity's
// traits and state. It returns ErrIdentityNotFound, rather than the
// underlying Kratos error, when id does not match any identity (a 404
// response).
func (c *Client) UpdateIdentity(
	ctx context.Context,
	id, state string,
	traits map[string]any,
) (*kratos.Identity, error) {
	body := kratos.NewUpdateIdentityBody(identitySchemaID, state, traits)

	identity, resp, err := c.Admin.IdentityAPI.UpdateIdentity(ctx, id).UpdateIdentityBody(*body).Execute()
	closeBody(resp)

	if err != nil {
		return nil, mapIdentityErr("update identity", resp, err)
	}

	return identity, nil
}

// DeleteIdentity implements api.IdentityAdmin. It returns
// ErrIdentityNotFound, rather than the underlying Kratos error, when id does
// not match any identity (a 404 response).
func (c *Client) DeleteIdentity(ctx context.Context, id string) error {
	resp, err := c.Admin.IdentityAPI.DeleteIdentity(ctx, id).Execute()
	closeBody(resp)

	if err != nil {
		return mapIdentityErr("delete identity", resp, err)
	}

	return nil
}

// mapIdentityErr translates a 404 response from Kratos's admin identity API
// into ErrIdentityNotFound, and wraps every other error with op for context.
func mapIdentityErr(op string, resp *http.Response, err error) error {
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		return ErrIdentityNotFound
	}

	return fmt.Errorf("kratosclient: %s: %w", op, err)
}

// closeBody closes resp's body, if resp is non-nil. Every SDK method here
// returns a [http.Response] alongside its error, and the response body must
// be closed either way to avoid leaking the underlying connection.
func closeBody(resp *http.Response) {
	if resp != nil {
		_ = resp.Body.Close()
	}
}
