// Package compiler implements the TSC compiler — it parses an IR spec (app.tsc.yaml),
// resolves trusted components from the registry, verifies audit hashes, and generates
// the minimal wiring code (main.go, go.mod, migrations/) needed to produce a working binary.
//
// AI agents interact ONLY with the IR spec. The compiler — not the agent — produces source code.
package compiler

// Spec is the top-level IR document (app.tsc.yaml).
type Spec struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`

	// Components is the SBOM — pinned component versions.
	// Keys are component names (e.g. "tsc-http"), values are semver strings (e.g. "v1.0.0").
	Components map[string]string `yaml:"components"`

	Resources   []Resource  `yaml:"resources"`
	API         APIConfig   `yaml:"api"`
	Auth        AuthConfig  `yaml:"auth"`
	Database    DBConfig    `yaml:"database"`
	Observability ObsConfig `yaml:"observability"`
}

// Metadata holds application identity fields.
type Metadata struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// Resource describes a data entity the application stores and manages.
type Resource struct {
	Name       string       `yaml:"name"`
	Plural     string       `yaml:"plural"`
	Fields     []Field      `yaml:"fields"`
	Operations []string     `yaml:"operations"`
	Events     bool         `yaml:"events"`
}

// Field describes a single field in a resource.
type Field struct {
	Name      string `yaml:"name"`
	Type      string `yaml:"type"`
	Required  bool   `yaml:"required"`
	MaxLength int    `yaml:"max_length,omitempty"`
	Auto      string `yaml:"auto,omitempty"`
	SoftDelete bool  `yaml:"soft_delete,omitempty"`
}

// APIConfig describes the API surface of the generated application.
type APIConfig struct {
	REST RESTConfig `yaml:"rest"`
	GRPC GRPCConfig `yaml:"grpc"`
}

// RESTConfig describes REST API settings.
type RESTConfig struct {
	BasePath      string `yaml:"base_path"`
	VersionHeader bool   `yaml:"version_header"`
}

// GRPCConfig describes gRPC settings.
type GRPCConfig struct {
	Enabled bool `yaml:"enabled"`
}

// AuthConfig describes authentication settings.
type AuthConfig struct {
	Type      string `yaml:"type"`
	JWKUrl    string `yaml:"jwk_url"`
	Required  bool   `yaml:"required"`
	AllowMock string `yaml:"allow_mock"`
}

// DBConfig describes database settings.
type DBConfig struct {
	Type       string `yaml:"type"`
	Migrations string `yaml:"migrations"`
}

// ObsConfig describes observability settings.
type ObsConfig struct {
	HealthCheck HealthCheckConfig `yaml:"health_check"`
	Metrics     MetricsConfig     `yaml:"metrics"`
}

// HealthCheckConfig describes the health check server.
type HealthCheckConfig struct {
	Port int `yaml:"port"`
}

// MetricsConfig describes Prometheus metrics.
type MetricsConfig struct {
	Port int    `yaml:"port"`
	Path string `yaml:"path"`
}
