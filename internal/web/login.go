package web

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	kratos "github.com/ory/kratos-client-go/v26"
)

// LoginFlowProvider fetches a Kratos login flow by ID, implemented by
// internal/clients/kratosclient.Client. web depends on this interface
// instead of that concrete type so it never imports internal/clients, per
// ARCHITECTURE.md's dependency-direction rule.
type LoginFlowProvider interface {
	GetLoginFlow(ctx context.Context, cookieHeader, flowID string) (*kratos.LoginFlow, error)
}

// loginField is a flattened, template-friendly view of one kratos.UiNode's
// input attributes. Flattening in Go (rather than having the template
// type-assert into kratos's "oneOf" UiNodeAttributes union) keeps the
// template a pure rendering concern.
type loginField struct {
	Name, Type, Value, Label string
	Hidden, Required         bool
}

// buildLoginFields flattens a login flow's ui.nodes into loginFields,
// skipping any non-input node (none expected for the code method, but
// kratos.UiNodeAttributes can represent other node kinds too). This is
// deliberately generic rather than hardcoded per login step: the same
// flattening renders step 1 (an "identifier" field), step 2 (a "code"
// field plus the resubmitted, now-hidden "identifier"), and any
// Kratos-added node (e.g. a resend button) without new code.
func buildLoginFields(nodes []kratos.UiNode) []loginField {
	fields := make([]loginField, 0, len(nodes))

	for _, node := range nodes {
		attrs := node.Attributes.UiNodeInputAttributes
		if attrs == nil {
			continue
		}

		f := loginField{Name: attrs.Name, Type: attrs.Type, Hidden: attrs.Type == "hidden"}
		if attrs.Value != nil {
			f.Value = fmt.Sprint(attrs.Value)
		}
		if attrs.Required != nil {
			f.Required = *attrs.Required
		}
		if node.Meta.Label != nil {
			f.Label = node.Meta.Label.Text
		}

		fields = append(fields, f)
	}

	return fields
}

// loginPageData is login.html's template data.
type loginPageData struct {
	Fields   []loginField
	Action   string
	Method   string
	Messages []kratos.UiText
}

// MountLogin adds GET /login to r. With no ?flow= query param, it redirects
// the browser to Kratos's own browser-flow-init endpoint (kratosBrowserURL
// — the browser-reachable Kratos public URL, never the server-internal one
// in production); Kratos sets its CSRF cookie and redirects back here with
// ?flow=<id>. With a ?flow= present, it fetches that flow and renders
// login.html from its current ui.nodes — the same handler serves every
// step of the code method.
func MountLogin(r chi.Router, flows LoginFlowProvider, kratosBrowserURL string, templates *template.Template) {
	r.Get("/login", func(w http.ResponseWriter, r *http.Request) {
		flowID := r.URL.Query().Get("flow")
		if flowID == "" {
			redirectToKratosBrowserFlow(w, r, kratosBrowserURL)
			return
		}

		flow, err := flows.GetLoginFlow(r.Context(), r.Header.Get("Cookie"), flowID)
		if err != nil {
			// Expired/invalid flow — start a fresh one rather than show an
			// error, matching Kratos's own UX convention for this case.
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		data := loginPageData{
			Fields:   buildLoginFields(flow.Ui.Nodes),
			Action:   flow.Ui.Action,
			Method:   flow.Ui.Method,
			Messages: flow.Ui.Messages,
		}
		_ = templates.ExecuteTemplate(w, "login.html", data)
	})
}

// redirectToKratosBrowserFlow 303s the browser to Kratos's browser-flow-init
// endpoint with an absolute return_to pointing back at this app (defaulting
// to /wishlist). Kratos validates return_to against its own
// allowed_return_urls config before honoring it.
func redirectToKratosBrowserFlow(w http.ResponseWriter, r *http.Request, kratosBrowserURL string) {
	returnTo := r.URL.Query().Get("return_to")
	if returnTo == "" {
		returnTo = "/wishlist"
	}

	absoluteReturnTo := (&url.URL{Scheme: schemeOf(r), Host: r.Host, Path: returnTo}).String()
	target := kratosBrowserURL + "/self-service/login/browser?return_to=" + url.QueryEscape(absoluteReturnTo)

	http.Redirect(w, r, target, http.StatusSeeOther)
}

// schemeOf reports r's externally-visible scheme, honoring
// X-Forwarded-Proto (Caddy's reverse_proxy sets this by default in
// production) and falling back to "http" for local, TLS-less dev.
func schemeOf(r *http.Request) string {
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		return proto
	}

	return "http"
}
