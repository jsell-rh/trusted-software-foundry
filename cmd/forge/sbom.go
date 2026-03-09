package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jsell-rh/trusted-software-foundry/foundry/compiler"
)

func sbomCmd() *cobra.Command {
	var outputDir string
	var stdout bool

	cmd := &cobra.Command{
		Use:   "sbom <spec.yaml>",
		Short: "Generate a CycloneDX Software Bill of Materials from a Foundry spec",
		Long: `forge sbom reads a Foundry IR spec and generates a CycloneDX 1.5 JSON SBOM
listing every trusted component pinned in the spec with its version, PURL, and role.

The SBOM enables:
  - Supply chain security audits
  - Vulnerability scanning against CVE databases
  - License compliance review
  - Sigstore/RHTAS attestation workflows

Output: <output>/sbom.cdx.json (or stdout with --stdout)`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			specPath := args[0]

			ir, err := compiler.Parse(specPath)
			if err != nil {
				return fmt.Errorf("parse: %w", err)
			}

			if stdout {
				buf, err := compiler.SBOMToWriter(ir)
				if err != nil {
					return fmt.Errorf("sbom generation failed: %w", err)
				}
				_, err = os.Stdout.Write(buf.Bytes())
				return err
			}

			if outputDir == "" {
				// Default: write next to the spec file.
				outputDir = filepath.Dir(specPath)
			}

			if err := compiler.SBOM(ir, outputDir); err != nil {
				return fmt.Errorf("sbom generation failed: %w", err)
			}

			dest := filepath.Join(outputDir, "sbom.cdx.json")
			fmt.Printf("Generated SBOM for %q → %s\n", specPath, dest)
			fmt.Printf("Components: %d trusted components pinned\n", len(ir.Components))
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "Output directory for sbom.cdx.json (default: spec file directory)")
	cmd.Flags().BoolVar(&stdout, "stdout", false, "Print SBOM to stdout instead of writing to file")

	return cmd
}
