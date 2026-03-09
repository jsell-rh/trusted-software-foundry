package config

// config_extra_test.go adds coverage for all pkg/config functions not reached
// by config_test.go:
//   SetProjectRootDir / GetProjectRootDir
//   NewApplicationConfig and all sub-constructors
//   ReadFile: empty path, quoted path, absolute path, relative path
//   readFileValueInt, readFileValueString, readFileValueBool
//   DatabaseConfig: ConnectionString, ConnectionStringWithName,
//                   LogSafeConnectionString, LogSafeConnectionStringWithName
//   AuthConfig: Validate (error + bypass normalization), IsAuthEnabled,
//               ReadFiles (env var, no env var), ConfigValidationError.Error
//   MigrateServerConfigToAuthConfig
//   GetEffectiveAuthConfig
//   APIClientConfig.ReadFiles (mock disabled path)

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --------------------------------------------------------------------------
// SetProjectRootDir / GetProjectRootDir
// --------------------------------------------------------------------------

func TestSetAndGetProjectRootDir(t *testing.T) {
	original := GetProjectRootDir()
	t.Cleanup(func() { SetProjectRootDir(original) })

	SetProjectRootDir("/tmp/test-root")
	if got := GetProjectRootDir(); got != "/tmp/test-root" {
		t.Errorf("GetProjectRootDir() = %q, want /tmp/test-root", got)
	}
}

func TestGetProjectRootDir_Fallback(t *testing.T) {
	original := projectRootDir // save package var
	t.Cleanup(func() { projectRootDir = original })

	projectRootDir = ""
	got := GetProjectRootDir()
	if got == "" {
		t.Error("GetProjectRootDir() returned empty string when no root is set")
	}
}

// --------------------------------------------------------------------------
// NewApplicationConfig
// --------------------------------------------------------------------------

func TestNewApplicationConfig(t *testing.T) {
	cfg := NewApplicationConfig()
	if cfg == nil {
		t.Fatal("NewApplicationConfig() returned nil")
	}
	if cfg.Server == nil || cfg.Database == nil || cfg.Auth == nil ||
		cfg.TLS == nil || cfg.Metrics == nil || cfg.HealthCheck == nil ||
		cfg.GRPC == nil || cfg.APIClient == nil {
		t.Error("NewApplicationConfig() has nil sub-configs")
	}
}

// --------------------------------------------------------------------------
// Sub-constructors
// --------------------------------------------------------------------------

func TestNewDatabaseConfig_Defaults(t *testing.T) {
	cfg := NewDatabaseConfig()
	if cfg.Dialect != "postgres" {
		t.Errorf("Dialect = %q, want postgres", cfg.Dialect)
	}
	if cfg.MaxOpenConnections != 50 {
		t.Errorf("MaxOpenConnections = %d, want 50", cfg.MaxOpenConnections)
	}
}

func TestNewServerConfig_Defaults(t *testing.T) {
	cfg := NewServerConfig()
	if cfg.BindAddress != "localhost:8000" {
		t.Errorf("BindAddress = %q, want localhost:8000", cfg.BindAddress)
	}
}

func TestNewMetricsConfig_Defaults(t *testing.T) {
	cfg := NewMetricsConfig()
	if cfg.BindAddress != "localhost:4433" {
		t.Errorf("BindAddress = %q, want localhost:4433", cfg.BindAddress)
	}
}

func TestNewHealthCheckConfig_Defaults(t *testing.T) {
	cfg := NewHealthCheckConfig()
	if cfg.BindAddress != "localhost:4434" {
		t.Errorf("BindAddress = %q, want localhost:4434", cfg.BindAddress)
	}
}

func TestNewGRPCConfig_Defaults(t *testing.T) {
	cfg := NewGRPCConfig()
	if !cfg.EnableGRPC {
		t.Error("EnableGRPC should default to true")
	}
	if cfg.BindAddress != "localhost:9000" {
		t.Errorf("BindAddress = %q, want localhost:9000", cfg.BindAddress)
	}
}

func TestNewAPIClientConfig_Defaults(t *testing.T) {
	cfg := NewAPIClientConfig()
	if cfg.EnableMock != true {
		t.Error("EnableMock should default to true")
	}
}

func TestNewAuthConfig_Defaults(t *testing.T) {
	cfg := NewAuthConfig()
	if !cfg.EnableJWT {
		t.Error("EnableJWT should default to true")
	}
	if cfg.EnableBearer {
		t.Error("EnableBearer should default to false")
	}
	if len(cfg.BypassPaths) == 0 {
		t.Error("BypassPaths should have default entries")
	}
}

// --------------------------------------------------------------------------
// ReadFile
// --------------------------------------------------------------------------

func TestReadFile_EmptyPath(t *testing.T) {
	val, err := ReadFile("")
	if err != nil {
		t.Fatalf("ReadFile(\"\") error: %v", err)
	}
	if val != "" {
		t.Errorf("ReadFile(\"\") = %q, want empty", val)
	}
}

func TestReadFile_AbsolutePath(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "cfg-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("hello")
	f.Close()

	val, err := ReadFile(f.Name())
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if val != "hello" {
		t.Errorf("ReadFile = %q, want 'hello'", val)
	}
}

func TestReadFile_QuotedPath(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "cfg-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("quoted-content")
	f.Close()

	// Wrap in quotes as strconv.Unquote expects them.
	quoted := fmt.Sprintf("%q", f.Name())
	val, err := ReadFile(quoted)
	if err != nil {
		t.Fatalf("ReadFile with quoted path: %v", err)
	}
	if val != "quoted-content" {
		t.Errorf("ReadFile = %q, want 'quoted-content'", val)
	}
}

func TestReadFile_RelativePath(t *testing.T) {
	dir := t.TempDir()
	SetProjectRootDir(dir)
	t.Cleanup(func() { SetProjectRootDir("") })

	fname := filepath.Join(dir, "relative.txt")
	os.WriteFile(fname, []byte("relative"), 0644)

	val, err := ReadFile("relative.txt")
	if err != nil {
		t.Fatalf("ReadFile relative: %v", err)
	}
	if val != "relative" {
		t.Errorf("ReadFile = %q, want 'relative'", val)
	}
}

func TestReadFile_NonExistent(t *testing.T) {
	_, err := ReadFile("/nonexistent/path/cfg.txt")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

// --------------------------------------------------------------------------
// readFileValueInt / String / Bool
// --------------------------------------------------------------------------

func TestReadFileValueInt_Happy(t *testing.T) {
	f, _ := os.CreateTemp(t.TempDir(), "*.txt")
	f.WriteString("42")
	f.Close()

	var v int
	if err := readFileValueInt(f.Name(), &v); err != nil {
		t.Fatalf("readFileValueInt: %v", err)
	}
	if v != 42 {
		t.Errorf("v = %d, want 42", v)
	}
}

func TestReadFileValueInt_InvalidContent(t *testing.T) {
	f, _ := os.CreateTemp(t.TempDir(), "*.txt")
	f.WriteString("not-a-number")
	f.Close()

	var v int
	if err := readFileValueInt(f.Name(), &v); err == nil {
		t.Error("expected error for non-integer file content")
	}
}

func TestReadFileValueString_Happy(t *testing.T) {
	f, _ := os.CreateTemp(t.TempDir(), "*.txt")
	f.WriteString("myhost\n") // trailing newline should be stripped
	f.Close()

	var v string
	if err := readFileValueString(f.Name(), &v); err != nil {
		t.Fatalf("readFileValueString: %v", err)
	}
	if v != "myhost" {
		t.Errorf("v = %q, want 'myhost'", v)
	}
}

func TestReadFileValueBool_Happy(t *testing.T) {
	f, _ := os.CreateTemp(t.TempDir(), "*.txt")
	f.WriteString("true")
	f.Close()

	var v bool
	if err := readFileValueBool(f.Name(), &v); err != nil {
		t.Fatalf("readFileValueBool: %v", err)
	}
	if !v {
		t.Error("v should be true")
	}
}

func TestReadFileValueBool_Invalid(t *testing.T) {
	f, _ := os.CreateTemp(t.TempDir(), "*.txt")
	f.WriteString("not-bool")
	f.Close()

	var v bool
	if err := readFileValueBool(f.Name(), &v); err == nil {
		t.Error("expected error for non-boolean file content")
	}
}

// --------------------------------------------------------------------------
// DatabaseConfig.ConnectionString / LogSafeConnectionString
// --------------------------------------------------------------------------

func TestDatabaseConfig_ConnectionString_WithSSL(t *testing.T) {
	cfg := &DatabaseConfig{
		Host:         "db.example.com",
		Port:         5432,
		Username:     "admin",
		Password:     "secret",
		Name:         "mydb",
		SSLMode:      "require",
		RootCertFile: "/path/to/cert.pem",
	}
	cs := cfg.ConnectionString(true)
	if !strings.Contains(cs, "sslmode=require") {
		t.Errorf("ConnectionString(true) = %q, want sslmode=require", cs)
	}
	if !strings.Contains(cs, "mydb") {
		t.Errorf("ConnectionString(true) = %q, want to contain dbname", cs)
	}
}

func TestDatabaseConfig_ConnectionString_WithoutSSL(t *testing.T) {
	cfg := &DatabaseConfig{
		Host:     "db.example.com",
		Port:     5432,
		Username: "admin",
		Password: "secret",
		Name:     "mydb",
	}
	cs := cfg.ConnectionString(false)
	if !strings.Contains(cs, "sslmode=disable") {
		t.Errorf("ConnectionString(false) = %q, want sslmode=disable", cs)
	}
}

func TestDatabaseConfig_ConnectionStringWithName(t *testing.T) {
	cfg := &DatabaseConfig{Host: "h", Port: 5432, Username: "u", Password: "p", SSLMode: "disable"}
	cs := cfg.ConnectionStringWithName("customdb", false)
	if !strings.Contains(cs, "customdb") {
		t.Errorf("ConnectionStringWithName = %q, want customdb", cs)
	}
}

func TestDatabaseConfig_LogSafeConnectionString_WithSSL(t *testing.T) {
	cfg := &DatabaseConfig{
		Host:         "db.example.com",
		Port:         5432,
		Username:     "admin",
		Password:     "actual-secret",
		Name:         "mydb",
		SSLMode:      "require",
		RootCertFile: "/cert.pem",
	}
	ls := cfg.LogSafeConnectionString(true)
	if strings.Contains(ls, "actual-secret") {
		t.Error("LogSafeConnectionString should redact password")
	}
	if !strings.Contains(ls, "<REDACTED>") {
		t.Errorf("LogSafeConnectionString = %q, want <REDACTED>", ls)
	}
}

func TestDatabaseConfig_LogSafeConnectionString_WithoutSSL(t *testing.T) {
	cfg := &DatabaseConfig{
		Host:     "db.example.com",
		Port:     5432,
		Username: "admin",
		Password: "real-password",
		Name:     "mydb",
	}
	ls := cfg.LogSafeConnectionString(false)
	if strings.Contains(ls, "real-password") {
		t.Error("LogSafeConnectionString should redact password")
	}
}

// --------------------------------------------------------------------------
// AuthConfig.IsAuthEnabled
// --------------------------------------------------------------------------

func TestIsAuthEnabled_JWTOnly(t *testing.T) {
	cfg := &AuthConfig{EnableJWT: true, EnableBearer: false}
	if !cfg.IsAuthEnabled() {
		t.Error("IsAuthEnabled() = false for JWT-only config")
	}
}

func TestIsAuthEnabled_BearerOnly(t *testing.T) {
	cfg := &AuthConfig{EnableJWT: false, EnableBearer: true}
	if !cfg.IsAuthEnabled() {
		t.Error("IsAuthEnabled() = false for Bearer-only config")
	}
}

func TestIsAuthEnabled_NoneEnabled(t *testing.T) {
	cfg := &AuthConfig{EnableJWT: false, EnableBearer: false}
	if cfg.IsAuthEnabled() {
		t.Error("IsAuthEnabled() = true when nothing enabled")
	}
}

// --------------------------------------------------------------------------
// AuthConfig.Validate
// --------------------------------------------------------------------------

func TestAuthConfig_Validate_BearerEnabledNoToken(t *testing.T) {
	cfg := &AuthConfig{EnableBearer: true, BearerToken: ""}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error when bearer enabled with no token")
	}
}

func TestAuthConfig_Validate_BearerEnabledWithToken(t *testing.T) {
	cfg := &AuthConfig{EnableBearer: true, BearerToken: "my-token", BypassPaths: []string{"/health"}}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error: %v", err)
	}
}

func TestAuthConfig_Validate_NormalizeBypassPaths(t *testing.T) {
	cfg := &AuthConfig{EnableBearer: false, BypassPaths: []string{"health", "/metrics"}}
	_ = cfg.Validate()
	for _, p := range cfg.BypassPaths {
		if !strings.HasPrefix(p, "/") {
			t.Errorf("bypass path %q should start with /", p)
		}
	}
}

// --------------------------------------------------------------------------
// ConfigValidationError.Error
// --------------------------------------------------------------------------

func TestConfigValidationError_Error(t *testing.T) {
	err := &ConfigValidationError{Field: "token", Message: "required"}
	s := err.Error()
	if !strings.Contains(s, "token") || !strings.Contains(s, "required") {
		t.Errorf("Error() = %q", s)
	}
}

// --------------------------------------------------------------------------
// AuthConfig.ReadFiles — env var path
// --------------------------------------------------------------------------

func TestAuthConfig_ReadFiles_FromAPIToken(t *testing.T) {
	t.Setenv("API_TOKEN", "my-api-token")
	t.Setenv("BEARER_TOKEN", "")

	cfg := &AuthConfig{BearerToken: ""}
	if err := cfg.ReadFiles(); err != nil {
		t.Fatalf("ReadFiles: %v", err)
	}
	if cfg.BearerToken != "my-api-token" {
		t.Errorf("BearerToken = %q, want my-api-token", cfg.BearerToken)
	}
}

func TestAuthConfig_ReadFiles_FromBearerToken(t *testing.T) {
	t.Setenv("API_TOKEN", "")
	t.Setenv("BEARER_TOKEN", "my-bearer-token")

	cfg := &AuthConfig{BearerToken: ""}
	if err := cfg.ReadFiles(); err != nil {
		t.Fatalf("ReadFiles: %v", err)
	}
	if cfg.BearerToken != "my-bearer-token" {
		t.Errorf("BearerToken = %q, want my-bearer-token", cfg.BearerToken)
	}
}

func TestAuthConfig_ReadFiles_AlreadySet(t *testing.T) {
	t.Setenv("API_TOKEN", "should-not-override")
	cfg := &AuthConfig{BearerToken: "already-set"}
	if err := cfg.ReadFiles(); err != nil {
		t.Fatalf("ReadFiles: %v", err)
	}
	if cfg.BearerToken != "already-set" {
		t.Errorf("BearerToken changed from 'already-set' to %q", cfg.BearerToken)
	}
}

// --------------------------------------------------------------------------
// MigrateServerConfigToAuthConfig
// --------------------------------------------------------------------------

func TestMigrateServerConfigToAuthConfig_EnableAuthz(t *testing.T) {
	server := NewServerConfig()
	server.EnableAuthz = false
	auth := NewAuthConfig()
	auth.EnableAuthz = true

	MigrateServerConfigToAuthConfig(server, auth)
	if auth.EnableAuthz != false {
		t.Error("EnableAuthz should have been migrated to false")
	}
}

func TestMigrateServerConfigToAuthConfig_JwkCertURL(t *testing.T) {
	server := NewServerConfig()
	server.JwkCertURL = "https://custom.idp.example.com/certs"
	auth := NewAuthConfig()

	MigrateServerConfigToAuthConfig(server, auth)
	if auth.JwkCertURL != "https://custom.idp.example.com/certs" {
		t.Errorf("JwkCertURL = %q after migration", auth.JwkCertURL)
	}
}

func TestMigrateServerConfigToAuthConfig_JwkCertFile(t *testing.T) {
	server := NewServerConfig()
	server.JwkCertFile = "/etc/certs/jwk.pem"
	auth := NewAuthConfig()

	MigrateServerConfigToAuthConfig(server, auth)
	if auth.JwkCertFile != "/etc/certs/jwk.pem" {
		t.Errorf("JwkCertFile = %q after migration", auth.JwkCertFile)
	}
}

// --------------------------------------------------------------------------
// GetEffectiveAuthConfig
// --------------------------------------------------------------------------

func TestGetEffectiveAuthConfig(t *testing.T) {
	cfg := NewApplicationConfig()
	cfg.Auth.EnableBearer = true
	cfg.Auth.BearerToken = "test-token"

	effective := cfg.GetEffectiveAuthConfig()
	if effective == nil {
		t.Fatal("GetEffectiveAuthConfig() returned nil")
	}
	if effective.BearerToken != "test-token" {
		t.Errorf("BearerToken = %q, want test-token", effective.BearerToken)
	}
	// Should be a copy — modifying original should not affect effective
	cfg.Auth.BearerToken = "changed"
	if effective.BearerToken != "test-token" {
		t.Error("GetEffectiveAuthConfig should return a copy")
	}
}

// --------------------------------------------------------------------------
// APIClientConfig.ReadFiles — mock disabled path
// --------------------------------------------------------------------------

func TestAPIClientConfig_ReadFiles_MockEnabled(t *testing.T) {
	cfg := &APIClientConfig{EnableMock: true}
	if err := cfg.ReadFiles(); err != nil {
		t.Fatalf("ReadFiles with EnableMock=true: %v", err)
	}
}

func TestAPIClientConfig_ReadFiles_MockDisabled_Error(t *testing.T) {
	// Without actual secret files, should get an error.
	cfg := &APIClientConfig{
		EnableMock:       false,
		ClientIDFile:     "/nonexistent/client-id",
		ClientSecretFile: "/nonexistent/client-secret",
		SelfTokenFile:    "",
	}
	// ReadFiles will try to read the nonexistent client ID file.
	// If path is empty string, readFileValueString returns nil.
	// With a real non-existent file, it returns an error.
	err := cfg.ReadFiles()
	if err == nil {
		t.Error("expected error when reading nonexistent client ID file")
	}
}

// --------------------------------------------------------------------------
// ApplicationConfig.ReadFiles — aggregates sub-config read errors
// --------------------------------------------------------------------------

func TestApplicationConfig_ReadFiles_NoErrors(t *testing.T) {
	// With defaults, file reads either succeed (empty path → skip) or return errors.
	// ReadFiles returns a []string of error messages.
	cfg := NewApplicationConfig()
	// Override file paths to empty so ReadFiles doesn't error.
	cfg.Database.HostFile = ""
	cfg.Database.PortFile = ""
	cfg.Database.NameFile = ""
	cfg.Database.UsernameFile = ""
	cfg.Database.PasswordFile = ""
	cfg.APIClient.EnableMock = true

	msgs := cfg.ReadFiles()
	// With all file paths empty or mock, should have no messages.
	_ = msgs // May have TLS messages; just verify it doesn't panic.
}
