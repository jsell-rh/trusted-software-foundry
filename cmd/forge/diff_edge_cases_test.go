package main

// diff_edge_cases_test.go covers the "nil/nil", "added", "removed", and
// "backend/strategy changed" branches in the diffXxx helper functions that are
// not exercised by TestDiffSpecs_AdvancedBlocks. Each test calls the unexported
// helpers directly to isolate exactly which branch is executed.

import (
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// --------------------------------------------------------------------------
// diffTenancy
// --------------------------------------------------------------------------

func TestDiffTenancy_BothNil(t *testing.T) {
	diffs := diffTenancy(nil, nil)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs when both nil, got: %v", diffs)
	}
}

func TestDiffTenancy_Added(t *testing.T) {
	diffs := diffTenancy(nil, &spec.IRTenancyConfig{Strategy: "row", Field: "org_id"})
	if len(diffs) == 0 {
		t.Fatal("expected diffs when tenancy added, got none")
	}
	if !strings.Contains(diffs[0], "+ tenancy") {
		t.Errorf("expected '+ tenancy' in diff, got: %v", diffs)
	}
}

func TestDiffTenancy_Removed(t *testing.T) {
	diffs := diffTenancy(&spec.IRTenancyConfig{Strategy: "row", Field: "org_id"}, nil)
	if len(diffs) == 0 {
		t.Fatal("expected diffs when tenancy removed, got none")
	}
	if !strings.Contains(diffs[0], "- tenancy") {
		t.Errorf("expected '- tenancy' in diff, got: %v", diffs)
	}
}

func TestDiffTenancy_StrategyChanged(t *testing.T) {
	old := &spec.IRTenancyConfig{Strategy: "row", Field: "org_id"}
	new := &spec.IRTenancyConfig{Strategy: "schema", Field: "org_id"}
	diffs := diffTenancy(old, new)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "strategy") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected strategy change diff, got: %v", diffs)
	}
}

// --------------------------------------------------------------------------
// diffAuthz
// --------------------------------------------------------------------------

func TestDiffAuthz_BothNil(t *testing.T) {
	diffs := diffAuthz(nil, nil)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs when both nil, got: %v", diffs)
	}
}

func TestDiffAuthz_Added(t *testing.T) {
	diffs := diffAuthz(nil, &spec.IRAuthzConfig{Backend: "spicedb"})
	if len(diffs) == 0 || !strings.Contains(diffs[0], "+ authz") {
		t.Errorf("expected '+ authz' diff, got: %v", diffs)
	}
}

func TestDiffAuthz_Removed(t *testing.T) {
	diffs := diffAuthz(&spec.IRAuthzConfig{Backend: "spicedb"}, nil)
	if len(diffs) == 0 || !strings.Contains(diffs[0], "- authz") {
		t.Errorf("expected '- authz' diff, got: %v", diffs)
	}
}

func TestDiffAuthz_BackendChanged(t *testing.T) {
	old := &spec.IRAuthzConfig{Backend: "spicedb"}
	new := &spec.IRAuthzConfig{Backend: "opa"}
	diffs := diffAuthz(old, new)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "backend") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected backend change diff, got: %v", diffs)
	}
}

func TestDiffAuthz_RelationRemoved(t *testing.T) {
	rel := spec.IRAuthzRelation{Resource: "Item", Relation: "owner", Subject: "User"}
	old := &spec.IRAuthzConfig{Backend: "spicedb", Relations: []spec.IRAuthzRelation{rel}}
	new := &spec.IRAuthzConfig{Backend: "spicedb", Relations: nil}
	diffs := diffAuthz(old, new)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "- authz.relation") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected '- authz.relation' diff for removed relation, got: %v", diffs)
	}
}

// --------------------------------------------------------------------------
// diffGraph
// --------------------------------------------------------------------------

func TestDiffGraph_BothNil(t *testing.T) {
	diffs := diffGraph(nil, nil)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs when both nil, got: %v", diffs)
	}
}

func TestDiffGraph_Added(t *testing.T) {
	diffs := diffGraph(nil, &spec.IRGraphConfig{Backend: "age"})
	if len(diffs) == 0 || !strings.Contains(diffs[0], "+ graph") {
		t.Errorf("expected '+ graph' diff, got: %v", diffs)
	}
}

func TestDiffGraph_Removed(t *testing.T) {
	diffs := diffGraph(&spec.IRGraphConfig{Backend: "age"}, nil)
	if len(diffs) == 0 || !strings.Contains(diffs[0], "- graph") {
		t.Errorf("expected '- graph' diff, got: %v", diffs)
	}
}

func TestDiffGraph_BackendChanged(t *testing.T) {
	old := &spec.IRGraphConfig{Backend: "age"}
	new := &spec.IRGraphConfig{Backend: "neo4j"}
	diffs := diffGraph(old, new)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "backend") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected backend change diff, got: %v", diffs)
	}
}

func TestDiffGraph_NodeRemoved(t *testing.T) {
	node := spec.IRGraphNodeType{Label: "Cluster"}
	old := &spec.IRGraphConfig{Backend: "age", NodeTypes: []spec.IRGraphNodeType{node}}
	new := &spec.IRGraphConfig{Backend: "age", NodeTypes: nil}
	diffs := diffGraph(old, new)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "- graph.node") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected '- graph.node' diff, got: %v", diffs)
	}
}

func TestDiffGraph_NodeAdded(t *testing.T) {
	node := spec.IRGraphNodeType{Label: "NodePool"}
	old := &spec.IRGraphConfig{Backend: "age", NodeTypes: nil}
	new := &spec.IRGraphConfig{Backend: "age", NodeTypes: []spec.IRGraphNodeType{node}}
	diffs := diffGraph(old, new)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "+ graph.node") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected '+ graph.node' diff, got: %v", diffs)
	}
}

// --------------------------------------------------------------------------
// diffServices
// --------------------------------------------------------------------------

func TestDiffServices_ServiceRemoved(t *testing.T) {
	old := []spec.IRService{{Name: "api", Role: "gateway", Port: 8080}}
	new := []spec.IRService{}
	diffs := diffServices(old, new)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "- service") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected '- service' diff, got: %v", diffs)
	}
}

func TestDiffServices_RoleChanged(t *testing.T) {
	old := []spec.IRService{{Name: "api", Role: "gateway", Port: 8080}}
	new := []spec.IRService{{Name: "api", Role: "worker", Port: 8080}}
	diffs := diffServices(old, new)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "role") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected role change diff, got: %v", diffs)
	}
}

func TestDiffServices_PortChanged(t *testing.T) {
	old := []spec.IRService{{Name: "api", Role: "gateway", Port: 8080}}
	new := []spec.IRService{{Name: "api", Role: "gateway", Port: 9090}}
	diffs := diffServices(old, new)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "port") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected port change diff, got: %v", diffs)
	}
}

// --------------------------------------------------------------------------
// diffEvents
// --------------------------------------------------------------------------

func TestDiffEvents_BothNil(t *testing.T) {
	diffs := diffEvents(nil, nil)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs when both nil, got: %v", diffs)
	}
}

func TestDiffEvents_Added(t *testing.T) {
	diffs := diffEvents(nil, &spec.IREventsConfig{Backend: "kafka"})
	if len(diffs) == 0 || !strings.Contains(diffs[0], "+ events") {
		t.Errorf("expected '+ events' diff, got: %v", diffs)
	}
}

func TestDiffEvents_Removed(t *testing.T) {
	diffs := diffEvents(&spec.IREventsConfig{Backend: "kafka"}, nil)
	if len(diffs) == 0 || !strings.Contains(diffs[0], "- events") {
		t.Errorf("expected '- events' diff, got: %v", diffs)
	}
}

func TestDiffEvents_TopicRemoved(t *testing.T) {
	topic := spec.IREventTopic{Name: "app.items", Partitions: 3}
	old := &spec.IREventsConfig{Backend: "kafka", Topics: []spec.IREventTopic{topic}}
	new := &spec.IREventsConfig{Backend: "kafka", Topics: nil}
	diffs := diffEvents(old, new)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "- events.topic") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected '- events.topic' diff, got: %v", diffs)
	}
}

// --------------------------------------------------------------------------
// diffState
// --------------------------------------------------------------------------

func TestDiffState_BothNil(t *testing.T) {
	diffs := diffState(nil, nil)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs when both nil, got: %v", diffs)
	}
}

func TestDiffState_Added(t *testing.T) {
	diffs := diffState(nil, &spec.IRStateConfig{Backend: "redis"})
	if len(diffs) == 0 || !strings.Contains(diffs[0], "+ state") {
		t.Errorf("expected '+ state' diff, got: %v", diffs)
	}
}

func TestDiffState_Removed(t *testing.T) {
	diffs := diffState(&spec.IRStateConfig{Backend: "redis"}, nil)
	if len(diffs) == 0 || !strings.Contains(diffs[0], "- state") {
		t.Errorf("expected '- state' diff, got: %v", diffs)
	}
}

func TestDiffState_KeyRemoved(t *testing.T) {
	key := spec.IRStateKey{Name: "item_lock", Strategy: "distributed_lock"}
	old := &spec.IRStateConfig{Backend: "redis", Keys: []spec.IRStateKey{key}}
	new := &spec.IRStateConfig{Backend: "redis", Keys: nil}
	diffs := diffState(old, new)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "- state.key") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected '- state.key' diff, got: %v", diffs)
	}
}

// --------------------------------------------------------------------------
// diffWorkflows
// --------------------------------------------------------------------------

func TestDiffWorkflows_BothNil(t *testing.T) {
	diffs := diffWorkflows(nil, nil)
	if len(diffs) != 0 {
		t.Errorf("expected no diffs when both nil, got: %v", diffs)
	}
}

func TestDiffWorkflows_Added(t *testing.T) {
	diffs := diffWorkflows(nil, &spec.IRWorkflowsConfig{Namespace: "my-ns"})
	if len(diffs) == 0 || !strings.Contains(diffs[0], "+ workflows") {
		t.Errorf("expected '+ workflows' diff, got: %v", diffs)
	}
}

func TestDiffWorkflows_Removed(t *testing.T) {
	diffs := diffWorkflows(&spec.IRWorkflowsConfig{Namespace: "my-ns"}, nil)
	if len(diffs) == 0 || !strings.Contains(diffs[0], "- workflows") {
		t.Errorf("expected '- workflows' diff, got: %v", diffs)
	}
}

func TestDiffWorkflows_WorkflowRemoved(t *testing.T) {
	wf := spec.IRWorkflowDef{Name: "ProcessItem"}
	old := &spec.IRWorkflowsConfig{Namespace: "ns", Workflows: []spec.IRWorkflowDef{wf}}
	new := &spec.IRWorkflowsConfig{Namespace: "ns", Workflows: nil}
	diffs := diffWorkflows(old, new)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "- workflow") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected '- workflow' diff, got: %v", diffs)
	}
}

// --------------------------------------------------------------------------
// diffHooks
// --------------------------------------------------------------------------

func TestDiffHooks_HookRemoved(t *testing.T) {
	old := []spec.IRHook{{Name: "audit", Point: "pre-db"}}
	new := []spec.IRHook{}
	diffs := diffHooks(old, new)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "- hook") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected '- hook' diff, got: %v", diffs)
	}
}

func TestDiffHooks_PointChanged(t *testing.T) {
	old := []spec.IRHook{{Name: "audit", Point: "pre-db"}}
	new := []spec.IRHook{{Name: "audit", Point: "post-db"}}
	diffs := diffHooks(old, new)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "point") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected hook point change diff, got: %v", diffs)
	}
}

// --------------------------------------------------------------------------
// diffResource
// --------------------------------------------------------------------------

func TestDiffResource_FieldRemoved(t *testing.T) {
	idField := spec.IRField{Name: "id", Type: "uuid"}
	extraField := spec.IRField{Name: "notes", Type: "string"}
	old := spec.IRResource{Fields: []spec.IRField{idField, extraField}, Operations: []string{"create"}}
	new := spec.IRResource{Fields: []spec.IRField{idField}, Operations: []string{"create"}}
	diffs := diffResource("Item", old, new)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "- resource") && strings.Contains(d, "notes") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected field removal diff, got: %v", diffs)
	}
}

func TestDiffResource_FieldTypeChanged(t *testing.T) {
	old := spec.IRResource{Fields: []spec.IRField{{Name: "score", Type: "int"}}, Operations: []string{"create"}}
	new := spec.IRResource{Fields: []spec.IRField{{Name: "score", Type: "float"}}, Operations: []string{"create"}}
	diffs := diffResource("Item", old, new)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "type") && strings.Contains(d, "score") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected field type change diff, got: %v", diffs)
	}
}

func TestDiffResource_OperationsChanged(t *testing.T) {
	old := spec.IRResource{Operations: []string{"create"}}
	new := spec.IRResource{Operations: []string{"create", "delete"}}
	diffs := diffResource("Item", old, new)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "operations") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected operations change diff, got: %v", diffs)
	}
}

func TestDiffResource_EventsChanged(t *testing.T) {
	old := spec.IRResource{Events: false, Operations: []string{"create"}}
	new := spec.IRResource{Events: true, Operations: []string{"create"}}
	diffs := diffResource("Item", old, new)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "events") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected events change diff, got: %v", diffs)
	}
}

// --------------------------------------------------------------------------
// diffSpecs — component removed/upgraded
// --------------------------------------------------------------------------

func TestDiffSpecs_ComponentRemoved(t *testing.T) {
	oldIR := &spec.IRSpec{
		Metadata:   spec.IRMetadata{Name: "app", Version: "1.0.0"},
		Components: map[string]string{"foundry-http": "v1.0.0", "foundry-postgres": "v1.0.0"},
	}
	newIR := &spec.IRSpec{
		Metadata:   spec.IRMetadata{Name: "app", Version: "1.0.0"},
		Components: map[string]string{"foundry-http": "v1.0.0"},
	}
	diffs := diffSpecs(oldIR, newIR)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "- component") && strings.Contains(d, "foundry-postgres") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected component removal diff, got: %v", diffs)
	}
}

func TestDiffSpecs_ComponentUpgraded(t *testing.T) {
	oldIR := &spec.IRSpec{
		Metadata:   spec.IRMetadata{Name: "app", Version: "1.0.0"},
		Components: map[string]string{"foundry-http": "v1.0.0"},
	}
	newIR := &spec.IRSpec{
		Metadata:   spec.IRMetadata{Name: "app", Version: "1.0.0"},
		Components: map[string]string{"foundry-http": "v2.0.0"},
	}
	diffs := diffSpecs(oldIR, newIR)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "~ component") && strings.Contains(d, "foundry-http") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected component upgrade diff, got: %v", diffs)
	}
}

func TestDiffSpecs_ResourceRemoved(t *testing.T) {
	oldIR := &spec.IRSpec{
		Metadata:   spec.IRMetadata{Name: "app", Version: "1.0.0"},
		Components: map[string]string{"foundry-http": "v1.0.0"},
		Resources:  []spec.IRResource{{Name: "Widget"}, {Name: "Gadget"}},
	}
	newIR := &spec.IRSpec{
		Metadata:   spec.IRMetadata{Name: "app", Version: "1.0.0"},
		Components: map[string]string{"foundry-http": "v1.0.0"},
		Resources:  []spec.IRResource{{Name: "Widget"}},
	}
	diffs := diffSpecs(oldIR, newIR)
	found := false
	for _, d := range diffs {
		if strings.Contains(d, "- resource") && strings.Contains(d, "Gadget") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected resource removal diff, got: %v", diffs)
	}
}
