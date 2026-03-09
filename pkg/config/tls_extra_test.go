package config

// tls_extra_test.go adds coverage for TLSConfig and AddFlags methods.

import (
	"crypto/tls"
	"testing"

	"github.com/spf13/pflag"
)

// --------------------------------------------------------------------------
// TLSConfig.Validate — all branches
// --------------------------------------------------------------------------

func TestTLSConfig_Validate_Disabled(t *testing.T) {
	cfg := &TLSConfig{EnableTLS: false}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate with TLS disabled should return nil, got: %v", err)
	}
}

func TestTLSConfig_Validate_MissingCertKey(t *testing.T) {
	cfg := &TLSConfig{EnableTLS: true, CertFile: "", KeyFile: "", MinVersion: "1.2", MaxVersion: "1.3"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing cert/key files")
	}
}

func TestTLSConfig_Validate_InvalidMinVersion(t *testing.T) {
	cfg := &TLSConfig{EnableTLS: true, CertFile: "cert.pem", KeyFile: "key.pem", MinVersion: "invalid", MaxVersion: "1.3"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid min TLS version")
	}
}

func TestTLSConfig_Validate_InsecureMinVersion(t *testing.T) {
	cfg := &TLSConfig{EnableTLS: true, CertFile: "cert.pem", KeyFile: "key.pem", MinVersion: "1.0", MaxVersion: "1.3"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for insecure min TLS version (1.0)")
	}
}

func TestTLSConfig_Validate_InvalidMaxVersion(t *testing.T) {
	cfg := &TLSConfig{EnableTLS: true, CertFile: "cert.pem", KeyFile: "key.pem", MinVersion: "1.2", MaxVersion: "invalid"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid max TLS version")
	}
}

func TestTLSConfig_Validate_MinGreaterThanMax(t *testing.T) {
	cfg := &TLSConfig{EnableTLS: true, CertFile: "cert.pem", KeyFile: "key.pem", MinVersion: "1.3", MaxVersion: "1.2"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error when min > max TLS version")
	}
}

func TestTLSConfig_Validate_InsecureSkipVerify(t *testing.T) {
	cfg := &TLSConfig{
		EnableTLS: true, CertFile: "cert.pem", KeyFile: "key.pem",
		MinVersion: "1.2", MaxVersion: "1.3", InsecureSkipVerify: true,
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for InsecureSkipVerify=true")
	}
}

func TestTLSConfig_Validate_ClientAuthWithoutCA(t *testing.T) {
	cfg := &TLSConfig{
		EnableTLS: true, CertFile: "cert.pem", KeyFile: "key.pem",
		MinVersion: "1.2", MaxVersion: "1.3",
		EnableClientAuth: true, ClientCAFile: "",
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for client auth without CA file")
	}
}

func TestTLSConfig_Validate_HappyPath(t *testing.T) {
	cfg := &TLSConfig{
		EnableTLS: true, CertFile: "cert.pem", KeyFile: "key.pem",
		MinVersion: "1.2", MaxVersion: "1.3",
		EnableClientAuth: true, ClientCAFile: "ca.pem",
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error: %v", err)
	}
}

// --------------------------------------------------------------------------
// validateTLSVersion
// --------------------------------------------------------------------------

func TestValidateTLSVersion_Valid12(t *testing.T) {
	cfg := &TLSConfig{}
	if err := cfg.validateTLSVersion("1.2", "min"); err != nil {
		t.Errorf("1.2 should be valid: %v", err)
	}
}

func TestValidateTLSVersion_Valid13(t *testing.T) {
	cfg := &TLSConfig{}
	if err := cfg.validateTLSVersion("1.3", "max"); err != nil {
		t.Errorf("1.3 should be valid: %v", err)
	}
}

func TestValidateTLSVersion_Insecure10(t *testing.T) {
	cfg := &TLSConfig{}
	if err := cfg.validateTLSVersion("1.0", "min"); err == nil {
		t.Error("1.0 should be rejected as insecure")
	}
}

func TestValidateTLSVersion_Insecure11(t *testing.T) {
	cfg := &TLSConfig{}
	if err := cfg.validateTLSVersion("1.1", "min"); err == nil {
		t.Error("1.1 should be rejected as insecure")
	}
}

func TestValidateTLSVersion_Unknown(t *testing.T) {
	cfg := &TLSConfig{}
	if err := cfg.validateTLSVersion("2.0", "min"); err == nil {
		t.Error("unknown TLS version should return error")
	}
}

// --------------------------------------------------------------------------
// parseVersionToInt
// --------------------------------------------------------------------------

func TestParseVersionToInt(t *testing.T) {
	cfg := &TLSConfig{}
	cases := []struct {
		version string
		want    int
	}{
		{"1.2", 12},
		{"1.3", 13},
		{"unknown", 0},
	}
	for _, c := range cases {
		if got := cfg.parseVersionToInt(c.version); got != c.want {
			t.Errorf("parseVersionToInt(%q) = %d, want %d", c.version, got, c.want)
		}
	}
}

// --------------------------------------------------------------------------
// tlsVersionToInt
// --------------------------------------------------------------------------

func TestTLSVersionToInt(t *testing.T) {
	cfg := &TLSConfig{}
	if cfg.tlsVersionToInt("1.2") != tls.VersionTLS12 {
		t.Error("1.2 should map to tls.VersionTLS12")
	}
	if cfg.tlsVersionToInt("1.3") != tls.VersionTLS13 {
		t.Error("1.3 should map to tls.VersionTLS13")
	}
	if cfg.tlsVersionToInt("unknown") != 0 {
		t.Error("unknown version should map to 0")
	}
}

// --------------------------------------------------------------------------
// cipherSuiteByName
// --------------------------------------------------------------------------

func TestCipherSuiteByName_Known(t *testing.T) {
	cfg := &TLSConfig{}
	id := cfg.cipherSuiteByName("TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384")
	if id == 0 {
		t.Error("known cipher suite should return non-zero ID")
	}
}

func TestCipherSuiteByName_Unknown(t *testing.T) {
	cfg := &TLSConfig{}
	id := cfg.cipherSuiteByName("UNKNOWN_SUITE")
	if id != 0 {
		t.Errorf("unknown cipher suite should return 0, got %d", id)
	}
}

// --------------------------------------------------------------------------
// GetSecurityInfo
// --------------------------------------------------------------------------

func TestGetSecurityInfo_TLSDisabled(t *testing.T) {
	cfg := &TLSConfig{EnableTLS: false, MinVersion: "1.2", MaxVersion: "1.3"}
	info := cfg.GetSecurityInfo()
	if info["tls_enabled"] != false {
		t.Errorf("tls_enabled = %v, want false", info["tls_enabled"])
	}
	if _, ok := info["cert_file_configured"]; ok {
		t.Error("cert_file_configured should not be present when TLS is disabled")
	}
}

func TestGetSecurityInfo_TLSEnabled(t *testing.T) {
	cfg := &TLSConfig{EnableTLS: true, CertFile: "cert.pem", MinVersion: "1.2", MaxVersion: "1.3", AutoDetectKubernetes: false}
	info := cfg.GetSecurityInfo()
	if info["tls_enabled"] != true {
		t.Errorf("tls_enabled = %v, want true", info["tls_enabled"])
	}
	if _, ok := info["cert_file_configured"]; !ok {
		t.Error("cert_file_configured should be present when TLS is enabled")
	}
}

func TestGetSecurityInfo_AutoDetect(t *testing.T) {
	cfg := &TLSConfig{EnableTLS: false, AutoDetectKubernetes: true}
	info := cfg.GetSecurityInfo()
	if _, ok := info["kubernetes_environment"]; !ok {
		t.Error("kubernetes_environment should be present when auto-detect is enabled")
	}
}

// --------------------------------------------------------------------------
// applySecuritySettings
// --------------------------------------------------------------------------

func TestApplySecuritySettings_WithCipherSuites(t *testing.T) {
	cfg := &TLSConfig{
		MinVersion:          "1.2",
		MaxVersion:          "1.3",
		CipherSuites:        []string{"TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384"},
		InsecureSkipVerify:  false,
		PreferServerCiphers: true,
	}
	base := &tls.Config{}
	result := cfg.applySecuritySettings(base)
	if result.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want %d", result.MinVersion, tls.VersionTLS12)
	}
	if len(result.CipherSuites) == 0 {
		t.Error("CipherSuites should be populated")
	}
}

func TestApplySecuritySettings_EmptyCipherSuites(t *testing.T) {
	cfg := &TLSConfig{MinVersion: "1.2", MaxVersion: "1.3", CipherSuites: []string{}}
	base := &tls.Config{}
	result := cfg.applySecuritySettings(base)
	if result.CipherSuites != nil {
		t.Error("CipherSuites should remain nil for empty list")
	}
}

func TestApplySecuritySettings_UnknownCipherSuite(t *testing.T) {
	cfg := &TLSConfig{
		MinVersion:   "1.2",
		MaxVersion:   "1.3",
		CipherSuites: []string{"UNKNOWN_SUITE"},
	}
	base := &tls.Config{}
	result := cfg.applySecuritySettings(base)
	// Unknown suites produce ID=0 which is filtered out — CipherSuites stays nil.
	if result.CipherSuites != nil {
		t.Error("CipherSuites should be nil when all suites are unknown")
	}
}

// --------------------------------------------------------------------------
// BuildServerTLSConfig / BuildClientTLSConfig — TLS disabled path
// --------------------------------------------------------------------------

func TestBuildServerTLSConfig_Disabled(t *testing.T) {
	cfg := &TLSConfig{EnableTLS: false}
	tlsCfg, err := cfg.BuildServerTLSConfig()
	if err != nil {
		t.Fatalf("BuildServerTLSConfig with TLS disabled: %v", err)
	}
	if tlsCfg != nil {
		t.Error("BuildServerTLSConfig should return nil when TLS is disabled")
	}
}

func TestBuildClientTLSConfig_Disabled(t *testing.T) {
	cfg := &TLSConfig{EnableTLS: false}
	tlsCfg, err := cfg.BuildClientTLSConfig()
	if err != nil {
		t.Fatalf("BuildClientTLSConfig with TLS disabled: %v", err)
	}
	if tlsCfg != nil {
		t.Error("BuildClientTLSConfig should return nil when TLS is disabled")
	}
}

func TestBuildClientTLSConfig_Enabled_ManualConfig(t *testing.T) {
	// With AutoDetectKubernetes=false and TLS enabled, uses manual NewClientTLSConfig.
	cfg := &TLSConfig{
		EnableTLS:            true,
		AutoDetectKubernetes: false,
		InsecureSkipVerify:   true, // simplest way to avoid needing real cert files
		MinVersion:           "1.2",
		MaxVersion:           "1.3",
	}
	tlsCfg, err := cfg.BuildClientTLSConfig()
	if err != nil {
		t.Fatalf("BuildClientTLSConfig: %v", err)
	}
	if tlsCfg == nil {
		t.Error("BuildClientTLSConfig should return non-nil config")
	}
}

// --------------------------------------------------------------------------
// AddFlags — just verify they don't panic
// --------------------------------------------------------------------------

func TestTLSConfig_AddFlags(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewTLSConfig()
	cfg.AddFlags(fs, "")
	if fs.Lookup("enable-tls") == nil {
		t.Error("enable-tls flag should be registered")
	}
}

func TestTLSConfig_AddFlags_WithPrefix(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewTLSConfig()
	cfg.AddFlags(fs, "grpc")
	if fs.Lookup("grpc-enable-tls") == nil {
		t.Error("grpc-enable-tls flag should be registered")
	}
}

func TestApplicationConfig_AddFlags(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewApplicationConfig()
	cfg.AddFlags(fs)
	if fs.Lookup("api-server-bindaddress") == nil {
		t.Error("api-server-bindaddress flag should be registered")
	}
}

func TestDatabaseConfig_AddFlags(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewDatabaseConfig()
	cfg.AddFlags(fs)
	if fs.Lookup("db-host-file") == nil {
		t.Error("db-host-file flag should be registered")
	}
}

func TestServerConfig_AddFlags(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewServerConfig()
	cfg.AddFlags(fs)
	if fs.Lookup("api-server-bindaddress") == nil {
		t.Error("api-server-bindaddress flag should be registered")
	}
}

func TestMetricsConfig_AddFlags(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewMetricsConfig()
	cfg.AddFlags(fs)
	if fs.Lookup("metrics-server-bindaddress") == nil {
		t.Error("metrics-server-bindaddress flag should be registered")
	}
}

func TestHealthCheckConfig_AddFlags(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewHealthCheckConfig()
	cfg.AddFlags(fs)
	if fs.Lookup("health-check-server-bindaddress") == nil {
		t.Error("health-check-server-bindaddress flag should be registered")
	}
}

func TestGRPCConfig_AddFlags(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewGRPCConfig()
	cfg.AddFlags(fs)
	if fs.Lookup("enable-grpc") == nil {
		t.Error("enable-grpc flag should be registered")
	}
}

func TestAPIClientConfig_AddFlags(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewAPIClientConfig()
	cfg.AddFlags(fs)
	if fs.Lookup("api-base-url") == nil {
		t.Error("api-base-url flag should be registered")
	}
}

func TestAuthConfig_AddFlags(t *testing.T) {
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewAuthConfig()
	cfg.AddFlags(fs)
	if fs.Lookup("enable-jwt") == nil {
		t.Error("enable-jwt flag should be registered")
	}
}
