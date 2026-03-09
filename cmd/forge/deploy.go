package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jsell-rh/trusted-software-foundry/foundry/compiler"
)

func deployCmd() *cobra.Command {
	var outputDir string

	cmd := &cobra.Command{
		Use:   "deploy <spec.yaml>",
		Short: "Generate Kubernetes manifests for a Foundry spec",
		Long: `forge deploy reads a Foundry IR spec and generates production-ready Kubernetes
manifests in the deploy/ subdirectory of the output directory.

Generated manifests:
  deploy/<service>/deployment.yaml   — Deployment with health probes + resource limits
  deploy/<service>/service.yaml      — ClusterIP Service
  deploy/secrets.yaml                — Secret template (fill in base64 values)
  deploy/kustomization.yaml          — Kustomize overlay listing all resources

Apply with:
  kubectl apply -k deploy/

Or with kustomize:
  kustomize build deploy/ | kubectl apply -f -`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			specPath := args[0]

			if outputDir == "" {
				return fmt.Errorf("--output is required")
			}

			ir, err := compiler.Parse(specPath)
			if err != nil {
				return fmt.Errorf("parse: %w", err)
			}

			if err := compiler.Deploy(ir, outputDir); err != nil {
				return fmt.Errorf("deploy generation failed: %w", err)
			}

			fmt.Printf("Generated Kubernetes manifests for %q → %s/deploy/\n", specPath, outputDir)
			fmt.Printf("Apply with: kubectl apply -k %s/deploy/\n", outputDir)
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "Output directory (deploy/ is created inside it)")

	return cmd
}
