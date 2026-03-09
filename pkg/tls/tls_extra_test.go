package tls

// tls_extra_test.go covers:
//   SecureTLSConfig
//   ValidateTLSConfig — no warnings, InsecureSkipVerify, old min version, weak cipher suites
//   GetTLSVersionString — all versions + unknown
//   PrintTLSInfo
//   NewClientTLSConfig — no CA file, nonexistent CA file
//   NewServerTLSConfig — empty cert/key, nonexistent files
//   NewMutualTLSConfig — propagates errors

import (
	"crypto/tls"
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// SecureTLSConfig
// --------------------------------------------------------------------------

func TestSecureTLSConfig_NotNil(t *testing.T) {
	cfg := SecureTLSConfig()
	if cfg == nil {
		t.Fatal("SecureTLSConfig returned nil")
	}
}

func TestSecureTLSConfig_MinVersion(t *testing.T) {
	cfg := SecureTLSConfig()
	if cfg.MinVersion < tls.VersionTLS12 {
		t.Errorf("MinVersion = %d, want >= TLS 1.2 (%d)", cfg.MinVersion, tls.VersionTLS12)
	}
}

func TestSecureTLSConfig_CipherSuites(t *testing.T) {
	cfg := SecureTLSConfig()
	if len(cfg.CipherSuites) == 0 {
		t.Error("expected non-empty cipher suite list")
	}
}

// --------------------------------------------------------------------------
// ValidateTLSConfig
// --------------------------------------------------------------------------

func TestValidateTLSConfig_SecureConfig_NoWarnings(t *testing.T) {
	cfg := SecureTLSConfig()
	warnings := ValidateTLSConfig(cfg)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for SecureTLSConfig, got: %v", warnings)
	}
}

func TestValidateTLSConfig_InsecureSkipVerify(t *testing.T) {
	cfg := SecureTLSConfig()
	cfg.InsecureSkipVerify = true
	warnings := ValidateTLSConfig(cfg)
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "InsecureSkipVerify") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected InsecureSkipVerify warning, got: %v", warnings)
	}
}

func TestValidateTLSConfig_LowMinVersion(t *testing.T) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS10}
	warnings := ValidateTLSConfig(cfg)
	if len(warnings) == 0 {
		t.Error("expected warning for TLS 1.0 min version")
	}
}

func TestValidateTLSConfig_WeakCipherSuite(t *testing.T) {
	cfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{tls.TLS_RSA_WITH_RC4_128_SHA},
	}
	warnings := ValidateTLSConfig(cfg)
	found := false
	for _, w := range warnings {
		if strings.Contains(w, "Weak cipher") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected weak cipher suite warning, got: %v", warnings)
	}
}

func TestValidateTLSConfig_3DES_WeakCipher(t *testing.T) {
	cfg := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA},
	}
	warnings := ValidateTLSConfig(cfg)
	if len(warnings) == 0 {
		t.Error("expected weak cipher suite warning for 3DES")
	}
}

// --------------------------------------------------------------------------
// GetTLSVersionString
// --------------------------------------------------------------------------

func TestGetTLSVersionString_TLS10(t *testing.T) {
	got := GetTLSVersionString(tls.VersionTLS10)
	if got != "TLS 1.0" {
		t.Errorf("GetTLSVersionString(TLS10) = %q, want TLS 1.0", got)
	}
}

func TestGetTLSVersionString_TLS11(t *testing.T) {
	got := GetTLSVersionString(tls.VersionTLS11)
	if got != "TLS 1.1" {
		t.Errorf("GetTLSVersionString(TLS11) = %q, want TLS 1.1", got)
	}
}

func TestGetTLSVersionString_TLS12(t *testing.T) {
	got := GetTLSVersionString(tls.VersionTLS12)
	if got != "TLS 1.2" {
		t.Errorf("GetTLSVersionString(TLS12) = %q, want TLS 1.2", got)
	}
}

func TestGetTLSVersionString_TLS13(t *testing.T) {
	got := GetTLSVersionString(tls.VersionTLS13)
	if got != "TLS 1.3" {
		t.Errorf("GetTLSVersionString(TLS13) = %q, want TLS 1.3", got)
	}
}

func TestGetTLSVersionString_Unknown(t *testing.T) {
	got := GetTLSVersionString(0xFFFF)
	if !strings.Contains(got, "Unknown") {
		t.Errorf("GetTLSVersionString(unknown) = %q, want to contain Unknown", got)
	}
}

// --------------------------------------------------------------------------
// PrintTLSInfo
// --------------------------------------------------------------------------

func TestPrintTLSInfo_ReturnsLines(t *testing.T) {
	cfg := SecureTLSConfig()
	info := PrintTLSInfo(cfg, "")
	if len(info) == 0 {
		t.Error("PrintTLSInfo should return non-empty info")
	}
}

func TestPrintTLSInfo_WithPrefix(t *testing.T) {
	cfg := SecureTLSConfig()
	info := PrintTLSInfo(cfg, "  ")
	for _, line := range info {
		if !strings.HasPrefix(line, "  ") {
			t.Errorf("line %q should start with prefix", line)
		}
	}
}

func TestPrintTLSInfo_InsecureIncludesWarning(t *testing.T) {
	cfg := SecureTLSConfig()
	cfg.InsecureSkipVerify = true
	info := PrintTLSInfo(cfg, "")
	found := false
	for _, line := range info {
		if strings.Contains(line, "Security warnings") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("PrintTLSInfo should include security warnings line, got: %v", info)
	}
}

// --------------------------------------------------------------------------
// NewClientTLSConfig
// --------------------------------------------------------------------------

func TestNewClientTLSConfig_NoCA(t *testing.T) {
	cfg, err := NewClientTLSConfig("example.com", "", false)
	if err != nil {
		t.Fatalf("NewClientTLSConfig (no CA): %v", err)
	}
	if cfg == nil {
		t.Fatal("NewClientTLSConfig returned nil")
	}
	if cfg.ServerName != "example.com" {
		t.Errorf("ServerName = %q, want example.com", cfg.ServerName)
	}
}

func TestNewClientTLSConfig_InsecureSkipVerify(t *testing.T) {
	cfg, err := NewClientTLSConfig("", "", true)
	if err != nil {
		t.Fatalf("NewClientTLSConfig (insecure): %v", err)
	}
	if !cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
}

func TestNewClientTLSConfig_NonExistentCAFile(t *testing.T) {
	_, err := NewClientTLSConfig("", "/nonexistent/ca.pem", false)
	if err == nil {
		t.Error("NewClientTLSConfig with nonexistent CA file should return error")
	}
}

// --------------------------------------------------------------------------
// NewServerTLSConfig
// --------------------------------------------------------------------------

func TestNewServerTLSConfig_EmptyCert(t *testing.T) {
	_, err := NewServerTLSConfig("", "key.pem")
	if err == nil {
		t.Error("NewServerTLSConfig with empty cert should return error")
	}
}

func TestNewServerTLSConfig_EmptyKey(t *testing.T) {
	_, err := NewServerTLSConfig("cert.pem", "")
	if err == nil {
		t.Error("NewServerTLSConfig with empty key should return error")
	}
}

func TestNewServerTLSConfig_NonExistentFiles(t *testing.T) {
	_, err := NewServerTLSConfig("/nonexistent/cert.pem", "/nonexistent/key.pem")
	if err == nil {
		t.Error("NewServerTLSConfig with nonexistent files should return error")
	}
}

// --------------------------------------------------------------------------
// NewMutualTLSConfig
// --------------------------------------------------------------------------

func TestNewMutualTLSConfig_EmptyCert(t *testing.T) {
	_, err := NewMutualTLSConfig("", "key.pem", "")
	if err == nil {
		t.Error("NewMutualTLSConfig with empty cert should return error")
	}
}

func TestNewMutualTLSConfig_NonExistentCAFile(t *testing.T) {
	// NewMutualTLSConfig first needs valid cert/key — we can only test the CA
	// path error if the server TLS config creation itself succeeds first.
	// Since we can't load nonexistent cert/key, the error surfaces from that:
	_, err := NewMutualTLSConfig("/nonexistent/cert.pem", "/nonexistent/key.pem", "/nonexistent/ca.pem")
	if err == nil {
		t.Error("expected error for nonexistent cert/key files")
	}
}
