package compiler

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	requiredAPIVersion = "tsc/v1"
	requiredKind       = "Application"
)

// Parse reads and validates an IR spec file (app.tsc.yaml).
// It performs structural validation; schema-level validation against JSON Schema
// (from TSC-Architect) is layered on top via ValidateSchema once available.
func Parse(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading spec file %q: %w", path, err)
	}

	var spec Spec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("parsing spec YAML: %w", err)
	}

	if err := validate(&spec); err != nil {
		return nil, fmt.Errorf("invalid spec: %w", err)
	}

	return &spec, nil
}

// validate performs structural validation on a parsed spec.
func validate(spec *Spec) error {
	var errs []string

	if spec.APIVersion != requiredAPIVersion {
		errs = append(errs, fmt.Sprintf("apiVersion must be %q, got %q", requiredAPIVersion, spec.APIVersion))
	}
	if spec.Kind != requiredKind {
		errs = append(errs, fmt.Sprintf("kind must be %q, got %q", requiredKind, spec.Kind))
	}
	if spec.Metadata.Name == "" {
		errs = append(errs, "metadata.name is required")
	}
	if spec.Metadata.Version == "" {
		errs = append(errs, "metadata.version is required")
	}
	if len(spec.Components) == 0 {
		errs = append(errs, "components block is required (defines the SBOM)")
	}
	for name, version := range spec.Components {
		if version == "" {
			errs = append(errs, fmt.Sprintf("component %q has no version pinned", name))
		}
	}
	for i, res := range spec.Resources {
		if res.Name == "" {
			errs = append(errs, fmt.Sprintf("resources[%d].name is required", i))
		}
		for j, f := range res.Fields {
			if f.Name == "" {
				errs = append(errs, fmt.Sprintf("resources[%d].fields[%d].name is required", i, j))
			}
			if f.Type == "" {
				errs = append(errs, fmt.Sprintf("resources[%d].fields[%d].type is required", i, j))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}
