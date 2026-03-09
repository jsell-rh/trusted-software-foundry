package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jsell-rh/trusted-software-foundry/foundry/compiler"
)

func verifyCmd() *cobra.Command {
	var (
		registryDir string
		sourceDir   string
	)

	cmd := &cobra.Command{
		Use:   "verify <spec.yaml>",
		Short: "Verify audit hashes of all trusted components in a Foundry spec",
		Long: `forge verify checks the integrity of every trusted component declared in the spec.

For each component it:
  1. Looks up the component in the registry to get the expected audit hash
  2. Computes the SHA-256 of the component source directory
  3. Compares the computed hash against the registry record

This is the tamper-detection layer of the Trusted Software Foundry. It guarantees
that the components you are compiling against are exactly the components that were
audited and approved.

Exit codes:
  0 — all components verified successfully
  1 — one or more components failed verification or are missing from the registry

Requires --source pointing to the component source directory tree.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			specPath := args[0]

			if sourceDir == "" {
				return fmt.Errorf("--source is required: path to the trusted-software-foundry checkout")
			}

			ir, err := compiler.Parse(specPath)
			if err != nil {
				return fmt.Errorf("parse: %w", err)
			}

			var registry compiler.Registry
			if registryDir != "" {
				registry = compiler.NewFileRegistry(registryDir)
			} else {
				registry = compiler.NewStubRegistry()
			}

			results := compiler.VerifyComponentMap(ir.Components, registry, sourceDir)

			allPassed := true
			fmt.Printf("Verifying %d components in %q:\n\n", len(results), ir.Metadata.Name)
			for _, r := range results {
				if r.Error != nil {
					fmt.Printf("  ✗ %-32s %s\n    error: %v\n", r.Name+"@"+r.Version, "FAIL", r.Error)
					allPassed = false
				} else {
					fmt.Printf("  ✓ %-32s %s  (sha256:%s)\n", r.Name+"@"+r.Version, "OK  ", r.Hash[:16]+"…")
				}
			}

			fmt.Println()
			if allPassed {
				fmt.Printf("All %d components verified. Supply chain integrity confirmed.\n", len(results))
				return nil
			}

			failed := 0
			for _, r := range results {
				if r.Error != nil {
					failed++
				}
			}
			fmt.Fprintf(os.Stderr, "%d of %d components failed verification.\n", failed, len(results))
			os.Exit(1)
			return nil
		},
	}

	cmd.Flags().StringVar(&registryDir, "registry", "", "Path to local component registry index directory")
	cmd.Flags().StringVar(&sourceDir, "source", "", "Path to component source directory (required)")
	_ = cmd.MarkFlagRequired("source")

	return cmd
}
