package web_test

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	kratos "github.com/ory/kratos-client-go/v26"

	"github.com/DanielKirkwood/unwrap-gift/internal/web"
)

// fakeLoginFlowProvider is a minimal web.LoginFlowProvider for testing the
// login handler without a real Kratos instance.
type fakeLoginFlowProvider struct {
	flow *kratos.LoginFlow
	err  error
}

func (f fakeLoginFlowProvider) GetLoginFlow(context.Context, string, string) (*kratos.LoginFlow, error) {
	return f.flow, f.err
}

func mustParseTemplates(t *testing.T) *template.Template {
	t.Helper()
	tmpl, err := web.ParseTemplates()
	if err != nil {
		t.Fatalf("ParseTemplates() error = %v, want nil", err)
	}
	return tmpl
}

func mountTestLogin(flows web.LoginFlowProvider, t *testing.T) http.Handler {
	r := chi.NewRouter()
	web.MountLogin(r, flows, testKratosBrowserURL, mustParseTemplates(t))
	return r
}

func TestMountLogin_NoFlow_RedirectsToKratosBrowser(t *testing.T) {
	t.Parallel()

	router := mountTestLogin(fakeLoginFlowProvider{}, t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login", nil))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, testKratosBrowserURL+"/self-service/login/browser?return_to=") {
		t.Errorf("Location = %q, want a self-service/login/browser redirect", loc)
	}
}

// inputNode builds a kratos.UiNode with UiNodeInputAttributes set — the
// only node shape this package's rendering cares about.
func inputNode(name, nodeType string, required bool) kratos.UiNode {
	return kratos.UiNode{
		Type: "input",
		Attributes: kratos.UiNodeAttributes{
			UiNodeInputAttributes: &kratos.UiNodeInputAttributes{
				Name: name, Type: nodeType, Required: &required,
			},
		},
		Meta: kratos.UiNodeMeta{Label: &kratos.UiText{Id: 1, Text: name, Type: "info"}},
	}
}

func TestMountLogin_FlowPresent_RendersIdentifierStep(t *testing.T) {
	t.Parallel()

	flow := &kratos.LoginFlow{
		Id:        "flow-1",
		ExpiresAt: time.Now().Add(time.Hour),
		Ui: kratos.UiContainer{
			Action: "http://127.0.0.1:4433/self-service/login?flow=flow-1",
			Method: "POST",
			Nodes: []kratos.UiNode{
				inputNode("csrf_token", "hidden", true),
				inputNode("identifier", "text", true),
				inputNode("method", "submit", false),
			},
		},
	}
	router := mountTestLogin(fakeLoginFlowProvider{flow: flow}, t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login?flow=flow-1", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="csrf_token"`) || !strings.Contains(body, `type="hidden"`) {
		t.Error("expected a hidden csrf_token input in the rendered body")
	}
	if !strings.Contains(body, `name="identifier"`) || !strings.Contains(body, "wa-input") {
		t.Error("expected a wa-input identifier field in the rendered body")
	}
	if !strings.Contains(body, "wa-button") {
		t.Error("expected a wa-button for the submit node")
	}
}

func TestMountLogin_FlowPresent_CodeNodeMapsToOtpInput(t *testing.T) {
	t.Parallel()

	flow := &kratos.LoginFlow{
		Id: "flow-2", ExpiresAt: time.Now().Add(time.Hour),
		Ui: kratos.UiContainer{
			Action: "http://127.0.0.1:4433/self-service/login?flow=flow-2",
			Method: "POST",
			Nodes: []kratos.UiNode{
				inputNode("csrf_token", "hidden", true),
				inputNode("identifier", "hidden", true),
				inputNode("code", "text", true),
				inputNode("method", "submit", false),
			},
		},
	}
	router := mountTestLogin(fakeLoginFlowProvider{flow: flow}, t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login?flow=flow-2", nil))

	body := rec.Body.String()
	if !strings.Contains(body, "wa-otp-input") {
		t.Error("expected a wa-otp-input for the code field")
	}
	// identifier is hidden (resubmitted) at this step — must render as a
	// plain hidden input, not another visible wa-input.
	if strings.Contains(body, `name="identifier"`) {
		idx := strings.Index(body, `name="identifier"`)
		surrounding := body[max(0, idx-60):idx]
		if !strings.Contains(surrounding, `type="hidden"`) {
			t.Errorf("expected identifier to render as a hidden input in step 2, got context: %q", surrounding)
		}
	}
}

func TestMountLogin_FlowExpired_RedirectsFresh(t *testing.T) {
	t.Parallel()

	router := mountTestLogin(fakeLoginFlowProvider{err: errors.New("flow expired")}, t)

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/login?flow=expired", nil))

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("Location = %q, want /login (fresh start, no flow param)", loc)
	}
}
