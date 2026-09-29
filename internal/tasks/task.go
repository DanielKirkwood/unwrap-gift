package tasks

import (
	"context"
	"fmt"
	"sort"

	keto "github.com/ory/keto-client-go/v26"
)

// RelationTupleWriter is the subset of ketoclient.Client's write API the
// Keto-related tasks in this package need. Declared here rather than
// imported from internal/clients, so this package never imports
// internal/clients or internal/app — per ARCHITECTURE.md's
// dependency-direction rule, only app imports internal/tasks, never the
// reverse. cmd wires the concrete *ketoclient.Client into Deps.Keto when it
// runs a task.
type RelationTupleWriter interface {
	CreateRelationTuple(
		ctx context.Context, namespace, object, relation string, subjectID *string, subjectSet *keto.SubjectSet,
	) error
	DeleteRelationTuple(
		ctx context.Context, namespace, object, relation string, subjectID *string, subjectSet *keto.SubjectSet,
	) error
}

// ArgIdentityID is the RequiredArgs key every Keto-related task in this
// package uses for the target identity's ID.
const ArgIdentityID = "identity-id"

// Deps holds every dependency a Task's Run might need, narrowed to the
// interfaces this package declares above. A field is nil when the backing
// feature is disabled — Run implementations must check before using one.
type Deps struct {
	Keto RelationTupleWriter
}

// Task describes one ad-hoc job runnable via `task exec <name>`.
type Task struct {
	// Name identifies the task: the key it's registered and looked up under,
	// and the value passed to `task exec <name>` on the CLI.
	Name string

	// RequiredArgs lists the keys that must be present in Run's args map.
	RequiredArgs []string

	// Run executes the task against deps, using args for any RequiredArgs
	// values.
	Run func(ctx context.Context, deps Deps, args map[string]string) error
}

//nolint:gochecknoglobals // deliberate: the registry every task's init() registers into; there's no alternative to package-level state for a self-registering pattern.
var registry = make(map[string]Task)

// Register adds t to the registry. Tasks call this from their own init(),
// so importing the tasks package (via a blank import in cmd) is enough to
// make every task available.
func Register(t Task) {
	registry[t.Name] = t
}

// All returns every registered task, sorted by name for stable `task list`
// output.
func All() []Task {
	out := make([]Task, 0, len(registry))
	for _, t := range registry {
		out = append(out, t)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	return out
}

// Get looks up the task registered under name, e.g. for `task exec <name>`
// to resolve before calling Run. ok is false when name was never
// registered — a typo, or its defining file was never blank-imported so its
// init() never ran.
func Get(name string) (Task, bool) {
	t, ok := registry[name]
	return t, ok
}

// CheckRequiredArgs returns an error naming the first RequiredArgs key
// missing from args, or nil if every required key is present.
func (t Task) CheckRequiredArgs(args map[string]string) error {
	for _, key := range t.RequiredArgs {
		if _, ok := args[key]; !ok {
			return fmt.Errorf("tasks: %s: missing required arg %q", t.Name, key)
		}
	}

	return nil
}
