package compiler

// hook_callsite_test.go tests the hookCallSite function for all lifecycle
// hook point types, including the paths not covered by the E2E codegen tests.

import (
	"strings"
	"testing"

	"github.com/jsell-rh/trusted-software-foundry/foundry/spec"
)

func TestHookCallSite_PrePublish(t *testing.T) {
	h := spec.IRHook{Name: "validator", Point: "pre-publish"}

	sig := hookCallSite(h, "sig")
	if !strings.Contains(sig, "*foundry.EventMessage") {
		t.Errorf("pre-publish sig should reference *foundry.EventMessage, got: %q", sig)
	}

	call := hookCallSite(h, "call")
	if call != "hctx, msg" {
		t.Errorf("pre-publish call = %q, want %q", call, "hctx, msg")
	}
}

func TestHookCallSite_PostConsume(t *testing.T) {
	h := spec.IRHook{Name: "consumer", Point: "post-consume"}

	sig := hookCallSite(h, "sig")
	if !strings.Contains(sig, "*foundry.ConsumedEvent") {
		t.Errorf("post-consume sig should reference *foundry.ConsumedEvent, got: %q", sig)
	}

	call := hookCallSite(h, "call")
	if call != "hctx, event" {
		t.Errorf("post-consume call = %q, want %q", call, "hctx, event")
	}
}

func TestHookCallSite_Default(t *testing.T) {
	// An unrecognised point falls back to the minimal HookContext-only signature.
	// This is a safety net in case the validator is bypassed.
	h := spec.IRHook{Name: "custom", Point: "unknown-point"}

	sig := hookCallSite(h, "sig")
	if sig != "hctx *foundry.HookContext" {
		t.Errorf("default sig = %q, want %q", sig, "hctx *foundry.HookContext")
	}

	call := hookCallSite(h, "call")
	if call != "hctx" {
		t.Errorf("default call = %q, want %q", call, "hctx")
	}
}

func TestHookCallSite_KnownPoints_Coverage(t *testing.T) {
	// Table-driven coverage of all six declared lifecycle points.
	tests := []struct {
		point   string
		sigSnip string // substring expected in sig
		call    string // exact call args
	}{
		{"pre-handler", "http.ResponseWriter", "hctx, w, r"},
		{"post-handler", "*foundry.PostHandlerRequest", "hctx, req"},
		{"pre-db", "*foundry.DBOperation", "hctx, op"},
		{"post-db", "*foundry.DBResult", "hctx, result"},
		{"pre-publish", "*foundry.EventMessage", "hctx, msg"},
		{"post-consume", "*foundry.ConsumedEvent", "hctx, event"},
	}
	for _, tc := range tests {
		t.Run(tc.point, func(t *testing.T) {
			h := spec.IRHook{Name: "h", Point: tc.point}
			sig := hookCallSite(h, "sig")
			if !strings.Contains(sig, tc.sigSnip) {
				t.Errorf("%s sig = %q, want it to contain %q", tc.point, sig, tc.sigSnip)
			}
			call := hookCallSite(h, "call")
			if call != tc.call {
				t.Errorf("%s call = %q, want %q", tc.point, call, tc.call)
			}
		})
	}
}
