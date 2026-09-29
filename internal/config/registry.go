package config

import (
	"errors"
	"fmt"
	"strings"
)

// Feature describes one optional (or always-on) subsystem: what it needs to
// be enabled, how to build its config once enabled, and how to validate that
// config before the app starts serving traffic.
type Feature struct {
	Name string

	// RequiredEnv lists EnvVars field names (matching EnvVars struct field
	// names, e.g. "DatabasePath") that must be non-empty for this feature
	// to be enabled.
	RequiredEnv []string

	// Requires lists other feature names that must be enabled for this
	// feature to be enabled.
	Requires []string

	// Build produces this feature's config once it's known to be enabled.
	Build func(EnvVars) any

	// Validate checks the built config for readiness. Nil skips validation.
	Validate func(any) error

	// Config, Enabled, and Reason are output fields, populated by
	// ResolveFeatureEnabledState (Enabled, Reason) and Configure (Config) —
	// both must run first for them to be meaningful. Config holds whatever
	// concrete type Build returned (e.g. DatabaseConfig); callers type-assert
	// it, the way app.Bootstrap does per feature. Reason explains a false
	// Enabled, e.g. "missing required env: DatabasePath".
	Config  any
	Enabled bool
	Reason  string
}

// Registry holds every known feature and resolves/configures/validates them
// together.
type Registry struct {
	order    []string
	features map[string]Feature
}

// NewRegistry returns an empty registry, with nothing registered. Use
// DefaultRegistry for the app's real feature set; NewRegistry exists mainly
// so tests can register a small, custom set of features to exercise
// Registry's resolution logic in isolation.
func NewRegistry() *Registry {
	return &Registry{features: make(map[string]Feature)}
}

// Register adds a feature to the registry, preserving registration order for
// All(). Registering a name that's already present replaces its Feature but
// keeps its original position in that order.
func (r *Registry) Register(f Feature) {
	if _, exists := r.features[f.Name]; !exists {
		r.order = append(r.order, f.Name)
	}
	r.features[f.Name] = f
}

// Feature returns the named feature and whether it exists. Call it after
// ResolveFeatureEnabledState/Configure to read one feature's
// Enabled/Config/Reason by name, as app.Bootstrap and app.BuildServers do
// (e.g. "database", "otel", "kratos", "keto").
func (r *Registry) Feature(name string) (Feature, bool) {
	f, ok := r.features[name]
	return f, ok
}

// All returns every registered feature in registration order, for
// diagnostics that need every feature regardless of name — e.g. the `info`
// command's feature listing (cmd/info.go).
func (r *Registry) All() []Feature {
	out := make([]Feature, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.features[name])
	}
	return out
}

// ResolveFeatureEnabledState determines Enabled/Reason for every feature,
// based on DISABLE_FEATURES, each feature's RequiredEnv, and its Requires
// dependencies. It runs a fixed-point pass so dependency chains resolve
// correctly regardless of registration order.
func (r *Registry) ResolveFeatureEnabledState(env EnvVars) error {
	disabled := disabledFeatureSet(env.DisableFeatures)
	envValues := envValuesByFieldName(env)

	for range r.order {
		if !r.resolvePass(disabled, envValues) {
			break
		}
	}

	r.markUnresolvedAsCyclic()

	return nil
}

// Configure builds Config for every enabled feature by calling its Build
// function. ResolveFeatureEnabledState must be called first.
func (r *Registry) Configure(env EnvVars) error {
	for _, name := range r.order {
		f := r.features[name]
		if !f.Enabled || f.Build == nil {
			continue
		}
		f.Config = f.Build(env)
		r.features[name] = f
	}
	return nil
}

// ValidateReadiness runs each enabled feature's Validate function (if set)
// against its built Config, returning a joined error for every failure.
func (r *Registry) ValidateReadiness() error {
	var errs []error
	for _, name := range r.order {
		f := r.features[name]
		if !f.Enabled || f.Validate == nil {
			continue
		}
		if err := f.Validate(f.Config); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

// resolvePass makes one pass over every unresolved feature, resolving those
// whose dependencies are already known. It reports whether any feature was
// resolved this pass.
func (r *Registry) resolvePass(disabled map[string]bool, envValues map[string]string) bool {
	changed := false

	for _, name := range r.order {
		f := r.features[name]
		if f.Enabled || f.Reason != "" {
			continue // already resolved
		}

		resolved, ok := r.resolveFeature(f, disabled, envValues)
		if !ok {
			continue // a dependency hasn't been resolved yet, retry next pass
		}

		r.features[name] = resolved
		changed = true
	}

	return changed
}

// resolveFeature decides one feature's Enabled/Reason. ok is false only when
// a Requires dependency hasn't been resolved yet.
func (r *Registry) resolveFeature(f Feature, disabled map[string]bool, envValues map[string]string) (Feature, bool) {
	if disabled[strings.ToLower(f.Name)] {
		f.Reason = "disabled via DISABLE_FEATURES"
		return f, true
	}

	if missing := firstMissingEnv(f.RequiredEnv, envValues); missing != "" {
		f.Reason = fmt.Sprintf("missing required env: %s", missing)
		return f, true
	}

	blockedBy, resolved := r.firstUnresolvedRequirement(f.Requires)
	if !resolved {
		return f, false
	}
	if blockedBy != "" {
		f.Reason = fmt.Sprintf("requires disabled feature: %s", blockedBy)
		return f, true
	}

	f.Enabled = true

	return f, true
}

// markUnresolvedAsCyclic labels any feature still unresolved after every
// pass — it sits in a Requires cycle — as disabled.
func (r *Registry) markUnresolvedAsCyclic() {
	for _, name := range r.order {
		f := r.features[name]
		if !f.Enabled && f.Reason == "" {
			f.Reason = "could not resolve requirements (cycle?)"
			r.features[name] = f
		}
	}
}

// firstUnresolvedRequirement reports the first disabled dependency in
// requires, or resolved=false if any dependency hasn't been resolved yet.
func (r *Registry) firstUnresolvedRequirement(requires []string) (string, bool) {
	for _, dep := range requires {
		depFeature, exists := r.features[dep]
		if !exists {
			return dep, true // unknown dependency, treat as unmet
		}
		if !depFeature.Enabled && depFeature.Reason == "" {
			return "", false // dependency not resolved yet
		}
		if !depFeature.Enabled {
			return dep, true
		}
	}

	return "", true
}

func disabledFeatureSet(disableFeatures string) map[string]bool {
	set := make(map[string]bool)
	for name := range strings.SplitSeq(disableFeatures, ",") {
		name = strings.ToLower(strings.TrimSpace(name))
		if name != "" {
			set[name] = true
		}
	}
	return set
}

func firstMissingEnv(requiredEnv []string, values map[string]string) string {
	for _, field := range requiredEnv {
		if values[field] == "" {
			return field
		}
	}
	return ""
}
