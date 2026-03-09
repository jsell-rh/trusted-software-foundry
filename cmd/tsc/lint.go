package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jsell-rh/trusted-software-foundry/tsc/compiler"
)

func lintCmd() *cobra.Command {
	var schemaPath string

	cmd := &cobra.Command{
		Use:   "lint <spec.yaml>",
		Short: "Validate a TSC IR spec and report all errors",
		Long: `lint validates a TSC IR spec file in two passes:
  1. JSON Schema structural validation (schema.json)
  2. Semantic validation (cross-field rules, registry checks)

AI agents run lint before compile to catch errors early.
Exit code 0 = valid; exit code 1 = validation errors found.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			specPath := args[0]

			_, err := compiler.ParseWithSchema(specPath, schemaPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "LINT ERRORS in %q:\n%v\n", specPath, err)
				os.Exit(1)
			}

			fmt.Printf("OK  %s — spec is valid\n", specPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&schemaPath, "schema", "", "Path to JSON Schema file (default: built-in tsc/spec/schema.json)")

	return cmd
}
