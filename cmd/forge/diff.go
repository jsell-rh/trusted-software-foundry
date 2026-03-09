package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jsell-rh/trusted-software-foundry/foundry/compiler"
	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

func diffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff <old-spec.yaml> <new-spec.yaml>",
		Short: "Show what changed between two Foundry IR specs",
		Long: `diff compares two Foundry IR spec files and reports:
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

	// Tenancy
	diffs = append(diffs, diffTenancy(old.Tenancy, new.Tenancy)...)

	// Authz
	diffs = append(diffs, diffAuthz(old.Authz, new.Authz)...)

	// Graph
	diffs = append(diffs, diffGraph(old.Graph, new.Graph)...)

	// Services
	diffs = append(diffs, diffServices(old.Services, new.Services)...)

	// Events
	diffs = append(diffs, diffEvents(old.Events, new.Events)...)

	// State
	diffs = append(diffs, diffState(old.State, new.State)...)

	// Workflows
	diffs = append(diffs, diffWorkflows(old.Workflows, new.Workflows)...)

	// Hooks
	diffs = append(diffs, diffHooks(old.Hooks, new.Hooks)...)

	sort.Strings(diffs)
	return diffs
}

func diffTenancy(old, new *spec.IRTenancyConfig) []string {
	if old == nil && new == nil {
		return nil
	}
	var diffs []string
	if old == nil {
		diffs = append(diffs, "+ tenancy: added (strategy="+new.Strategy+")")
		return diffs
	}
	if new == nil {
		diffs = append(diffs, "- tenancy: removed")
		return diffs
	}
	if old.Strategy != new.Strategy {
		diffs = append(diffs, fmt.Sprintf("~ tenancy.strategy: %s → %s", old.Strategy, new.Strategy))
	}
	if old.Field != new.Field {
		diffs = append(diffs, fmt.Sprintf("~ tenancy.field: %s → %s", old.Field, new.Field))
	}
	return diffs
}

func diffAuthz(old, new *spec.IRAuthzConfig) []string {
	if old == nil && new == nil {
		return nil
	}
	var diffs []string
	if old == nil {
		diffs = append(diffs, "+ authz: added (backend="+new.Backend+")")
		return diffs
	}
	if new == nil {
		diffs = append(diffs, "- authz: removed")
		return diffs
	}
	if old.Backend != new.Backend {
		diffs = append(diffs, fmt.Sprintf("~ authz.backend: %s → %s", old.Backend, new.Backend))
	}
	// Relations
	oldRels := map[string]bool{}
	for _, r := range old.Relations {
		oldRels[r.Resource+"."+r.Relation+"→"+r.Subject] = true
	}
	for _, r := range new.Relations {
		key := r.Resource + "." + r.Relation + "→" + r.Subject
		if !oldRels[key] {
			diffs = append(diffs, fmt.Sprintf("+ authz.relation: %s.%s → %s", r.Resource, r.Relation, r.Subject))
		}
	}
	newRels := map[string]bool{}
	for _, r := range new.Relations {
		newRels[r.Resource+"."+r.Relation+"→"+r.Subject] = true
	}
	for _, r := range old.Relations {
		key := r.Resource + "." + r.Relation + "→" + r.Subject
		if !newRels[key] {
			diffs = append(diffs, fmt.Sprintf("- authz.relation: %s.%s → %s", r.Resource, r.Relation, r.Subject))
		}
	}
	return diffs
}

func diffGraph(old, new *spec.IRGraphConfig) []string {
	if old == nil && new == nil {
		return nil
	}
	var diffs []string
	if old == nil {
		diffs = append(diffs, "+ graph: added (backend="+new.Backend+")")
		return diffs
	}
	if new == nil {
		diffs = append(diffs, "- graph: removed")
		return diffs
	}
	if old.Backend != new.Backend {
		diffs = append(diffs, fmt.Sprintf("~ graph.backend: %s → %s", old.Backend, new.Backend))
	}
	// Node types
	oldNodes := map[string]bool{}
	for _, n := range old.NodeTypes {
		oldNodes[n.Label] = true
	}
	for _, n := range new.NodeTypes {
		if !oldNodes[n.Label] {
			diffs = append(diffs, fmt.Sprintf("+ graph.node: %s (added)", n.Label))
		}
	}
	newNodes := map[string]bool{}
	for _, n := range new.NodeTypes {
		newNodes[n.Label] = true
	}
	for _, n := range old.NodeTypes {
		if !newNodes[n.Label] {
			diffs = append(diffs, fmt.Sprintf("- graph.node: %s (removed)", n.Label))
		}
	}
	return diffs
}

func diffServices(old, new []spec.IRService) []string {
	var diffs []string
	oldMap := map[string]spec.IRService{}
	for _, s := range old {
		oldMap[s.Name] = s
	}
	newMap := map[string]spec.IRService{}
	for _, s := range new {
		newMap[s.Name] = s
	}
	for name := range oldMap {
		if _, ok := newMap[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("- service: %s (removed)", name))
		}
	}
	for name, s := range newMap {
		if _, ok := oldMap[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("+ service: %s role:%s port:%d (added)", name, s.Role, s.Port))
		}
	}
	for name, os := range oldMap {
		ns, ok := newMap[name]
		if !ok {
			continue
		}
		if os.Role != ns.Role {
			diffs = append(diffs, fmt.Sprintf("~ service %s role: %s → %s", name, os.Role, ns.Role))
		}
		if os.Port != ns.Port {
			diffs = append(diffs, fmt.Sprintf("~ service %s port: %d → %d", name, os.Port, ns.Port))
		}
	}
	return diffs
}

func diffEvents(old, new *spec.IREventsConfig) []string {
	if old == nil && new == nil {
		return nil
	}
	var diffs []string
	if old == nil {
		diffs = append(diffs, "+ events: added (backend="+new.Backend+")")
		return diffs
	}
	if new == nil {
		diffs = append(diffs, "- events: removed")
		return diffs
	}
	oldTopics := map[string]bool{}
	for _, t := range old.Topics {
		oldTopics[t.Name] = true
	}
	for _, t := range new.Topics {
		if !oldTopics[t.Name] {
			diffs = append(diffs, fmt.Sprintf("+ events.topic: %s (added)", t.Name))
		}
	}
	newTopics := map[string]bool{}
	for _, t := range new.Topics {
		newTopics[t.Name] = true
	}
	for _, t := range old.Topics {
		if !newTopics[t.Name] {
			diffs = append(diffs, fmt.Sprintf("- events.topic: %s (removed)", t.Name))
		}
	}
	return diffs
}

func diffState(old, new *spec.IRStateConfig) []string {
	if old == nil && new == nil {
		return nil
	}
	var diffs []string
	if old == nil {
		diffs = append(diffs, "+ state: added (backend="+new.Backend+")")
		return diffs
	}
	if new == nil {
		diffs = append(diffs, "- state: removed")
		return diffs
	}
	oldKeys := map[string]bool{}
	for _, k := range old.Keys {
		oldKeys[k.Name] = true
	}
	for _, k := range new.Keys {
		if !oldKeys[k.Name] {
			diffs = append(diffs, fmt.Sprintf("+ state.key: %s (added, strategy=%s)", k.Name, k.Strategy))
		}
	}
	newKeys := map[string]bool{}
	for _, k := range new.Keys {
		newKeys[k.Name] = true
	}
	for _, k := range old.Keys {
		if !newKeys[k.Name] {
			diffs = append(diffs, fmt.Sprintf("- state.key: %s (removed)", k.Name))
		}
	}
	return diffs
}

func diffWorkflows(old, new *spec.IRWorkflowsConfig) []string {
	if old == nil && new == nil {
		return nil
	}
	var diffs []string
	if old == nil {
		diffs = append(diffs, "+ workflows: added (namespace="+new.Namespace+")")
		return diffs
	}
	if new == nil {
		diffs = append(diffs, "- workflows: removed")
		return diffs
	}
	if old.Namespace != new.Namespace {
		diffs = append(diffs, fmt.Sprintf("~ workflows.namespace: %s → %s", old.Namespace, new.Namespace))
	}
	oldWFs := map[string]bool{}
	for _, w := range old.Workflows {
		oldWFs[w.Name] = true
	}
	for _, w := range new.Workflows {
		if !oldWFs[w.Name] {
			diffs = append(diffs, fmt.Sprintf("+ workflow: %s (added)", w.Name))
		}
	}
	newWFs := map[string]bool{}
	for _, w := range new.Workflows {
		newWFs[w.Name] = true
	}
	for _, w := range old.Workflows {
		if !newWFs[w.Name] {
			diffs = append(diffs, fmt.Sprintf("- workflow: %s (removed)", w.Name))
		}
	}
	return diffs
}

func diffHooks(old, new []spec.IRHook) []string {
	var diffs []string
	oldMap := map[string]spec.IRHook{}
	for _, h := range old {
		oldMap[h.Name] = h
	}
	newMap := map[string]spec.IRHook{}
	for _, h := range new {
		newMap[h.Name] = h
	}
	for name := range oldMap {
		if _, ok := newMap[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("- hook: %s (removed)", name))
		}
	}
	for name, h := range newMap {
		if _, ok := oldMap[name]; !ok {
			diffs = append(diffs, fmt.Sprintf("+ hook: %s point:%s (added)", name, h.Point))
		}
	}
	for name, oh := range oldMap {
		nh, ok := newMap[name]
		if !ok {
			continue
		}
		if oh.Point != nh.Point {
			diffs = append(diffs, fmt.Sprintf("~ hook %s point: %s → %s", name, oh.Point, nh.Point))
		}
	}
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
