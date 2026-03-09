package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jsell-rh/trusted-software-foundry/tsc/compiler"
	"github.com/jsell-rh/trusted-software-foundry/tsc/spec"
)

func diffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <old-spec.yaml> <new-spec.yaml>",
		Short: "Show what changed between two TSC IR specs",
		Long: `diff compares two TSC IR spec files and reports:
  + Added resources or fields
  - Removed resources or fields
  ~ Changed component versions, field types, operations, or config

AI agents use diff to verify their edits before compiling, and to communicate
changes to reviewers in a structured way.
Exit code 0 = no diff (specs are equivalent); exit code 1 = differences found.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			oldPath, newPath := args[0], args[1]

			oldIR, err := compiler.Parse(oldPath)
			if err != nil {
				return fmt.Errorf("parsing old spec %q: %w", oldPath, err)
			}
			newIR, err := compiler.Parse(newPath)
			if err != nil {
				return fmt.Errorf("parsing new spec %q: %w", newPath, err)
			}

			diffs := diffSpecs(oldIR, newIR)
			if len(diffs) == 0 {
				fmt.Println("No differences found — specs are equivalent.")
				return nil
			}

			fmt.Printf("diff %s → %s\n\n", oldPath, newPath)
			for _, d := range diffs {
				fmt.Println(d)
			}
			os.Exit(1) // non-zero = differences exist
			return nil
		},
	}

	return cmd
}

// diffSpecs compares two IRSpec values and returns a list of human-readable diff lines.
func diffSpecs(old, new *spec.IRSpec) []string {
	var diffs []string

	// Metadata
	if old.Metadata.Version != new.Metadata.Version {
		diffs = append(diffs, fmt.Sprintf("~ version: %s → %s", old.Metadata.Version, new.Metadata.Version))
	}

	// Components (SBOM)
	for name, oldVer := range old.Components {
		if newVer, ok := new.Components[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("- component: %s %s (removed)", name, oldVer))
		} else if oldVer != newVer {
			diffs = append(diffs, fmt.Sprintf("~ component: %s %s → %s", name, oldVer, newVer))
		}
	}
	for name, newVer := range new.Components {
		if _, ok := old.Components[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("+ component: %s %s (added)", name, newVer))
		}
	}

	// Resources
	oldResMap := make(map[string]spec.IRResource, len(old.Resources))
	for _, r := range old.Resources {
		oldResMap[r.Name] = r
	}
	newResMap := make(map[string]spec.IRResource, len(new.Resources))
	for _, r := range new.Resources {
		newResMap[r.Name] = r
	}

	// Removed resources
	for name := range oldResMap {
		if _, ok := newResMap[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("- resource: %s (removed)", name))
		}
	}
	// Added resources
	for name, r := range newResMap {
		if _, ok := oldResMap[name]; !ok {
			fieldNames := make([]string, len(r.Fields))
			for i, f := range r.Fields {
				fieldNames[i] = f.Name
			}
			diffs = append(diffs, fmt.Sprintf("+ resource: %s {%s}", name, strings.Join(fieldNames, ", ")))
		}
	}
	// Changed resources
	for name, oldR := range oldResMap {
		newR, ok := newResMap[name]
		if !ok {
			continue
		}
		diffs = append(diffs, diffResource(name, oldR, newR)...)
	}

	sort.Strings(diffs)
	return diffs
}

func diffResource(name string, old, new spec.IRResource) []string {
	var diffs []string

	// Fields
	oldFields := make(map[string]spec.IRField)
	for _, f := range old.Fields {
		oldFields[f.Name] = f
	}
	newFields := make(map[string]spec.IRField)
	for _, f := range new.Fields {
		newFields[f.Name] = f
	}

	for fname, of := range oldFields {
		nf, ok := newFields[fname]
		if !ok {
			diffs = append(diffs, fmt.Sprintf("- resource %s field: %s %s (removed)", name, fname, of.Type))
		} else if of.Type != nf.Type {
			diffs = append(diffs, fmt.Sprintf("~ resource %s field: %s type %s → %s", name, fname, of.Type, nf.Type))
		}
	}
	for fname, nf := range newFields {
		if _, ok := oldFields[fname]; !ok {
			diffs = append(diffs, fmt.Sprintf("+ resource %s field: %s %s (added)", name, fname, nf.Type))
		}
	}

	// Operations
	oldOps := strings.Join(old.Operations, ",")
	newOps := strings.Join(new.Operations, ",")
	if oldOps != newOps {
		diffs = append(diffs, fmt.Sprintf("~ resource %s operations: [%s] → [%s]", name, oldOps, newOps))
	}

	// Events
	if old.Events != new.Events {
		diffs = append(diffs, fmt.Sprintf("~ resource %s events: %v → %v", name, old.Events, new.Events))
	}

	return diffs
}
