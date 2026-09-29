// Package ketoclient wraps Ory Keto's read (permission checks) and write
// (relation-tuple CRUD) API clients behind the small interface
// internal/api declares (PermissionChecker) — api never imports this
// package directly, per ARCHITECTURE.md's dependency-direction rule.
package ketoclient

import (
	"context"
	"fmt"
	"net/http"

	keto "github.com/ory/keto-client-go/v26"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/DanielKirkwood/unwrap-gift/internal/config"
)

// Client holds the read and write Keto API clients. A nil *Client means the
// keto feature is disabled; app only assigns a *Client into an
// api.PermissionChecker field once it knows the feature is enabled, so no
// code here needs to be nil-receiver-safe (contrast with db.Store, which is
// used unconditionally).
type Client struct {
	// Read is the permission-check client, used by CheckPermission.
	Read *keto.APIClient
	// Write is the relation-tuple CRUD client, used by CreateRelationTuple
	// and DeleteRelationTuple.
	Write *keto.APIClient
}

// New builds a Client from cfg, with both the read and write SDK clients'
// transports instrumented via otelhttp. It returns (nil, nil) when enabled
// is false, mirroring kratosclient.New/db.New/otelclient.New's
// disabled-is-nil convention.
//
//nolint:nilnil // deliberate: nil is the "keto feature disabled" state, not an error.
func New(cfg config.KetoConfig, enabled bool) (*Client, error) {
	if !enabled {
		return nil, nil
	}

	return &Client{
		Read:  newAPIClient(cfg.ReadURL),
		Write: newAPIClient(cfg.WriteURL),
	}, nil
}

func newAPIClient(url string) *keto.APIClient {
	cfg := keto.NewConfiguration()
	cfg.Servers = keto.ServerConfigurations{{URL: url}}
	cfg.HTTPClient = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport)}

	return keto.NewAPIClient(cfg)
}

// CheckPermission implements api.PermissionChecker, reporting whether
// subjectID holds relation on the namespace/object pair.
func (c *Client) CheckPermission(ctx context.Context, namespace, object, relation, subjectID string) (bool, error) {
	result, resp, err := c.Read.PermissionAPI.CheckPermission(ctx).
		Namespace(namespace).
		Object(object).
		Relation(relation).
		SubjectId(subjectID).
		Execute()
	closeBody(resp)

	if err != nil {
		return false, fmt.Errorf("ketoclient: check permission: %w", err)
	}

	return result.Allowed, nil
}

// CreateRelationTuple creates a relation tuple namespace:object#relation@subject,
// where subject is either a bare subjectID or a subjectSet — exactly one of
// the two must be non-nil, mirroring Keto's own CreateRelationshipBody
// contract (SubjectId and SubjectSet are mutually exclusive).
func (c *Client) CreateRelationTuple(
	ctx context.Context,
	namespace, object, relation string,
	subjectID *string,
	subjectSet *keto.SubjectSet,
) error {
	body := keto.NewCreateRelationshipBody()
	body.Namespace = &namespace
	body.Object = &object
	body.Relation = &relation
	body.SubjectId = subjectID
	body.SubjectSet = subjectSet

	_, resp, err := c.Write.RelationshipAPI.CreateRelationship(ctx).CreateRelationshipBody(*body).Execute()
	closeBody(resp)

	if err != nil {
		return fmt.Errorf("ketoclient: create relation tuple: %w", err)
	}

	return nil
}

// DeleteRelationTuple deletes the relation tuple(s) matching
// namespace:object#relation@subject, where subject is either a bare
// subjectID or a subjectSet — exactly one of the two must be non-nil,
// mirroring CreateRelationTuple's contract. Unlike CreateRelationTuple,
// Keto's delete endpoint takes the tuple as query parameters, not a body.
func (c *Client) DeleteRelationTuple(
	ctx context.Context,
	namespace, object, relation string,
	subjectID *string,
	subjectSet *keto.SubjectSet,
) error {
	req := c.Write.RelationshipAPI.DeleteRelationships(ctx).
		Namespace(namespace).
		Object(object).
		Relation(relation)

	if subjectID != nil {
		req = req.SubjectId(*subjectID)
	}
	if subjectSet != nil {
		req = req.SubjectSetNamespace(subjectSet.Namespace).
			SubjectSetObject(subjectSet.Object).
			SubjectSetRelation(subjectSet.Relation)
	}

	resp, err := req.Execute()
	closeBody(resp)

	if err != nil {
		return fmt.Errorf("ketoclient: delete relation tuple: %w", err)
	}

	return nil
}

// closeBody closes resp's body, if resp is non-nil. Every SDK method here
// returns a [http.Response] alongside its error, and the response body must
// be closed either way to avoid leaking the underlying connection.
func closeBody(resp *http.Response) {
	if resp != nil {
		_ = resp.Body.Close()
	}
}
