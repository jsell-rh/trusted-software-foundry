package spec

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
)

// IRSpec is the top-level parsed representation of an app.foundry.yaml file.
// All fields map 1:1 to the JSON Schema (foundry/spec/schema.json).
type IRSpec struct {
	APIVersion string            `yaml:"apiVersion" json:"apiVersion"`
	Kind       string            `yaml:"kind"       json:"kind"`
	Metadata   IRMetadata        `yaml:"metadata"   json:"metadata"`
	Components map[string]string `yaml:"components" json:"components"`
	Resources  []IRResource      `yaml:"resources"  json:"resources"`
	API        *IRAPI            `yaml:"api"        json:"api,omitempty"`
	Auth       *IRAuth           `yaml:"auth"       json:"auth,omitempty"`
	Database   *IRDatabase       `yaml:"database"   json:"database,omitempty"`
	Observ     *IRObservability  `yaml:"observability" json:"observability,omitempty"`
	// Advanced capabilities — all optional
	Graph      *IRGraphConfig      `yaml:"graph"        json:"graph,omitempty"`
	Services   []IRService         `yaml:"services"     json:"services,omitempty"`
	Events     *IREventsConfig     `yaml:"events"       json:"events,omitempty"`
	Authz      *IRAuthzConfig      `yaml:"authz"        json:"authz,omitempty"`
	State      *IRStateConfig      `yaml:"state"        json:"state,omitempty"`
	BiTemporal *IRBiTemporalConfig `yaml:"bi_temporal"  json:"bi_temporal,omitempty"`
	Workflows  *IRWorkflowsConfig  `yaml:"workflows"    json:"workflows,omitempty"`
	Tenancy    *IRTenancyConfig    `yaml:"tenancy"      json:"tenancy,omitempty"`
	Hooks      []IRHook            `yaml:"hooks"        json:"hooks,omitempty"`
}

// IRHook declares a custom code injection point in the application lifecycle.
// The compiler copies the referenced Go file into the generated project and
// generates a typed call site at the declared lifecycle point.
type IRHook struct {
	Name           string   `yaml:"name"           json:"name"`
	Point          string   `yaml:"point"          json:"point"`
	Service        string   `yaml:"service"        json:"service,omitempty"`
	Routes         []string `yaml:"routes"         json:"routes,omitempty"`
	Topic          string   `yaml:"topic"          json:"topic,omitempty"`
	Implementation string   `yaml:"implementation" json:"implementation"`
}

// IRGraphConfig configures property graph capabilities (Apache AGE).
type IRGraphConfig struct {
	Backend   string            `yaml:"backend"    json:"backend"`
	GraphName string            `yaml:"graph_name" json:"graph_name,omitempty"`
	NodeTypes []IRGraphNodeType `yaml:"node_types" json:"node_types,omitempty"`
	EdgeTypes []IRGraphEdgeType `yaml:"edge_types" json:"edge_types,omitempty"`
	Mutations *IRGraphMutations `yaml:"mutations"  json:"mutations,omitempty"`
	Queries   *IRGraphQueries   `yaml:"queries"    json:"queries,omitempty"`
}

// IRGraphNodeType describes a node label in the Apache AGE property graph.
// Label is PascalCase (e.g. "Cluster"). Properties is a list of field names
// to persist on the node — type information is derived from the resource definition.
type IRGraphNodeType struct {
	Label      string   `yaml:"label"      json:"label"`
	IDField    string   `yaml:"id_field"   json:"id_field,omitempty"`
	Properties []string `yaml:"properties" json:"properties,omitempty"`
}

// IRGraphEdgeType describes a directed relationship between two node labels.
// Label follows SCREAMING_SNAKE_CASE convention (e.g. "HAS_NODE_POOL").
type IRGraphEdgeType struct {
	Label      string   `yaml:"label"      json:"label"`
	From       string   `yaml:"from"       json:"from"`
	To         string   `yaml:"to"         json:"to"`
	Directed   bool     `yaml:"directed"   json:"directed"`
	Properties []string `yaml:"properties" json:"properties,omitempty"`
}

// IRGraphMutations configures what mutation operations are allowed.
type IRGraphMutations struct {
	Operations     []string `yaml:"operations"      json:"operations,omitempty"`
	BulkLoading    bool     `yaml:"bulk_loading"    json:"bulk_loading"`
	MutationFormat string   `yaml:"mutation_format" json:"mutation_format"`
}

// IRGraphQueries configures the query language and API exposure.
type IRGraphQueries struct {
	Language  string `yaml:"language"   json:"language"`
	MaxDepth  int    `yaml:"max_depth"  json:"max_depth"`
	ExposeAPI bool   `yaml:"expose_api" json:"expose_api"`
}

// IRService describes one service in a multi-service application.
type IRService struct {
	Name       string             `yaml:"name"       json:"name"`
	Role       string             `yaml:"role"       json:"role"`
	Port       int                `yaml:"port"       json:"port,omitempty"`
	Components []string           `yaml:"components" json:"components,omitempty"`
	Resources  interface{}        `yaml:"resources"  json:"resources,omitempty"` // "all" or []string
	Triggers   []IRServiceTrigger `yaml:"triggers"  json:"triggers,omitempty"`
}

// IRServiceTrigger maps an event to a handler in a worker service.
type IRServiceTrigger struct {
	Event   string `yaml:"event"   json:"event"`
	Handler string `yaml:"handler" json:"handler"`
}

// IREventsConfig describes the event bus and topic layout.
type IREventsConfig struct {
	Backend        string                  `yaml:"backend"         json:"backend"`
	BrokerURL      string                  `yaml:"broker_url"      json:"broker_url,omitempty"` // shorthand for Broker.URL
	Broker         *IREventsBroker         `yaml:"broker"          json:"broker,omitempty"`
	SchemaRegistry *IREventsSchemaRegistry `yaml:"schema_registry" json:"schema_registry,omitempty"`
	Topics         []IREventTopic          `yaml:"topics"          json:"topics,omitempty"`
	Producers      []IREventProducer       `yaml:"producers"       json:"producers,omitempty"`
	Consumers      []IREventConsumer       `yaml:"consumers"       json:"consumers,omitempty"`
}

// IREventsBroker holds broker connection info.
type IREventsBroker struct {
	URL string `yaml:"url" json:"url"`
}

// IREventsSchemaRegistry configures the event schema registry.
type IREventsSchemaRegistry struct {
	URL    string `yaml:"url"    json:"url"`
	Format string `yaml:"format" json:"format"`
}

// IREventTopic describes a single topic.
type IREventTopic struct {
	Name           string   `yaml:"name"            json:"name"`
	Resource       string   `yaml:"resource"        json:"resource,omitempty"`
	Operations     []string `yaml:"operations"      json:"operations,omitempty"`
	Partitions     int      `yaml:"partitions"      json:"partitions"`
	Replication    int      `yaml:"replication"     json:"replication"`
	RetentionHours int      `yaml:"retention_hours" json:"retention_hours,omitempty"`
	Schema         string   `yaml:"schema"          json:"schema,omitempty"`
	Role           string   `yaml:"role"            json:"role"`
	Source         string   `yaml:"source"          json:"source,omitempty"`
}

// IREventProducer associates a service with the topics it produces to.
type IREventProducer struct {
	Service string   `yaml:"service" json:"service"`
	Topics  []string `yaml:"topics"  json:"topics"`
}

// IREventConsumer associates a service with the topics it consumes.
type IREventConsumer struct {
	Service    string   `yaml:"service"     json:"service"`
	Topics     []string `yaml:"topics"      json:"topics"`
	GroupID    string   `yaml:"group_id"    json:"group_id,omitempty"`
	ErrorTopic string   `yaml:"error_topic" json:"error_topic,omitempty"`
}

// IRAuthzConfig configures external authorization.
type IRAuthzConfig struct {
	Backend     string            `yaml:"backend"     json:"backend"`
	SpiceDB     *IRSpiceDB        `yaml:"spicedb"     json:"spicedb,omitempty"`
	SchemaFile  string            `yaml:"schema_file" json:"schema_file,omitempty"`
	Enforcement *IREnforcement    `yaml:"enforcement" json:"enforcement,omitempty"`
	Relations   []IRAuthzRelation `yaml:"relations"   json:"relations,omitempty"`
	Policies    []IRAuthzPolicy   `yaml:"policies"    json:"policies,omitempty"`
}

// IRAuthzRelation declares a SpiceDB relation between a resource and a subject type.
type IRAuthzRelation struct {
	Resource string `yaml:"resource" json:"resource"`
	Relation string `yaml:"relation" json:"relation"`
	Subject  string `yaml:"subject"  json:"subject"`
}

// IRSpiceDB holds SpiceDB connection config.
type IRSpiceDB struct {
	Endpoint string `yaml:"endpoint" json:"endpoint"`
	Token    string `yaml:"token"    json:"token"`
	TLS      bool   `yaml:"tls"      json:"tls"`
}

// IREnforcement holds the default authz decision.
type IREnforcement struct {
	Default string `yaml:"default" json:"default"`
}

// IRAuthzPolicy binds resource operations to authz permission strings.
type IRAuthzPolicy struct {
	Resource    string            `yaml:"resource"     json:"resource"`
	SubjectType string            `yaml:"subject_type" json:"subject_type,omitempty"`
	ObjectType  string            `yaml:"object_type"  json:"object_type,omitempty"`
	Operations  map[string]string `yaml:"operations"   json:"operations"`
}

// IRStateConfig configures external state backends (Redis).
// Backend/URL/Keys are the short-form fields. Backends/Uses are the long-form.
type IRStateConfig struct {
	Backend  string           `yaml:"backend"  json:"backend,omitempty"` // "redis"
	URL      string           `yaml:"url"      json:"url,omitempty"`     // Redis URL
	Keys     []IRStateKey     `yaml:"keys"     json:"keys,omitempty"`    // named state entries
	Backends []IRStateBackend `yaml:"backends" json:"backends,omitempty"`
	Uses     []IRStateUse     `yaml:"uses"     json:"uses,omitempty"`
}

// IRStateKey is a named state entry with a strategy.
type IRStateKey struct {
	Name       string `yaml:"name"        json:"name"`
	Resource   string `yaml:"resource"    json:"resource,omitempty"`
	Strategy   string `yaml:"strategy"    json:"strategy"` // cache, distributed_lock, rate_limit, counter
	TTLSeconds int    `yaml:"ttl_seconds" json:"ttl_seconds,omitempty"`
}

// IRStateBackend is a named Redis backend.
type IRStateBackend struct {
	Name       string `yaml:"name"        json:"name"`
	Type       string `yaml:"type"        json:"type"`
	URL        string `yaml:"url"         json:"url"`
	DefaultTTL int    `yaml:"default_ttl" json:"default_ttl,omitempty"`
}

// IRStateUse declares how a backend is used (cache, rate limit, lock).
type IRStateUse struct {
	Cache             string      `yaml:"cache"                json:"cache,omitempty"`
	RateLimit         string      `yaml:"rate_limit"           json:"rate_limit,omitempty"`
	DistributedLock   string      `yaml:"distributed_lock"     json:"distributed_lock,omitempty"`
	Resources         interface{} `yaml:"resources"            json:"resources,omitempty"`
	Routes            []string    `yaml:"routes"               json:"routes,omitempty"`
	RequestsPerSecond int         `yaml:"requests_per_second"  json:"requests_per_second,omitempty"`
	Burst             int         `yaml:"burst"                json:"burst,omitempty"`
	Operations        []string    `yaml:"operations"           json:"operations,omitempty"`
}

// IRBiTemporalConfig configures bi-temporal data tracking (valid time + transaction time).
// Use the workflows block for Temporal.io workflow engine configuration.
type IRBiTemporalConfig struct {
	Enabled         bool                  `yaml:"enabled"          json:"enabled"`
	ValidTime       *IRValidTime          `yaml:"valid_time"       json:"valid_time,omitempty"`
	TransactionTime *IRTransactionTime    `yaml:"transaction_time" json:"transaction_time,omitempty"`
	Resources       interface{}           `yaml:"resources"        json:"resources,omitempty"`
	QueryAPI        *IRBiTemporalQueryAPI `yaml:"query_api"       json:"query_api,omitempty"`
}

// IRValidTime configures the valid-time field.
type IRValidTime struct {
	Field string `yaml:"field" json:"field"`
}

// IRTransactionTime configures auto-managed transaction time.
type IRTransactionTime struct {
	Auto bool `yaml:"auto" json:"auto"`
}

// IRBiTemporalQueryAPI configures AS-OF query parameters.
type IRBiTemporalQueryAPI struct {
	AsOfParam    string `yaml:"as_of_param"   json:"as_of_param"`
	BetweenParam string `yaml:"between_param" json:"between_param"`
}

// IRWorkflowsConfig configures Temporal.io durable workflow execution.
type IRWorkflowsConfig struct {
	Namespace   string          `yaml:"namespace"    json:"namespace"`
	WorkerQueue string          `yaml:"worker_queue" json:"worker_queue"`
	Host        string          `yaml:"host"         json:"host,omitempty"`
	Workflows   []IRWorkflowDef `yaml:"workflows"    json:"workflows,omitempty"`
}

// IRWorkflowDef declares a single Temporal.io workflow.
type IRWorkflowDef struct {
	Name       string   `yaml:"name"       json:"name"`
	Resource   string   `yaml:"resource"   json:"resource,omitempty"`
	Trigger    string   `yaml:"trigger"    json:"trigger,omitempty"`
	Activities []string `yaml:"activities" json:"activities,omitempty"`
}

// IRTenancyConfig configures multi-tenant isolation.
// Strategy, Field, and Header are the preferred short-form fields.
// Model is kept for compatibility with the longer form.
type IRTenancyConfig struct {
	Strategy         string              `yaml:"strategy"            json:"strategy,omitempty"`
	Field            string              `yaml:"field"               json:"field,omitempty"`
	Header           string              `yaml:"header"              json:"header,omitempty"`
	Claim            string              `yaml:"claim"               json:"claim,omitempty"`
	Model            string              `yaml:"model"               json:"model,omitempty"`
	TenantIdentifier *IRTenantIdentifier `yaml:"tenant_identifier"   json:"tenant_identifier,omitempty"`
	Resources        interface{}         `yaml:"resources"           json:"resources,omitempty"`
	AdminBypass      *IRAdminBypass      `yaml:"admin_bypass"        json:"admin_bypass,omitempty"`
}

// IRTenantIdentifier specifies how tenant identity is extracted from requests.
type IRTenantIdentifier struct {
	Source string `yaml:"source" json:"source"`
	Claim  string `yaml:"claim"  json:"claim,omitempty"`
	Header string `yaml:"header" json:"header,omitempty"`
}

// IRAdminBypass configures a JWT role that bypasses tenant filtering.
type IRAdminBypass struct {
	Role string `yaml:"role" json:"role"`
}

// IRMetadata holds application identity fields.
type IRMetadata struct {
	Name        string `yaml:"name"        json:"name"`
	Version     string `yaml:"version"     json:"version"`
	Description string `yaml:"description" json:"description,omitempty"`
}

// IRResource describes a data resource in the spec.
type IRResource struct {
	Name       string    `yaml:"name"       json:"name"`
	Plural     string    `yaml:"plural"     json:"plural"`
	Fields     []IRField `yaml:"fields"     json:"fields"`
	Operations []string  `yaml:"operations" json:"operations"`
	Events     bool      `yaml:"events"     json:"events"`
}

// IRField is a single field within a resource.
type IRField struct {
	Name       string `yaml:"name"        json:"name"`
	Type       string `yaml:"type"        json:"type"`
	Required   bool   `yaml:"required"    json:"required"`
	MaxLength  int    `yaml:"max_length"  json:"max_length,omitempty"`
	Auto       string `yaml:"auto"        json:"auto,omitempty"`
	SoftDelete bool   `yaml:"soft_delete" json:"soft_delete,omitempty"`
}

// IRAPI holds REST and gRPC API configuration.
type IRAPI struct {
	REST *IRRESTConfig `yaml:"rest" json:"rest,omitempty"`
	GRPC *IRGRPCConfig `yaml:"grpc" json:"grpc,omitempty"`
}

// IRRESTConfig holds REST-specific options.
type IRRESTConfig struct {
	BasePath      string `yaml:"base_path"      json:"base_path"`
	VersionHeader bool   `yaml:"version_header" json:"version_header"`
}

// IRGRPCConfig holds gRPC-specific options.
type IRGRPCConfig struct {
	Enabled bool `yaml:"enabled" json:"enabled"`
	Port    int  `yaml:"port"    json:"port"`
}

// IRAuth holds authentication configuration.
type IRAuth struct {
	Type      string `yaml:"type"       json:"type"`
	JWKURL    string `yaml:"jwk_url"    json:"jwk_url,omitempty"`
	Required  bool   `yaml:"required"   json:"required"`
	AllowMock string `yaml:"allow_mock" json:"allow_mock,omitempty"`

	// OCM-specific fields (only used when Type == "ocm").
	// OCMURL is the Red Hat SSO / OCM OIDC realm base URL; the JWKS endpoint is
	// derived automatically as <ocm_url>/protocol/openid-connect/certs.
	OCMURL       string   `yaml:"ocm_url"      json:"ocm_url,omitempty"`
	JWKSOverride string   `yaml:"jwks_url"     json:"jwks_url,omitempty"`
	AllowedOrgs  []string `yaml:"allowed_orgs" json:"allowed_orgs,omitempty"`
	CacheTTL     string   `yaml:"cache_ttl"    json:"cache_ttl,omitempty"`
}

// IRDatabase holds database configuration.
type IRDatabase struct {
	Type       string `yaml:"type"       json:"type"`
	Migrations string `yaml:"migrations" json:"migrations"`
}

// IRObservability holds health check and metrics config.
type IRObservability struct {
	HealthCheck *IRHealthCheck `yaml:"health_check" json:"health_check,omitempty"`
	Metrics     *IRMetrics     `yaml:"metrics"      json:"metrics,omitempty"`
}

// IRHealthCheck holds health check endpoint configuration.
type IRHealthCheck struct {
	Port int    `yaml:"port" json:"port"`
	Path string `yaml:"path" json:"path"`
}

// IRMetrics holds Prometheus metrics endpoint configuration.
type IRMetrics struct {
	Port int    `yaml:"port" json:"port"`
	Path string `yaml:"path" json:"path"`
}

var (
	validComponents = map[string]bool{
		// rh-trex parity components (v1)
		"foundry-http":                true,
		"foundry-postgres":            true,
		"foundry-auth-jwt":            true,
		"foundry-auth-ocm":            true,
		"foundry-health":              true,
		"foundry-metrics":             true,
		"foundry-tenancy":             true,
		"foundry-logging":             true,
		"foundry-errortracker":        true,
		"foundry-errortracker-sentry": true,
	}
	validFieldTypes = map[string]bool{
		"string": true, "int": true, "float": true,
		"bool": true, "timestamp": true, "uuid": true,
	}
	validOperations = map[string]bool{
		"create": true, "read": true, "update": true, "delete": true, "list": true,
	}
	reKebab  = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	rePascal = regexp.MustCompile(`^[A-Z][a-zA-Z0-9]*$`)
	reSnake  = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	reSemver = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)
	reAppVer = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
)

// Validate performs semantic validation of a parsed IRSpec.
// It is called by the compiler after YAML/JSON parsing.
// Returns a list of all validation errors (never nil on success).
func Validate(spec *IRSpec) []error {
	var errs []error

	add := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}

	if spec.APIVersion != "foundry/v1" {
		add("apiVersion must be 'foundry/v1', got %q", spec.APIVersion)
	}
	if spec.Kind != "Application" {
		add("kind must be 'Application', got %q", spec.Kind)
	}

	// Metadata
	if !reKebab.MatchString(spec.Metadata.Name) {
		add("metadata.name must be kebab-case, got %q", spec.Metadata.Name)
	}
	if !reAppVer.MatchString(spec.Metadata.Version) {
		add("metadata.version must be semver (e.g. 1.0.0), got %q", spec.Metadata.Version)
	}

	// Components (SBOM)
	if len(spec.Components) == 0 {
		add("components block must list at least one component")
	}
	for name, ver := range spec.Components {
		if !validComponents[name] {
			add("unknown component %q — not in registry", name)
		}
		if !reSemver.MatchString(ver) {
			add("component %q version must be semver (e.g. v1.0.0), got %q", name, ver)
		}
	}

	// Resources
	resourceNames := map[string]bool{}
	for i, r := range spec.Resources {
		prefix := fmt.Sprintf("resources[%d](%s)", i, r.Name)
		if !rePascal.MatchString(r.Name) {
			add("%s: name must be PascalCase", prefix)
		}
		if !reKebab.MatchString(r.Plural) {
			add("%s: plural must be lowercase-kebab", prefix)
		}
		if resourceNames[r.Name] {
			add("%s: duplicate resource name", prefix)
		}
		resourceNames[r.Name] = true

		if len(r.Fields) == 0 {
			add("%s: must have at least one field", prefix)
		}
		fieldNames := map[string]bool{}
		softDeleteCount := 0
		for j, f := range r.Fields {
			fp := fmt.Sprintf("%s.fields[%d](%s)", prefix, j, f.Name)
			if !reSnake.MatchString(f.Name) {
				add("%s: name must be snake_case", fp)
			}
			if fieldNames[f.Name] {
				add("%s: duplicate field name", fp)
			}
			fieldNames[f.Name] = true
			if !validFieldTypes[f.Type] {
				add("%s: unknown type %q", fp, f.Type)
			}
			if f.MaxLength != 0 && f.Type != "string" {
				add("%s: max_length only applies to string fields", fp)
			}
			if f.Auto != "" && f.Auto != "created" && f.Auto != "updated" {
				add("%s: auto must be 'created' or 'updated'", fp)
			}
			if f.SoftDelete {
				softDeleteCount++
				if f.Type != "timestamp" {
					add("%s: soft_delete field must have type 'timestamp'", fp)
				}
			}
		}
		if softDeleteCount > 1 {
			add("%s: at most one soft_delete field is allowed", prefix)
		}

		if len(r.Operations) == 0 {
			add("%s: must list at least one operation", prefix)
		}
		seen := map[string]bool{}
		for _, op := range r.Operations {
			if !validOperations[op] {
				add("%s: unknown operation %q", prefix, op)
			}
			if seen[op] {
				add("%s: duplicate operation %q", prefix, op)
			}
			seen[op] = true
		}

		// events requires foundry-events component
		if r.Events && spec.Components["foundry-events"] == "" {
			add("%s: events:true requires foundry-events in components", prefix)
		}
	}

	// Auth cross-checks
	if spec.Auth != nil && spec.Auth.Type == "jwt" {
		if spec.Auth.JWKURL == "" {
			add("auth.jwk_url is required when auth.type is 'jwt'")
		}
		if spec.Components["foundry-auth-jwt"] == "" {
			add("auth.type=jwt requires foundry-auth-jwt in components")
		}
	}
	if spec.Auth != nil && spec.Auth.Type == "ocm" {
		if spec.Auth.OCMURL == "" && spec.Auth.JWKSOverride == "" {
			add("auth.type=ocm requires either auth.ocm_url or auth.jwks_url")
		}
		if spec.Components["foundry-auth-ocm"] == "" {
			add("auth.type=ocm requires foundry-auth-ocm in components")
		}
	}

	// gRPC cross-checks
	if spec.API != nil && spec.API.GRPC != nil && spec.API.GRPC.Enabled {
		if spec.Components["foundry-grpc"] == "" {
			add("api.grpc.enabled=true requires foundry-grpc in components")
		}
	}

	// Database cross-checks
	if spec.Database != nil {
		if spec.Components["foundry-postgres"] == "" {
			add("database block requires foundry-postgres in components")
		}
	}
	if len(spec.Resources) > 0 && spec.Database == nil {
		add("resources declared but no database block — add a database block")
	}

	// Graph cross-checks
	if spec.Graph != nil {
		if spec.Components["foundry-graph-age"] == "" && spec.Graph.Backend == "age" {
			add("graph.backend=age requires foundry-graph-age in components")
		}
		// Collect declared node labels for edge cross-referencing.
		nodeLabels := make(map[string]bool)
		for i, n := range spec.Graph.NodeTypes {
			if n.Label == "" {
				add("graph.node_types[%d]: label is required", i)
			} else {
				nodeLabels[n.Label] = true
			}
		}
		// Validate edge type labels and node references.
		for i, e := range spec.Graph.EdgeTypes {
			ep := fmt.Sprintf("graph.edge_types[%d](%s)", i, e.Label)
			if e.Label == "" {
				add("graph.edge_types[%d]: label is required", i)
			}
			if e.From == "" {
				add("%s: from is required", ep)
			} else if len(nodeLabels) > 0 && !nodeLabels[e.From] {
				add("%s: from=%q is not a declared node_type label", ep, e.From)
			}
			if e.To == "" {
				add("%s: to is required", ep)
			} else if len(nodeLabels) > 0 && !nodeLabels[e.To] {
				add("%s: to=%q is not a declared node_type label", ep, e.To)
			}
		}
	}

	// Events cross-checks
	if spec.Events != nil {
		switch spec.Events.Backend {
		case "kafka":
			if spec.Components["foundry-kafka"] == "" {
				add("events.backend=kafka requires foundry-kafka in components")
			}
		case "nats":
			if spec.Components["foundry-nats"] == "" {
				add("events.backend=nats requires foundry-nats in components")
			}
		case "redis-streams":
			if spec.Components["foundry-redis-streams"] == "" && spec.Components["foundry-redis"] == "" {
				add("events.backend=redis-streams requires foundry-redis-streams or foundry-redis in components")
			}
		}
	}

	// Authz cross-checks
	if spec.Authz != nil && spec.Authz.Backend == "spicedb" {
		if spec.Components["foundry-auth-spicedb"] == "" {
			add("authz.backend=spicedb requires foundry-auth-spicedb in components")
		}
	}

	// State cross-checks
	if spec.State != nil {
		if spec.Components["foundry-redis"] == "" {
			add("state block requires foundry-redis in components")
		}
	}

	// Bi-temporal tracking cross-checks
	if spec.BiTemporal != nil && spec.BiTemporal.Enabled {
		if spec.Components["foundry-temporal"] == "" {
			add("bi_temporal.enabled=true requires foundry-temporal in components")
		}
		if spec.Database == nil {
			add("bi_temporal requires a database block")
		}
	}

	// Workflows (Temporal.io) cross-checks
	if spec.Workflows != nil {
		if spec.Components["foundry-temporal"] == "" {
			add("workflows block requires foundry-temporal in components")
		}
		if spec.Workflows.Namespace == "" {
			add("workflows.namespace is required")
		}
		if spec.Workflows.WorkerQueue == "" {
			add("workflows.worker_queue is required")
		}
	}

	// Tenancy cross-checks
	if spec.Tenancy != nil {
		if spec.Components["foundry-tenancy"] == "" {
			add("tenancy block requires foundry-tenancy in components")
		}
		validTenancyStrategies := map[string]bool{
			"row": true, "schema": true, "database": true,
		}
		if spec.Tenancy.Strategy != "" && !validTenancyStrategies[spec.Tenancy.Strategy] {
			add("tenancy.strategy must be one of row/schema/database, got %q", spec.Tenancy.Strategy)
		}
		// tenancy.field is used directly as a SQL column identifier in DDL and queries;
		// it must be snake_case to prevent SQL injection via identifier interpolation.
		if spec.Tenancy.Field != "" && !reSnake.MatchString(spec.Tenancy.Field) {
			add("tenancy.field must be snake_case (e.g. org_id), got %q", spec.Tenancy.Field)
		}
	}

	// Services cross-checks — components referenced must be declared in the global SBOM.
	for si, svc := range spec.Services {
		sp := fmt.Sprintf("services[%d](%s)", si, svc.Name)
		for _, comp := range svc.Components {
			if spec.Components[comp] == "" {
				add("%s: component %q is not declared in the top-level components block", sp, comp)
			}
		}
	}

	// Hooks validation
	validHookPoints := map[string]bool{
		"pre-handler": true, "post-handler": true,
		"pre-db": true, "post-db": true,
		"pre-publish": true, "post-consume": true,
	}
	reHookImpl := regexp.MustCompile(`^hooks/[a-z][a-z0-9_/]*\.go$`)
	hookNames := map[string]bool{}
	for i, h := range spec.Hooks {
		hp := fmt.Sprintf("hooks[%d](%s)", i, h.Name)
		if !reKebab.MatchString(h.Name) {
			add("%s: name must be kebab-case", hp)
		}
		if hookNames[h.Name] {
			add("%s: duplicate hook name", hp)
		}
		hookNames[h.Name] = true
		if !validHookPoints[h.Point] {
			add("%s: unknown point %q", hp, h.Point)
		}
		if !reHookImpl.MatchString(h.Implementation) {
			add("%s: implementation must match hooks/*.go pattern, got %q", hp, h.Implementation)
		}
		if h.Topic != "" && h.Point != "pre-publish" && h.Point != "post-consume" {
			add("%s: topic is only valid for pre-publish or post-consume hooks", hp)
		}
		if len(h.Routes) > 0 && h.Point != "pre-handler" && h.Point != "post-handler" {
			add("%s: routes is only valid for pre-handler or post-handler hooks", hp)
		}
		if (h.Point == "pre-db" || h.Point == "post-db") && spec.Database == nil {
			add("%s: pre-db/post-db hooks require a database block", hp)
		}
	}

	return errs
}

// ToResourceDefinitions converts IRResource slice into spec.ResourceDefinition
// slice for use by the Application runtime.
func ToResourceDefinitions(resources []IRResource) []ResourceDefinition {
	out := make([]ResourceDefinition, len(resources))
	for i, r := range resources {
		fields := make([]FieldDefinition, len(r.Fields))
		for j, f := range r.Fields {
			fields[j] = FieldDefinition{
				Name:       f.Name,
				Type:       f.Type,
				Required:   f.Required,
				MaxLength:  f.MaxLength,
				Auto:       f.Auto,
				SoftDelete: f.SoftDelete,
			}
		}
		out[i] = ResourceDefinition{
			Name:       r.Name,
			Plural:     r.Plural,
			Fields:     fields,
			Operations: r.Operations,
			Events:     r.Events,
		}
	}
	return out
}

// LoadSchemaJSON reads and returns the embedded schema.json bytes.
// path is normally the path to this package's schema.json.
func LoadSchemaJSON(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read schema: %w", err)
	}
	// Sanity-check: must be valid JSON.
	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("schema.json is not valid JSON: %w", err)
	}
	return data, nil
}
