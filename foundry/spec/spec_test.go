package spec

// spec_test.go provides comprehensive coverage for the foundry/spec package:
//   - application.go: NewApplication, AddComponent, Configure, Register, Run,
//     AddHTTPHandler, AddMiddleware, AddGRPCService, SetDB, DB, Resources,
//     HTTPHandlers, Middlewares, GRPCServices
//   - validate.go: Validate (all semantic branches), ToResourceDefinitions, LoadSchemaJSON

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// Stub Component — satisfies the Component interface for unit tests
// --------------------------------------------------------------------------

type stubComponent struct {
	name         string
	configureErr error
	registerErr  error
	startErr     error
	stopErr      error
	// Capture calls for assertions
	configured bool
	registered bool
	started    bool
	stopped    bool
}

func (s *stubComponent) Name() string      { return s.name }
func (s *stubComponent) Version() string   { return "v1.0.0" }
func (s *stubComponent) AuditHash() string { return "abc123" }

func (s *stubComponent) Configure(cfg ComponentConfig) error {
	s.configured = true
	return s.configureErr
}

func (s *stubComponent) Register(app *Application) error {
	s.registered = true
	return s.registerErr
}

func (s *stubComponent) Start(ctx context.Context) error {
	s.started = true
	return s.startErr
}

func (s *stubComponent) Stop(ctx context.Context) error {
	s.stopped = true
	return s.stopErr
}

// --------------------------------------------------------------------------
// Stub DB
// --------------------------------------------------------------------------

type stubDB struct{}

func (d *stubDB) ExecContext(ctx context.Context, query string, args ...any) error { return nil }
func (d *stubDB) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	return nil, nil
}

// --------------------------------------------------------------------------
// Stub HTTPHandler
// --------------------------------------------------------------------------

type stubHandler struct{}

func (h *stubHandler) ServeHTTP(w ResponseWriter, r *Request) {}

// --------------------------------------------------------------------------
// Application — NewApplication and basic accessors
// --------------------------------------------------------------------------

func TestNewApplication_Empty(t *testing.T) {
	app := NewApplication(nil)
	if app == nil {
		t.Fatal("NewApplication returned nil")
	}
	if len(app.Resources()) != 0 {
		t.Errorf("expected 0 resources, got %d", len(app.Resources()))
	}
}

func TestNewApplication_WithResources(t *testing.T) {
	resources := []ResourceDefinition{
		{Name: "Dinosaur", Plural: "dinosaurs"},
		{Name: "Cluster", Plural: "clusters"},
	}
	app := NewApplication(resources)
	got := app.Resources()
	if len(got) != 2 {
		t.Fatalf("expected 2 resources, got %d", len(got))
	}
	if got[0].Name != "Dinosaur" {
		t.Errorf("expected Dinosaur, got %q", got[0].Name)
	}
}

// --------------------------------------------------------------------------
// AddComponent
// --------------------------------------------------------------------------

func TestAddComponent_Single(t *testing.T) {
	app := NewApplication(nil)
	c := &stubComponent{name: "foundry-http"}
	app.AddComponent(c)
	if len(app.components) != 1 {
		t.Errorf("expected 1 component, got %d", len(app.components))
	}
}

func TestAddComponent_Multiple(t *testing.T) {
	app := NewApplication(nil)
	app.AddComponent(&stubComponent{name: "foundry-postgres"})
	app.AddComponent(&stubComponent{name: "foundry-http"})
	app.AddComponent(&stubComponent{name: "foundry-health"})
	if len(app.components) != 3 {
		t.Errorf("expected 3 components, got %d", len(app.components))
	}
}

// --------------------------------------------------------------------------
// Configure
// --------------------------------------------------------------------------

func TestConfigure_NoComponents(t *testing.T) {
	app := NewApplication(nil)
	if err := app.Configure(nil); err != nil {
		t.Errorf("Configure with no components should return nil, got: %v", err)
	}
}

func TestConfigure_CallsEachComponent(t *testing.T) {
	app := NewApplication(nil)
	c1 := &stubComponent{name: "foundry-postgres"}
	c2 := &stubComponent{name: "foundry-http"}
	app.AddComponent(c1)
	app.AddComponent(c2)

	configs := map[string]ComponentConfig{
		"foundry-postgres": {"url": "postgres://localhost/db"},
	}
	if err := app.Configure(configs); err != nil {
		t.Fatalf("Configure unexpected error: %v", err)
	}
	if !c1.configured {
		t.Error("foundry-postgres Configure not called")
	}
	if !c2.configured {
		t.Error("foundry-http Configure not called")
	}
}

func TestConfigure_NilConfigFallsBackToEmpty(t *testing.T) {
	// Component with no entry in configs map should receive empty ComponentConfig
	app := NewApplication(nil)
	c := &stubComponent{name: "foundry-http"}
	app.AddComponent(c)

	if err := app.Configure(map[string]ComponentConfig{}); err != nil {
		t.Fatalf("Configure unexpected error: %v", err)
	}
	if !c.configured {
		t.Error("Configure not called when config map has no entry")
	}
}

func TestConfigure_PropagatesError(t *testing.T) {
	app := NewApplication(nil)
	want := errors.New("bad config")
	c := &stubComponent{name: "foundry-http", configureErr: want}
	app.AddComponent(c)

	err := app.Configure(nil)
	if err == nil {
		t.Fatal("expected error from Configure, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-http") {
		t.Errorf("error should mention component name, got: %v", err)
	}
	if !errors.Is(err, want) {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// Register
// --------------------------------------------------------------------------

func TestRegister_NoComponents(t *testing.T) {
	app := NewApplication(nil)
	if err := app.Register(); err != nil {
		t.Errorf("Register with no components should return nil, got: %v", err)
	}
}

func TestRegister_CallsEachComponent(t *testing.T) {
	app := NewApplication(nil)
	c1 := &stubComponent{name: "foundry-postgres"}
	c2 := &stubComponent{name: "foundry-http"}
	app.AddComponent(c1)
	app.AddComponent(c2)

	if err := app.Register(); err != nil {
		t.Fatalf("Register unexpected error: %v", err)
	}
	if !c1.registered {
		t.Error("foundry-postgres Register not called")
	}
	if !c2.registered {
		t.Error("foundry-http Register not called")
	}
}

func TestRegister_PropagatesError(t *testing.T) {
	app := NewApplication(nil)
	want := errors.New("register failed")
	c := &stubComponent{name: "foundry-grpc", registerErr: want}
	app.AddComponent(c)

	err := app.Register()
	if err == nil {
		t.Fatal("expected error from Register, got nil")
	}
	if !strings.Contains(err.Error(), "foundry-grpc") {
		t.Errorf("error should mention component name, got: %v", err)
	}
	if !errors.Is(err, want) {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}

// --------------------------------------------------------------------------
// Run
// --------------------------------------------------------------------------

func TestRun_NoComponents(t *testing.T) {
	app := NewApplication(nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancelled
	if err := app.Run(ctx); err != nil {
		t.Errorf("Run with no components should return nil, got: %v", err)
	}
}

func TestRun_StartsAndStopsComponents(t *testing.T) {
	app := NewApplication(nil)
	c1 := &stubComponent{name: "foundry-postgres"}
	c2 := &stubComponent{name: "foundry-http"}
	app.AddComponent(c1)
	app.AddComponent(c2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancelled so Run does not block

	if err := app.Run(ctx); err != nil {
		t.Fatalf("Run unexpected error: %v", err)
	}
	if !c1.started {
		t.Error("foundry-postgres Start not called")
	}
	if !c2.started {
		t.Error("foundry-http Start not called")
	}
	if !c1.stopped {
		t.Error("foundry-postgres Stop not called")
	}
	if !c2.stopped {
		t.Error("foundry-http Stop not called")
	}
}

func TestRun_StartError(t *testing.T) {
	app := NewApplication(nil)
	want := errors.New("cannot bind port")
	c := &stubComponent{name: "foundry-http", startErr: want}
	app.AddComponent(c)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := app.Run(ctx)
	if err == nil {
		t.Fatal("expected error from Run when Start fails, got nil")
	}
	if !errors.Is(err, want) {
		t.Errorf("expected wrapped start error, got: %v", err)
	}
}

func TestRun_StopError_ReturnsFirst(t *testing.T) {
	app := NewApplication(nil)
	want := errors.New("stop failed")
	c1 := &stubComponent{name: "foundry-http", stopErr: want}
	c2 := &stubComponent{name: "foundry-health"}
	app.AddComponent(c1)
	app.AddComponent(c2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := app.Run(ctx)
	if err == nil {
		t.Fatal("expected error from Run when Stop fails, got nil")
	}
	if !errors.Is(err, want) {
		t.Errorf("expected wrapped stop error, got: %v", err)
	}
}

func TestRun_StopReverseOrder(t *testing.T) {
	// Verify that Stop is called in reverse registration order.
	app := NewApplication(nil)
	var order []string
	type trackingComponent struct {
		stubComponent
		stopFn func()
	}
	makeTracking := func(name string) Component {
		return &struct{ stubComponent }{
			stubComponent: stubComponent{name: name},
		}
	}
	_ = makeTracking // unused below — use explicit stub

	// Use a slice to record stop order
	stopOrder := make([]string, 0, 3)
	for _, name := range []string{"a", "b", "c"} {
		n := name
		app.AddComponent(&stopOrderComponent{name: n, stopped: &stopOrder})
	}
	_ = order

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := app.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(stopOrder) != 3 {
		t.Fatalf("expected 3 stops, got %d", len(stopOrder))
	}
	// c, b, a — reverse of registration
	if stopOrder[0] != "c" || stopOrder[1] != "b" || stopOrder[2] != "a" {
		t.Errorf("expected reverse stop order [c b a], got %v", stopOrder)
	}
}

// stopOrderComponent records its Stop call name into a shared slice.
type stopOrderComponent struct {
	name    string
	stopped *[]string
}

func (s *stopOrderComponent) Name() string                        { return s.name }
func (s *stopOrderComponent) Version() string                     { return "v0.0.0" }
func (s *stopOrderComponent) AuditHash() string                   { return "" }
func (s *stopOrderComponent) Configure(cfg ComponentConfig) error { return nil }
func (s *stopOrderComponent) Register(app *Application) error     { return nil }
func (s *stopOrderComponent) Start(ctx context.Context) error     { return nil }
func (s *stopOrderComponent) Stop(ctx context.Context) error {
	*s.stopped = append(*s.stopped, s.name)
	return nil
}

// --------------------------------------------------------------------------
// HTTP / Middleware / gRPC / DB accessors
// --------------------------------------------------------------------------

func TestAddHTTPHandler(t *testing.T) {
	app := NewApplication(nil)
	h := &stubHandler{}
	app.AddHTTPHandler("/api/v1/items", h)
	app.AddHTTPHandler("/healthz", h)

	handlers := app.HTTPHandlers()
	if len(handlers) != 2 {
		t.Fatalf("expected 2 handlers, got %d", len(handlers))
	}
	if handlers[0].Pattern != "/api/v1/items" {
		t.Errorf("expected pattern /api/v1/items, got %q", handlers[0].Pattern)
	}
}

func TestAddMiddleware(t *testing.T) {
	app := NewApplication(nil)
	mw := func(next HTTPHandler) HTTPHandler { return next }
	app.AddMiddleware(mw)
	app.AddMiddleware(mw)

	mws := app.Middlewares()
	if len(mws) != 2 {
		t.Fatalf("expected 2 middlewares, got %d", len(mws))
	}
}

func TestAddGRPCService(t *testing.T) {
	app := NewApplication(nil)
	app.AddGRPCService("desc1", "impl1")
	app.AddGRPCService("desc2", struct{}{})

	svcs := app.GRPCServices()
	if len(svcs) != 2 {
		t.Fatalf("expected 2 gRPC services, got %d", len(svcs))
	}
	if svcs[0].Desc != "desc1" {
		t.Errorf("expected desc1, got %v", svcs[0].Desc)
	}
}

func TestSetDB_And_DB(t *testing.T) {
	app := NewApplication(nil)
	if app.DB() != nil {
		t.Error("expected nil DB before SetDB")
	}
	db := &stubDB{}
	app.SetDB(db)
	if app.DB() != db {
		t.Error("DB() did not return the value set by SetDB")
	}
}

// --------------------------------------------------------------------------
// ToResourceDefinitions
// --------------------------------------------------------------------------

func TestToResourceDefinitions_Empty(t *testing.T) {
	out := ToResourceDefinitions(nil)
	if len(out) != 0 {
		t.Errorf("expected 0 definitions, got %d", len(out))
	}
}

func TestToResourceDefinitions_CopiesAllFields(t *testing.T) {
	ir := []IRResource{
		{
			Name:   "Dinosaur",
			Plural: "dinosaurs",
			Fields: []IRField{
				{Name: "id", Type: "uuid", Required: true},
				{Name: "species", Type: "string", MaxLength: 255},
				{Name: "deleted_at", Type: "timestamp", SoftDelete: true},
				{Name: "created_at", Type: "timestamp", Auto: "created"},
			},
			Operations: []string{"create", "read", "list"},
			Events:     true,
		},
	}
	out := ToResourceDefinitions(ir)
	if len(out) != 1 {
		t.Fatalf("expected 1, got %d", len(out))
	}
	r := out[0]
	if r.Name != "Dinosaur" {
		t.Errorf("Name = %q, want Dinosaur", r.Name)
	}
	if r.Plural != "dinosaurs" {
		t.Errorf("Plural = %q, want dinosaurs", r.Plural)
	}
	if !r.Events {
		t.Error("Events should be true")
	}
	if len(r.Fields) != 4 {
		t.Fatalf("expected 4 fields, got %d", len(r.Fields))
	}
	if !r.Fields[0].Required {
		t.Error("id field Required should be true")
	}
	if r.Fields[1].MaxLength != 255 {
		t.Errorf("species MaxLength = %d, want 255", r.Fields[1].MaxLength)
	}
	if !r.Fields[2].SoftDelete {
		t.Error("deleted_at SoftDelete should be true")
	}
	if r.Fields[3].Auto != "created" {
		t.Errorf("created_at Auto = %q, want created", r.Fields[3].Auto)
	}
	if len(r.Operations) != 3 {
		t.Errorf("expected 3 operations, got %d", len(r.Operations))
	}
}

func TestToResourceDefinitions_Multiple(t *testing.T) {
	ir := []IRResource{
		{Name: "Widget", Plural: "widgets", Fields: []IRField{{Name: "id", Type: "uuid"}}, Operations: []string{"create"}},
		{Name: "Gadget", Plural: "gadgets", Fields: []IRField{{Name: "id", Type: "uuid"}}, Operations: []string{"read"}},
	}
	out := ToResourceDefinitions(ir)
	if len(out) != 2 {
		t.Fatalf("expected 2, got %d", len(out))
	}
	if out[0].Name != "Widget" || out[1].Name != "Gadget" {
		t.Errorf("unexpected names: %v %v", out[0].Name, out[1].Name)
	}
}

// --------------------------------------------------------------------------
// LoadSchemaJSON
// --------------------------------------------------------------------------

func TestLoadSchemaJSON_MissingFile(t *testing.T) {
	_, err := LoadSchemaJSON("/nonexistent/path/schema.json")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "read schema") {
		t.Errorf("expected 'read schema' in error, got: %v", err)
	}
}

func TestLoadSchemaJSON_InvalidJSON(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "schema*.json")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("{invalid json")
	f.Close()

	_, err = LoadSchemaJSON(f.Name())
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("expected 'not valid JSON' in error, got: %v", err)
	}
}

func TestLoadSchemaJSON_ValidFile(t *testing.T) {
	// Use the actual schema.json in the spec package
	schemaPath := filepath.Join(".", "schema.json")
	data, err := LoadSchemaJSON(schemaPath)
	if err != nil {
		t.Fatalf("LoadSchemaJSON(%q): %v", schemaPath, err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty schema data")
	}
}

// --------------------------------------------------------------------------
// Validate — helper to build a minimal valid IRSpec
// --------------------------------------------------------------------------

func minimalValidSpec() *IRSpec {
	return &IRSpec{
		APIVersion: "foundry/v1",
		Kind:       "Application",
		Metadata:   IRMetadata{Name: "my-app", Version: "1.0.0"},
		Components: map[string]string{"foundry-http": "v1.0.0"},
	}
}

func errContains(errs []error, sub string) bool {
	for _, e := range errs {
		if strings.Contains(e.Error(), sub) {
			return true
		}
	}
	return false
}

// --------------------------------------------------------------------------
// Validate — top-level fields
// --------------------------------------------------------------------------

func TestValidate_ValidMinimal(t *testing.T) {
	errs := Validate(minimalValidSpec())
	if len(errs) != 0 {
		t.Errorf("expected no errors for minimal valid spec, got: %v", errs)
	}
}

func TestValidate_WrongAPIVersion(t *testing.T) {
	spec := minimalValidSpec()
	spec.APIVersion = "v1"
	errs := Validate(spec)
	if !errContains(errs, "apiVersion") {
		t.Errorf("expected apiVersion error, got: %v", errs)
	}
}

func TestValidate_WrongKind(t *testing.T) {
	spec := minimalValidSpec()
	spec.Kind = "Service"
	errs := Validate(spec)
	if !errContains(errs, "kind") {
		t.Errorf("expected kind error, got: %v", errs)
	}
}

func TestValidate_MetadataNameNotKebab(t *testing.T) {
	spec := minimalValidSpec()
	spec.Metadata.Name = "My App"
	errs := Validate(spec)
	if !errContains(errs, "metadata.name") {
		t.Errorf("expected metadata.name error, got: %v", errs)
	}
}

func TestValidate_MetadataVersionBadFormat(t *testing.T) {
	spec := minimalValidSpec()
	spec.Metadata.Version = "v1.0.0" // should be 1.0.0 without v
	errs := Validate(spec)
	if !errContains(errs, "metadata.version") {
		t.Errorf("expected metadata.version error, got: %v", errs)
	}
}

func TestValidate_ComponentsEmpty(t *testing.T) {
	spec := minimalValidSpec()
	spec.Components = map[string]string{}
	errs := Validate(spec)
	if !errContains(errs, "components block must list") {
		t.Errorf("expected components error, got: %v", errs)
	}
}

func TestValidate_ComponentsNil(t *testing.T) {
	spec := minimalValidSpec()
	spec.Components = nil
	errs := Validate(spec)
	if !errContains(errs, "components block must list") {
		t.Errorf("expected components error, got: %v", errs)
	}
}

func TestValidate_UnknownComponent(t *testing.T) {
	spec := minimalValidSpec()
	spec.Components["not-a-real-component"] = "v1.0.0"
	errs := Validate(spec)
	if !errContains(errs, "unknown component") {
		t.Errorf("expected unknown component error, got: %v", errs)
	}
}

func TestValidate_ComponentVersionBadSemver(t *testing.T) {
	spec := minimalValidSpec()
	spec.Components["foundry-http"] = "1.0.0" // missing v prefix
	errs := Validate(spec)
	if !errContains(errs, "semver") {
		t.Errorf("expected semver error, got: %v", errs)
	}
}

// --------------------------------------------------------------------------
// Validate — resources
// --------------------------------------------------------------------------

func validResource() IRResource {
	return IRResource{
		Name:   "Widget",
		Plural: "widgets",
		Fields: []IRField{
			{Name: "id", Type: "uuid", Required: true},
			{Name: "label", Type: "string"},
		},
		Operations: []string{"create", "read"},
	}
}

func TestValidate_ResourceNameNotPascal(t *testing.T) {
	spec := minimalValidSpec()
	r := validResource()
	r.Name = "my-widget"
	spec.Resources = []IRResource{r}
	errs := Validate(spec)
	if !errContains(errs, "PascalCase") {
		t.Errorf("expected PascalCase error, got: %v", errs)
	}
}

func TestValidate_ResourcePluralNotKebab(t *testing.T) {
	spec := minimalValidSpec()
	r := validResource()
	r.Plural = "My Widgets"
	spec.Resources = []IRResource{r}
	errs := Validate(spec)
	if !errContains(errs, "plural") {
		t.Errorf("expected plural error, got: %v", errs)
	}
}

func TestValidate_DuplicateResourceName(t *testing.T) {
	spec := minimalValidSpec()
	r := validResource()
	spec.Resources = []IRResource{r, r}
	errs := Validate(spec)
	if !errContains(errs, "duplicate resource") {
		t.Errorf("expected duplicate resource error, got: %v", errs)
	}
}

func TestValidate_ResourceNoFields(t *testing.T) {
	spec := minimalValidSpec()
	r := validResource()
	r.Fields = nil
	spec.Resources = []IRResource{r}
	errs := Validate(spec)
	if !errContains(errs, "at least one field") {
		t.Errorf("expected field error, got: %v", errs)
	}
}

func TestValidate_FieldNameNotSnakeCase(t *testing.T) {
	spec := minimalValidSpec()
	r := validResource()
	r.Fields = []IRField{{Name: "MyField", Type: "string"}}
	spec.Resources = []IRResource{r}
	errs := Validate(spec)
	if !errContains(errs, "snake_case") {
		t.Errorf("expected snake_case error, got: %v", errs)
	}
}

func TestValidate_DuplicateFieldName(t *testing.T) {
	spec := minimalValidSpec()
	r := validResource()
	r.Fields = []IRField{
		{Name: "id", Type: "uuid"},
		{Name: "id", Type: "string"},
	}
	spec.Resources = []IRResource{r}
	errs := Validate(spec)
	if !errContains(errs, "duplicate field") {
		t.Errorf("expected duplicate field error, got: %v", errs)
	}
}

func TestValidate_UnknownFieldType(t *testing.T) {
	spec := minimalValidSpec()
	r := validResource()
	r.Fields = []IRField{{Name: "score", Type: "decimal"}}
	spec.Resources = []IRResource{r}
	errs := Validate(spec)
	if !errContains(errs, "unknown type") {
		t.Errorf("expected unknown type error, got: %v", errs)
	}
}

func TestValidate_MaxLengthOnNonString(t *testing.T) {
	spec := minimalValidSpec()
	r := validResource()
	r.Fields = []IRField{{Name: "count", Type: "int", MaxLength: 100}}
	spec.Resources = []IRResource{r}
	errs := Validate(spec)
	if !errContains(errs, "max_length") {
		t.Errorf("expected max_length error, got: %v", errs)
	}
}

func TestValidate_AutoInvalidValue(t *testing.T) {
	spec := minimalValidSpec()
	r := validResource()
	r.Fields = []IRField{{Name: "ts", Type: "timestamp", Auto: "modified"}}
	spec.Resources = []IRResource{r}
	errs := Validate(spec)
	if !errContains(errs, "auto must be") {
		t.Errorf("expected auto error, got: %v", errs)
	}
}

func TestValidate_SoftDeleteNonTimestamp(t *testing.T) {
	spec := minimalValidSpec()
	r := validResource()
	r.Fields = []IRField{{Name: "deleted", Type: "bool", SoftDelete: true}}
	spec.Resources = []IRResource{r}
	errs := Validate(spec)
	if !errContains(errs, "soft_delete field must have type 'timestamp'") {
		t.Errorf("expected soft_delete type error, got: %v", errs)
	}
}

func TestValidate_MultipleSoftDeleteFields(t *testing.T) {
	spec := minimalValidSpec()
	r := validResource()
	r.Fields = []IRField{
		{Name: "deleted_at", Type: "timestamp", SoftDelete: true},
		{Name: "purged_at", Type: "timestamp", SoftDelete: true},
	}
	spec.Resources = []IRResource{r}
	errs := Validate(spec)
	if !errContains(errs, "at most one soft_delete") {
		t.Errorf("expected multiple soft_delete error, got: %v", errs)
	}
}

func TestValidate_ResourceNoOperations(t *testing.T) {
	spec := minimalValidSpec()
	r := validResource()
	r.Operations = nil
	spec.Resources = []IRResource{r}
	errs := Validate(spec)
	if !errContains(errs, "at least one operation") {
		t.Errorf("expected operations error, got: %v", errs)
	}
}

func TestValidate_UnknownOperation(t *testing.T) {
	spec := minimalValidSpec()
	r := validResource()
	r.Operations = []string{"create", "fly"}
	spec.Resources = []IRResource{r}
	errs := Validate(spec)
	if !errContains(errs, "unknown operation") {
		t.Errorf("expected unknown operation error, got: %v", errs)
	}
}

func TestValidate_DuplicateOperation(t *testing.T) {
	spec := minimalValidSpec()
	r := validResource()
	r.Operations = []string{"create", "create"}
	spec.Resources = []IRResource{r}
	errs := Validate(spec)
	if !errContains(errs, "duplicate operation") {
		t.Errorf("expected duplicate operation error, got: %v", errs)
	}
}

func TestValidate_EventsRequiresFoundryEvents(t *testing.T) {
	spec := minimalValidSpec()
	r := validResource()
	r.Events = true
	spec.Resources = []IRResource{r}
	errs := Validate(spec)
	if !errContains(errs, "foundry-events") {
		t.Errorf("expected foundry-events error, got: %v", errs)
	}
}

func TestValidate_ResourcesWithoutDatabase(t *testing.T) {
	spec := minimalValidSpec()
	r := validResource()
	spec.Resources = []IRResource{r}
	// no spec.Database block
	errs := Validate(spec)
	if !errContains(errs, "no database block") {
		t.Errorf("expected database block error, got: %v", errs)
	}
}

// --------------------------------------------------------------------------
// Validate — auth
// --------------------------------------------------------------------------

func TestValidate_JWTMissingJWKURL(t *testing.T) {
	spec := minimalValidSpec()
	spec.Components["foundry-auth-jwt"] = "v1.0.0"
	spec.Auth = &IRAuth{Type: "jwt", JWKURL: ""}
	errs := Validate(spec)
	if !errContains(errs, "jwk_url") {
		t.Errorf("expected jwk_url error, got: %v", errs)
	}
}

func TestValidate_JWTMissingFoundryAuthJWT(t *testing.T) {
	spec := minimalValidSpec()
	// No foundry-auth-jwt in components
	spec.Auth = &IRAuth{Type: "jwt", JWKURL: "https://example.com/.well-known/jwks.json"}
	errs := Validate(spec)
	if !errContains(errs, "foundry-auth-jwt") {
		t.Errorf("expected foundry-auth-jwt error, got: %v", errs)
	}
}

// --------------------------------------------------------------------------
// Validate — gRPC
// --------------------------------------------------------------------------

func TestValidate_GRPCEnabledWithoutComponent(t *testing.T) {
	spec := minimalValidSpec()
	spec.API = &IRAPI{GRPC: &IRGRPCConfig{Enabled: true, Port: 9090}}
	errs := Validate(spec)
	if !errContains(errs, "foundry-grpc") {
		t.Errorf("expected foundry-grpc error, got: %v", errs)
	}
}

// --------------------------------------------------------------------------
// Validate — database
// --------------------------------------------------------------------------

func TestValidate_DatabaseWithoutFoundryPostgres(t *testing.T) {
	spec := minimalValidSpec()
	spec.Database = &IRDatabase{Type: "postgres", Migrations: "migrations"}
	errs := Validate(spec)
	if !errContains(errs, "foundry-postgres") {
		t.Errorf("expected foundry-postgres error, got: %v", errs)
	}
}

// --------------------------------------------------------------------------
// Validate — graph
// --------------------------------------------------------------------------

func TestValidate_GraphAgeWithoutComponent(t *testing.T) {
	spec := minimalValidSpec()
	spec.Graph = &IRGraphConfig{Backend: "age"}
	errs := Validate(spec)
	if !errContains(errs, "foundry-graph-age") {
		t.Errorf("expected foundry-graph-age error, got: %v", errs)
	}
}

func TestValidate_GraphNodeEmptyLabel(t *testing.T) {
	spec := minimalValidSpec()
	spec.Components["foundry-graph-age"] = "v1.0.0"
	spec.Graph = &IRGraphConfig{
		Backend:   "age",
		NodeTypes: []IRGraphNodeType{{Label: ""}},
	}
	errs := Validate(spec)
	if !errContains(errs, "label is required") {
		t.Errorf("expected label required error, got: %v", errs)
	}
}

func TestValidate_GraphEdgeEmptyLabel(t *testing.T) {
	spec := minimalValidSpec()
	spec.Components["foundry-graph-age"] = "v1.0.0"
	spec.Graph = &IRGraphConfig{
		Backend:   "age",
		EdgeTypes: []IRGraphEdgeType{{Label: "", From: "A", To: "B"}},
	}
	errs := Validate(spec)
	if !errContains(errs, "label is required") {
		t.Errorf("expected edge label required error, got: %v", errs)
	}
}

func TestValidate_GraphEdgeEmptyFrom(t *testing.T) {
	spec := minimalValidSpec()
	spec.Components["foundry-graph-age"] = "v1.0.0"
	spec.Graph = &IRGraphConfig{
		Backend:   "age",
		EdgeTypes: []IRGraphEdgeType{{Label: "HAS_NODE", From: "", To: "B"}},
	}
	errs := Validate(spec)
	if !errContains(errs, "from is required") {
		t.Errorf("expected from required error, got: %v", errs)
	}
}

func TestValidate_GraphEdgeEmptyTo(t *testing.T) {
	spec := minimalValidSpec()
	spec.Components["foundry-graph-age"] = "v1.0.0"
	spec.Graph = &IRGraphConfig{
		Backend:   "age",
		EdgeTypes: []IRGraphEdgeType{{Label: "HAS_NODE", From: "A", To: ""}},
	}
	errs := Validate(spec)
	if !errContains(errs, "to is required") {
		t.Errorf("expected to required error, got: %v", errs)
	}
}

func TestValidate_GraphEdgeFromNotDeclaredNode(t *testing.T) {
	spec := minimalValidSpec()
	spec.Components["foundry-graph-age"] = "v1.0.0"
	spec.Graph = &IRGraphConfig{
		Backend:   "age",
		NodeTypes: []IRGraphNodeType{{Label: "Cluster"}},
		EdgeTypes: []IRGraphEdgeType{{Label: "HAS_NODE", From: "Unknown", To: "Cluster"}},
	}
	errs := Validate(spec)
	if !errContains(errs, "not a declared node_type label") {
		t.Errorf("expected undeclared node label error, got: %v", errs)
	}
}

func TestValidate_GraphEdgeToNotDeclaredNode(t *testing.T) {
	spec := minimalValidSpec()
	spec.Components["foundry-graph-age"] = "v1.0.0"
	spec.Graph = &IRGraphConfig{
		Backend:   "age",
		NodeTypes: []IRGraphNodeType{{Label: "Cluster"}},
		EdgeTypes: []IRGraphEdgeType{{Label: "HAS_NODE", From: "Cluster", To: "Unknown"}},
	}
	errs := Validate(spec)
	if !errContains(errs, "not a declared node_type label") {
		t.Errorf("expected undeclared node label error for 'to', got: %v", errs)
	}
}

// --------------------------------------------------------------------------
// Validate — events
// --------------------------------------------------------------------------

func TestValidate_EventsKafkaWithoutComponent(t *testing.T) {
	spec := minimalValidSpec()
	spec.Events = &IREventsConfig{Backend: "kafka"}
	errs := Validate(spec)
	if !errContains(errs, "foundry-kafka") {
		t.Errorf("expected foundry-kafka error, got: %v", errs)
	}
}

func TestValidate_EventsNATSWithoutComponent(t *testing.T) {
	spec := minimalValidSpec()
	spec.Events = &IREventsConfig{Backend: "nats"}
	errs := Validate(spec)
	if !errContains(errs, "foundry-nats") {
		t.Errorf("expected foundry-nats error, got: %v", errs)
	}
}

func TestValidate_EventsRedisStreamsWithoutComponent(t *testing.T) {
	spec := minimalValidSpec()
	spec.Events = &IREventsConfig{Backend: "redis-streams"}
	errs := Validate(spec)
	if !errContains(errs, "foundry-redis-streams") {
		t.Errorf("expected foundry-redis-streams error, got: %v", errs)
	}
}

// --------------------------------------------------------------------------
// Validate — authz
// --------------------------------------------------------------------------

func TestValidate_AuthzSpiceDBWithoutComponent(t *testing.T) {
	spec := minimalValidSpec()
	spec.Authz = &IRAuthzConfig{Backend: "spicedb"}
	errs := Validate(spec)
	if !errContains(errs, "foundry-auth-spicedb") {
		t.Errorf("expected foundry-auth-spicedb error, got: %v", errs)
	}
}

// --------------------------------------------------------------------------
// Validate — state
// --------------------------------------------------------------------------

func TestValidate_StateWithoutFoundryRedis(t *testing.T) {
	spec := minimalValidSpec()
	spec.State = &IRStateConfig{Backend: "redis"}
	errs := Validate(spec)
	if !errContains(errs, "foundry-redis") {
		t.Errorf("expected foundry-redis error, got: %v", errs)
	}
}

// --------------------------------------------------------------------------
// Validate — bi-temporal
// --------------------------------------------------------------------------

func TestValidate_BiTemporalWithoutFoundryTemporal(t *testing.T) {
	spec := minimalValidSpec()
	spec.BiTemporal = &IRBiTemporalConfig{Enabled: true}
	errs := Validate(spec)
	if !errContains(errs, "foundry-temporal") {
		t.Errorf("expected foundry-temporal error, got: %v", errs)
	}
}

func TestValidate_BiTemporalWithoutDatabase(t *testing.T) {
	spec := minimalValidSpec()
	spec.Components["foundry-temporal"] = "v1.0.0"
	spec.BiTemporal = &IRBiTemporalConfig{Enabled: true}
	errs := Validate(spec)
	if !errContains(errs, "bi_temporal requires a database") {
		t.Errorf("expected bi_temporal database error, got: %v", errs)
	}
}

func TestValidate_BiTemporalDisabledNoErrors(t *testing.T) {
	spec := minimalValidSpec()
	spec.BiTemporal = &IRBiTemporalConfig{Enabled: false}
	errs := Validate(spec)
	if errContains(errs, "bi_temporal") {
		t.Errorf("disabled bi_temporal should not trigger errors, got: %v", errs)
	}
}

// --------------------------------------------------------------------------
// Validate — workflows
// --------------------------------------------------------------------------

func TestValidate_WorkflowsWithoutFoundryTemporal(t *testing.T) {
	spec := minimalValidSpec()
	spec.Workflows = &IRWorkflowsConfig{Namespace: "ns", WorkerQueue: "q"}
	errs := Validate(spec)
	if !errContains(errs, "foundry-temporal") {
		t.Errorf("expected foundry-temporal error, got: %v", errs)
	}
}

func TestValidate_WorkflowsMissingNamespace(t *testing.T) {
	spec := minimalValidSpec()
	spec.Components["foundry-temporal"] = "v1.0.0"
	spec.Workflows = &IRWorkflowsConfig{Namespace: "", WorkerQueue: "my-queue"}
	errs := Validate(spec)
	if !errContains(errs, "namespace is required") {
		t.Errorf("expected namespace error, got: %v", errs)
	}
}

func TestValidate_WorkflowsMissingWorkerQueue(t *testing.T) {
	spec := minimalValidSpec()
	spec.Components["foundry-temporal"] = "v1.0.0"
	spec.Workflows = &IRWorkflowsConfig{Namespace: "my-ns", WorkerQueue: ""}
	errs := Validate(spec)
	if !errContains(errs, "worker_queue is required") {
		t.Errorf("expected worker_queue error, got: %v", errs)
	}
}

// --------------------------------------------------------------------------
// Validate — tenancy
// --------------------------------------------------------------------------

func TestValidate_TenancyWithoutFoundryTenancy(t *testing.T) {
	spec := minimalValidSpec()
	spec.Tenancy = &IRTenancyConfig{Strategy: "row"}
	errs := Validate(spec)
	if !errContains(errs, "foundry-tenancy") {
		t.Errorf("expected foundry-tenancy error, got: %v", errs)
	}
}

func TestValidate_TenancyInvalidStrategy(t *testing.T) {
	spec := minimalValidSpec()
	spec.Components["foundry-tenancy"] = "v1.0.0"
	spec.Tenancy = &IRTenancyConfig{Strategy: "shard"}
	errs := Validate(spec)
	if !errContains(errs, "tenancy.strategy") {
		t.Errorf("expected tenancy.strategy error, got: %v", errs)
	}
}

func TestValidate_TenancyValidStrategies(t *testing.T) {
	for _, strategy := range []string{"row", "schema", "database"} {
		spec := minimalValidSpec()
		spec.Components["foundry-tenancy"] = "v1.0.0"
		spec.Tenancy = &IRTenancyConfig{Strategy: strategy}
		errs := Validate(spec)
		if errContains(errs, "tenancy.strategy") {
			t.Errorf("strategy %q should be valid, got: %v", strategy, errs)
		}
	}
}

func TestValidate_TenancyEmptyStrategy(t *testing.T) {
	// empty strategy is allowed (Strategy == "")
	spec := minimalValidSpec()
	spec.Components["foundry-tenancy"] = "v1.0.0"
	spec.Tenancy = &IRTenancyConfig{Strategy: ""}
	errs := Validate(spec)
	if errContains(errs, "tenancy.strategy") {
		t.Errorf("empty strategy should not produce error, got: %v", errs)
	}
}

// --------------------------------------------------------------------------
// Validate — services
// --------------------------------------------------------------------------

func TestValidate_ServiceUndeclaredComponent(t *testing.T) {
	spec := minimalValidSpec()
	spec.Services = []IRService{
		{Name: "api", Role: "gateway", Components: []string{"foundry-postgres"}},
	}
	errs := Validate(spec)
	if !errContains(errs, "not declared in the top-level components") {
		t.Errorf("expected undeclared component error, got: %v", errs)
	}
}

// --------------------------------------------------------------------------
// Validate — hooks
// --------------------------------------------------------------------------

func TestValidate_HookNameNotKebab(t *testing.T) {
	spec := minimalValidSpec()
	spec.Hooks = []IRHook{{Name: "My Hook", Point: "pre-db", Implementation: "hooks/my.go"}}
	errs := Validate(spec)
	if !errContains(errs, "name must be kebab-case") {
		t.Errorf("expected hook name error, got: %v", errs)
	}
}

func TestValidate_DuplicateHookName(t *testing.T) {
	spec := minimalValidSpec()
	spec.Hooks = []IRHook{
		{Name: "audit", Point: "pre-db", Implementation: "hooks/audit.go"},
		{Name: "audit", Point: "post-db", Implementation: "hooks/audit2.go"},
	}
	errs := Validate(spec)
	if !errContains(errs, "duplicate hook") {
		t.Errorf("expected duplicate hook error, got: %v", errs)
	}
}

func TestValidate_HookUnknownPoint(t *testing.T) {
	spec := minimalValidSpec()
	spec.Hooks = []IRHook{{Name: "my-hook", Point: "pre-cache", Implementation: "hooks/my.go"}}
	errs := Validate(spec)
	if !errContains(errs, "unknown point") {
		t.Errorf("expected unknown point error, got: %v", errs)
	}
}

func TestValidate_HookBadImplementationPattern(t *testing.T) {
	spec := minimalValidSpec()
	spec.Hooks = []IRHook{{Name: "my-hook", Point: "pre-handler", Implementation: "src/my.go"}}
	errs := Validate(spec)
	if !errContains(errs, "implementation must match") {
		t.Errorf("expected implementation pattern error, got: %v", errs)
	}
}

func TestValidate_HookTopicOnWrongPoint(t *testing.T) {
	spec := minimalValidSpec()
	spec.Hooks = []IRHook{{
		Name:           "my-hook",
		Point:          "pre-handler",
		Topic:          "some-topic",
		Implementation: "hooks/my.go",
	}}
	errs := Validate(spec)
	if !errContains(errs, "topic is only valid for pre-publish or post-consume") {
		t.Errorf("expected topic point error, got: %v", errs)
	}
}

func TestValidate_HookRoutesOnWrongPoint(t *testing.T) {
	spec := minimalValidSpec()
	spec.Hooks = []IRHook{{
		Name:           "my-hook",
		Point:          "pre-db",
		Routes:         []string{"/api/items"},
		Implementation: "hooks/my.go",
	}}
	errs := Validate(spec)
	if !errContains(errs, "routes is only valid for pre-handler or post-handler") {
		t.Errorf("expected routes point error, got: %v", errs)
	}
}

func TestValidate_HookPreDBWithoutDatabase(t *testing.T) {
	spec := minimalValidSpec()
	spec.Hooks = []IRHook{{
		Name:           "audit",
		Point:          "pre-db",
		Implementation: "hooks/audit.go",
	}}
	// No spec.Database set
	errs := Validate(spec)
	if !errContains(errs, "pre-db/post-db hooks require a database") {
		t.Errorf("expected pre-db requires database error, got: %v", errs)
	}
}

func TestValidate_HookPostDBWithoutDatabase(t *testing.T) {
	spec := minimalValidSpec()
	spec.Hooks = []IRHook{{
		Name:           "audit",
		Point:          "post-db",
		Implementation: "hooks/audit.go",
	}}
	errs := Validate(spec)
	if !errContains(errs, "pre-db/post-db hooks require a database") {
		t.Errorf("expected post-db requires database error, got: %v", errs)
	}
}

// --------------------------------------------------------------------------
// Validate — valid full spec (smoke test)
// --------------------------------------------------------------------------

func TestValidate_FullValidSpec(t *testing.T) {
	spec := &IRSpec{
		APIVersion: "foundry/v1",
		Kind:       "Application",
		Metadata:   IRMetadata{Name: "fleet-manager", Version: "1.0.0"},
		Components: map[string]string{
			"foundry-http":     "v1.0.0",
			"foundry-postgres": "v1.0.0",
			"foundry-auth-jwt": "v1.0.0",
		},
		Database: &IRDatabase{Type: "postgres", Migrations: "migrations"},
		Auth:     &IRAuth{Type: "jwt", JWKURL: "https://example.com/.well-known/jwks.json"},
		Resources: []IRResource{
			{
				Name:   "Fleet",
				Plural: "fleets",
				Fields: []IRField{
					{Name: "id", Type: "uuid", Required: true},
					{Name: "name", Type: "string", MaxLength: 255},
					{Name: "deleted_at", Type: "timestamp", SoftDelete: true},
				},
				Operations: []string{"create", "read", "update", "delete", "list"},
			},
		},
	}
	errs := Validate(spec)
	if len(errs) != 0 {
		t.Errorf("expected no errors for full valid spec, got: %v", errs)
	}
}
