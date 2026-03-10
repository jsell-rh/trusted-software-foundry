// forge is the Trusted Software Foundry compiler CLI.
//
// Usage:
//
//	forge compile <spec.yaml> --output <dir> [--registry <dir>] [--source <dir>]
//
// The compiler reads an IR spec (app.foundry.yaml), resolves trusted components from
// the registry, verifies audit hashes, and generates the minimal wiring code needed
// to produce a working binary via `go build`.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jsell-rh/trusted-software-foundry/foundry/compiler"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "forge",
		Short: "Trusted Software Foundry compiler",
		Long: `forge compiles a Trusted Software Foundry IR spec (app.foundry.yaml) into a deployable Go application.

AI agents write the IR spec. The compiler produces the source code.
All generated code is assembled from pre-audited, version-pinned trusted components.`,
	}
	cmd.AddCommand(compileCmd())
	cmd.AddCommand(initCmd())
	cmd.AddCommand(scaffoldCmd())
	cmd.AddCommand(lintCmd())
	cmd.AddCommand(explainCmd())
	cmd.AddCommand(diffCmd())
	cmd.AddCommand(deployCmd())
	cmd.AddCommand(sbomCmd())
	cmd.AddCommand(verifyCmd())
	return cmd
}

func compileCmd() *cobra.Command {
	var (
		outputDir   string
		registryDir string
		sourceDir   string
		foundryPath string
	)

	cmd := &cobra.Command{
		Use:   "compile <spec.yaml>",
		Short: "Compile a Foundry IR spec into a Go application",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			specPath := args[0]

			if outputDir == "" {
				return fmt.Errorf("--output is required")
			}

			var registry compiler.Registry
			if registryDir != "" {
				registry = compiler.NewFileRegistry(registryDir)
			} else {
				// Use the embedded stub registry for development.
				// Production builds will require --registry pointing to the signed catalog.
				registry = compiler.NewStubRegistry()
			}

			c := compiler.New(registry, sourceDir, foundryPath)
			if err := c.Compile(specPath, outputDir); err != nil {
				return fmt.Errorf("compilation failed: %w", err)
			}

			fmt.Printf("Compiled %q → %s\n", specPath, outputDir)
			if foundryPath != "" {
				fmt.Println("Run: cd", outputDir, "&& go build -o app .")
			} else {
				fmt.Println("Run: cd", outputDir, "&& go build -o app . (ensure trusted-software-foundry is published or use --foundry-path)")
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "Output directory for generated project (required)")
	cmd.Flags().StringVar(&registryDir, "registry", "", "Path to local component registry index directory")
	cmd.Flags().StringVar(&sourceDir, "source", "", "Path to component source directory (enables audit hash verification)")
	cmd.Flags().StringVar(&foundryPath, "foundry-path", "", "Absolute path to local trusted-software-foundry checkout (adds replace directive to generated go.mod)")

	return cmd
}
