package config

import (
	"reflect"
	"regexp"
)

var secretPattern = regexp.MustCompile(`(?i)secret|token|key|password`)

// Field describes one resolved EnvVars field, for diagnostic output (e.g.
// the `info env` CLI command).
type Field struct {
	Name   string // struct field name
	Env    string // environment variable name
	Value  string
	Secret bool // true if the field name suggests a credential
}

// Fields returns every bound field with its resolved value, in struct
// declaration order.
func (e EnvVars) Fields() []Field {
	v := reflect.ValueOf(e)

	fields := make([]Field, 0, v.NumField())
	for sf, fv := range v.Fields() {
		envName := sf.Tag.Get("env")
		if envName == "" {
			continue
		}
		fields = append(fields, Field{
			Name:   sf.Name,
			Env:    envName,
			Value:  fv.String(),
			Secret: secretPattern.MatchString(sf.Name) || secretPattern.MatchString(envName),
		})
	}
	return fields
}

// envValuesByFieldName indexes Fields() by struct field name, for the
// registry's RequiredEnv lookups.
func envValuesByFieldName(e EnvVars) map[string]string {
	values := make(map[string]string, len(e.Fields()))
	for _, f := range e.Fields() {
		values[f.Name] = f.Value
	}
	return values
}
