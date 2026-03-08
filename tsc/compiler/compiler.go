package compiler

import (
	"fmt"
)

// Compiler orchestrates the full TSC compilation pipeline.
// It operates like a build tool (go build, cargo build) — not a code generator.
//
// Pipeline:
//  1. Parse + validate the IR spec (app.tsc.yaml) using the canonical spec validator
//  2. Resolve component entries from the trusted registry
//  3. Verify audit hashes (when a local source dir is configured)
//  4. Generate wiring code using the frozen spec.Application API
type Compiler struct {
	registry  Registry
	sourceDir string // optional; enables audit hash verification
}

// New creates a Compiler backed by the given registry.
// Pass a non-empty sourceDir to enable audit hash verification against local component source.
func New(registry Registry, sourceDir string) *Compiler {
	return &Compiler{
		registry:  registry,
		sourceDir: sourceDir,
	}
}

// Compile compiles specPath and writes the generated project into outputDir.
func (c *Compiler) Compile(specPath, outputDir string) error {
	// Step 1: Parse + validate IR spec
	ir, err := Parse(specPath)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	// Step 2 + 3: Resolve components, verify audit hashes
	resolver := NewResolver(c.registry, c.sourceDir)
	components, err := resolver.ResolveAll(ir.Components)
	if err != nil {
		return fmt.Errorf("resolve: %w", err)
	}

	// Step 4: Generate wiring code
	gen := NewGenerator(outputDir)
	if err := gen.Generate(ir, components); err != nil {
		return fmt.Errorf("generate: %w", err)
	}

	return nil
}
