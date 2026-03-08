package compiler

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/openshift-online/rh-trex-ai/tsc/spec"
)

// Parse reads, parses, and validates an IR spec file (app.tsc.yaml).
// It delegates structural validation to the canonical validator from tsc/spec,
// which enforces the frozen JSON Schema rules and semantic constraints.
func Parse(path string) (*spec.IRSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading spec file: %w", err)
	}

	var ir spec.IRSpec
	if err := yaml.Unmarshal(data, &ir); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	if errs := spec.Validate(&ir); len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		return nil, fmt.Errorf("spec validation failed:\n  - %s", strings.Join(msgs, "\n  - "))
	}

	return &ir, nil
}
