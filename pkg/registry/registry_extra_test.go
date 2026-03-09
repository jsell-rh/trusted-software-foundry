package registry

// registry_extra_test.go covers:
//   RegisterService, LoadDiscoveredServices

import (
	"testing"
)

// --------------------------------------------------------------------------
// Mock ServicesInterface
// --------------------------------------------------------------------------

type mockServices struct {
	services map[string]interface{}
}

func (m *mockServices) SetService(name string, service interface{}) {
	m.services[name] = service
}

// --------------------------------------------------------------------------
// RegisterService / LoadDiscoveredServices
// --------------------------------------------------------------------------

func TestRegisterService_AndLoad(t *testing.T) {
	// Use a unique name to avoid collisions with other tests.
	const svcName = "test-service-unique-123"

	RegisterService(svcName, func(env interface{}) interface{} {
		return "my-service-instance"
	})

	ms := &mockServices{services: make(map[string]interface{})}
	LoadDiscoveredServices(ms, nil)

	val, ok := ms.services[svcName]
	if !ok {
		t.Fatalf("expected service %q to be loaded", svcName)
	}
	if val != "my-service-instance" {
		t.Errorf("service value = %v, want my-service-instance", val)
	}
}

func TestRegisterService_Overwrite(t *testing.T) {
	const svcName = "overwrite-service-456"

	RegisterService(svcName, func(env interface{}) interface{} { return "v1" })
	RegisterService(svcName, func(env interface{}) interface{} { return "v2" })

	ms := &mockServices{services: make(map[string]interface{})}
	LoadDiscoveredServices(ms, nil)

	val := ms.services[svcName]
	if val != "v2" {
		t.Errorf("overwritten service = %v, want v2", val)
	}
}

func TestLoadDiscoveredServices_PassesEnv(t *testing.T) {
	const svcName = "env-service-789"
	var capturedEnv interface{}

	RegisterService(svcName, func(env interface{}) interface{} {
		capturedEnv = env
		return "ok"
	})

	ms := &mockServices{services: make(map[string]interface{})}
	LoadDiscoveredServices(ms, "my-env")

	if capturedEnv != "my-env" {
		t.Errorf("captured env = %v, want my-env", capturedEnv)
	}
}
