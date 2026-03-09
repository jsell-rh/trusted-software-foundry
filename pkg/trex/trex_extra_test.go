package trex

// trex_extra_test.go covers:
//   IsInitialized (before Init)
//   Init with defaults (all empty fields)
//   Init with explicit values
//   GetConfig (after Init)
//   GetCORSOrigins — default origins, custom origins

import (
	"sync"
	"testing"
)

// resetState resets the global state so Init can be called again.
// Only used in testing — sync.Once is reset to allow re-initialization.
func resetState() {
	once = sync.Once{}
	initialized = false
	globalConfig = Config{}
}

func TestIsInitialized_FalseBeforeInit(t *testing.T) {
	resetState()
	if IsInitialized() {
		t.Error("IsInitialized() should be false before Init()")
	}
}

func TestInit_WithDefaults(t *testing.T) {
	resetState()
	Init(Config{}) // all empty — triggers all defaults
	if !IsInitialized() {
		t.Error("IsInitialized() should be true after Init()")
	}
	cfg := GetConfig()
	if cfg.ServiceName != "rh-trex-ai" {
		t.Errorf("ServiceName = %q, want rh-trex-ai", cfg.ServiceName)
	}
	if cfg.BasePath != "/api/rh-trex-ai/v1" {
		t.Errorf("BasePath = %q, want /api/rh-trex-ai/v1", cfg.BasePath)
	}
}

func TestInit_WithExplicitValues(t *testing.T) {
	resetState()
	Init(Config{
		ServiceName: "my-svc",
		BasePath:    "/api/my-svc/v1",
		ErrorHref:   "/api/my-svc/v1/errors/",
		MetadataID:  "my-meta",
	})
	cfg := GetConfig()
	if cfg.ServiceName != "my-svc" {
		t.Errorf("ServiceName = %q, want my-svc", cfg.ServiceName)
	}
	if cfg.MetadataID != "my-meta" {
		t.Errorf("MetadataID = %q, want my-meta", cfg.MetadataID)
	}
}

func TestInit_Idempotent(t *testing.T) {
	resetState()
	Init(Config{ServiceName: "first"})
	Init(Config{ServiceName: "second"}) // second call should be no-op
	cfg := GetConfig()
	if cfg.ServiceName != "first" {
		t.Errorf("ServiceName = %q after second Init, want first (sync.Once)", cfg.ServiceName)
	}
}

func TestGetCORSOrigins_DefaultOrigins(t *testing.T) {
	resetState()
	Init(Config{})
	origins := GetCORSOrigins()
	if len(origins) == 0 {
		t.Error("GetCORSOrigins() should return non-empty list")
	}
}

func TestGetCORSOrigins_CustomOrigins(t *testing.T) {
	resetState()
	Init(Config{
		CORSOrigins: []string{"https://custom.example.com"},
	})
	origins := GetCORSOrigins()
	if len(origins) != 1 || origins[0] != "https://custom.example.com" {
		t.Errorf("GetCORSOrigins() = %v, want [https://custom.example.com]", origins)
	}
}

func TestInit_WithProjectRootDir(t *testing.T) {
	resetState()
	Init(Config{ProjectRootDir: "/tmp/test-root"})
	// Should not panic.
	cfg := GetConfig()
	if cfg.ProjectRootDir != "/tmp/test-root" {
		t.Errorf("ProjectRootDir = %q, want /tmp/test-root", cfg.ProjectRootDir)
	}
}

func TestInit_MetadataIDDefaultsToServiceName(t *testing.T) {
	resetState()
	Init(Config{ServiceName: "foo-svc"})
	cfg := GetConfig()
	if cfg.MetadataID != "foo-svc" {
		t.Errorf("MetadataID = %q, want foo-svc (defaults to ServiceName)", cfg.MetadataID)
	}
}

func TestInit_ErrorHrefDefaultsToBasePath(t *testing.T) {
	resetState()
	Init(Config{BasePath: "/api/v2"})
	cfg := GetConfig()
	if cfg.ErrorHref != "/api/v2/errors/" {
		t.Errorf("ErrorHref = %q, want /api/v2/errors/", cfg.ErrorHref)
	}
}
