package grpc

// grpc_test.go provides coverage for the foundry-grpc component:
//   New, Name, Version, AuditHash, Configure, Register, AddPreAuthInterceptor,
//   Start, Stop, serverOptions, recoveryInterceptor, loggingInterceptor.

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

// --------------------------------------------------------------------------
// Constructor and accessors
// --------------------------------------------------------------------------

func TestNew_Defaults(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("New() returned nil")
	}
	if c.listenAddr != ":9000" {
		t.Errorf("default listenAddr = %q, want :9000", c.listenAddr)
	}
	if c.maxRecvMB != 4 {
		t.Errorf("default maxRecvMB = %d, want 4", c.maxRecvMB)
	}
	if c.maxSendMB != 4 {
		t.Errorf("default maxSendMB = %d, want 4", c.maxSendMB)
	}
}

func TestName(t *testing.T) {
	if got := New().Name(); got != "foundry-grpc" {
		t.Errorf("Name() = %q, want foundry-grpc", got)
	}
}

func TestVersion(t *testing.T) {
	if got := New().Version(); got != "v1.0.0" {
		t.Errorf("Version() = %q, want v1.0.0", got)
	}
}

func TestAuditHash(t *testing.T) {
	got := New().AuditHash()
	if len(got) == 0 {
		t.Error("AuditHash() should not be empty")
	}
}

// --------------------------------------------------------------------------
// Configure
// --------------------------------------------------------------------------

func TestConfigure_Empty(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{}); err != nil {
		t.Fatalf("Configure({}): %v", err)
	}
}

func TestConfigure_SetListenAddr(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"listen_addr": ":19090"}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.listenAddr != ":19090" {
		t.Errorf("listenAddr = %q, want :19090", c.listenAddr)
	}
}

func TestConfigure_EmptyListenAddrIgnored(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"listen_addr": ""}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.listenAddr != ":9000" {
		t.Errorf("listenAddr = %q, want default :9000", c.listenAddr)
	}
}

func TestConfigure_SetMaxSizes(t *testing.T) {
	c := New()
	if err := c.Configure(spec.ComponentConfig{"max_recv_mb": 16, "max_send_mb": 32}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.maxRecvMB != 16 {
		t.Errorf("maxRecvMB = %d, want 16", c.maxRecvMB)
	}
	if c.maxSendMB != 32 {
		t.Errorf("maxSendMB = %d, want 32", c.maxSendMB)
	}
}

func TestConfigure_MaxSizeZeroIgnored(t *testing.T) {
	c := New()
	c.maxRecvMB = 8
	if err := c.Configure(spec.ComponentConfig{"max_recv_mb": 0}); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.maxRecvMB != 8 {
		t.Errorf("maxRecvMB = %d, want 8 (unchanged)", c.maxRecvMB)
	}
}

func TestConfigure_SetTLSCertAndKey(t *testing.T) {
	c := New()
	cfg := spec.ComponentConfig{
		"tls_cert_file": "/path/to/cert.pem",
		"tls_key_file":  "/path/to/key.pem",
	}
	if err := c.Configure(cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.tlsCertFile != "/path/to/cert.pem" {
		t.Errorf("tlsCertFile = %q", c.tlsCertFile)
	}
}

func TestConfigure_TLSMismatch_OnlyCert(t *testing.T) {
	// Only tls_cert_file without tls_key_file → error.
	c := New()
	err := c.Configure(spec.ComponentConfig{"tls_cert_file": "/cert.pem"})
	if err == nil {
		t.Fatal("expected error for TLS cert without key, got nil")
	}
	if !strings.Contains(err.Error(), "tls_cert_file and tls_key_file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConfigure_TLSMismatch_OnlyKey(t *testing.T) {
	// Only tls_key_file without tls_cert_file → error.
	c := New()
	err := c.Configure(spec.ComponentConfig{"tls_key_file": "/key.pem"})
	if err == nil {
		t.Fatal("expected error for TLS key without cert, got nil")
	}
	if !strings.Contains(err.Error(), "tls_cert_file and tls_key_file") {
		t.Errorf("unexpected error: %v", err)
	}
}

// --------------------------------------------------------------------------
// Register
// --------------------------------------------------------------------------

func TestRegister_StoresApp(t *testing.T) {
	c := New()
	app := spec.NewApplication(nil)
	if err := c.Register(app); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if c.app != app {
		t.Error("Register did not store the application reference")
	}
}

// --------------------------------------------------------------------------
// AddPreAuthInterceptor
// --------------------------------------------------------------------------

func TestAddPreAuthInterceptor(t *testing.T) {
	c := New()
	interceptor := func(
		ctx context.Context, req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		return handler(ctx, req)
	}

	c.AddPreAuthInterceptor(interceptor)
	c.AddPreAuthInterceptor(interceptor)

	if len(c.preAuthInterceptors) != 2 {
		t.Errorf("expected 2 pre-auth interceptors, got %d", len(c.preAuthInterceptors))
	}
}

// --------------------------------------------------------------------------
// Start / Stop — lifecycle
// --------------------------------------------------------------------------

func randomPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port
}

func TestStart_HappyPath(t *testing.T) {
	app := spec.NewApplication(nil)
	c := New()
	port := randomPort(t)
	c.listenAddr = fmt.Sprintf(":%d", port)

	if err := c.Register(app); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = c.Stop(context.Background()) }()
}

func TestStart_WithPreAuthInterceptor(t *testing.T) {
	app := spec.NewApplication(nil)
	c := New()
	port := randomPort(t)
	c.listenAddr = fmt.Sprintf(":%d", port)

	c.AddPreAuthInterceptor(func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		return handler(ctx, req)
	})

	if err := c.Register(app); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start with pre-auth interceptor: %v", err)
	}
	defer func() { _ = c.Stop(context.Background()) }()
}

func TestStart_InvalidPortFails(t *testing.T) {
	app := spec.NewApplication(nil)
	c := New()
	// Use a privileged port that should fail (unless running as root).
	c.listenAddr = ":1" // requires root

	if err := c.Register(app); err != nil {
		t.Fatal(err)
	}

	if err := c.Start(context.Background()); err == nil {
		// If somehow port 1 is available (e.g., running as root), skip the check.
		t.Skip("port :1 is available — cannot test error path")
	} else {
		if !strings.Contains(err.Error(), "listen on") {
			t.Errorf("expected 'listen on' in error, got: %v", err)
		}
		_ = c.Stop(context.Background())
	}
}

func TestStart_NonServiceDescError(t *testing.T) {
	app := spec.NewApplication(nil)
	// Register a service entry with a Desc that is NOT *grpc.ServiceDesc.
	app.AddGRPCService("not-a-service-desc", struct{}{})

	c := New()
	port := randomPort(t)
	c.listenAddr = fmt.Sprintf(":%d", port)

	if err := c.Register(app); err != nil {
		t.Fatal(err)
	}

	err := c.Start(context.Background())
	if err == nil {
		t.Fatal("expected error for non-*grpc.ServiceDesc, got nil")
	}
	if !strings.Contains(err.Error(), "GRPCServiceDesc must be *grpc.ServiceDesc") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestStop_NilServer(t *testing.T) {
	c := New()
	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop(nil server): %v", err)
	}
}

func TestStop_GracefulShutdown(t *testing.T) {
	app := spec.NewApplication(nil)
	c := New()
	port := randomPort(t)
	c.listenAddr = fmt.Sprintf(":%d", port)

	if err := c.Register(app); err != nil {
		t.Fatal(err)
	}
	if err := c.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	if err := c.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

// --------------------------------------------------------------------------
// recoveryInterceptor — panic recovery
// --------------------------------------------------------------------------

func TestRecoveryInterceptor_PanickingHandler(t *testing.T) {
	interceptor := recoveryInterceptor()

	info := &grpc.UnaryServerInfo{FullMethod: "/test.TestService/PanickingRPC"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		panic("test panic")
	}

	resp, err := interceptor(context.Background(), nil, info, handler)
	if resp != nil {
		t.Errorf("expected nil response after panic, got: %v", resp)
	}
	if err == nil {
		t.Fatal("expected error after panic, got nil")
	}
	// Should be an Internal gRPC status code.
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got: %T: %v", err, err)
	}
	if st.Code() != codes.Internal {
		t.Errorf("code = %v, want Internal", st.Code())
	}
}

func TestRecoveryInterceptor_NormalHandler(t *testing.T) {
	interceptor := recoveryInterceptor()

	info := &grpc.UnaryServerInfo{FullMethod: "/test.TestService/NormalRPC"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "response", nil
	}

	resp, err := interceptor(context.Background(), nil, info, handler)
	if err != nil {
		t.Errorf("unexpected error from normal handler: %v", err)
	}
	if resp != "response" {
		t.Errorf("resp = %v, want response", resp)
	}
}

// --------------------------------------------------------------------------
// loggingInterceptor — method/duration logging
// --------------------------------------------------------------------------

func TestLoggingInterceptor_Success(t *testing.T) {
	interceptor := loggingInterceptor()

	info := &grpc.UnaryServerInfo{FullMethod: "/test.TestService/SuccessRPC"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	}

	resp, err := interceptor(context.Background(), nil, info, handler)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Errorf("resp = %v, want ok", resp)
	}
}

func TestLoggingInterceptor_HandlerError(t *testing.T) {
	interceptor := loggingInterceptor()

	info := &grpc.UnaryServerInfo{FullMethod: "/test.TestService/FailRPC"}
	want := errors.New("handler error")
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, want
	}

	_, err := interceptor(context.Background(), nil, info, handler)
	if err != want {
		t.Errorf("error = %v, want %v", err, want)
	}
}

func TestLoggingInterceptor_GRPCStatusError(t *testing.T) {
	interceptor := loggingInterceptor()

	info := &grpc.UnaryServerInfo{FullMethod: "/test.TestService/NotFoundRPC"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, status.Errorf(codes.NotFound, "not found")
	}

	_, err := interceptor(context.Background(), nil, info, handler)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("code = %v, want NotFound", st.Code())
	}
}
