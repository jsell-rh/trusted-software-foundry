package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jsell-rh/trusted-software-foundry/foundry/compiler"
)

func deployCmd() *cobra.Command {
	var (
		outputDir string
		helmChart bool
	)

	cmd := &cobra.Command{
		Use:   "deploy <spec.yaml>",
		Short: "Generate Kubernetes manifests for a Foundry spec",
		Long: `forge deploy reads a Foundry IR spec and generates production-ready Kubernetes
manifests in the deploy/ subdirectory of the output directory.

Generated manifests (default — kustomize):
  deploy/<service>/deployment.yaml   — Deployment with health probes + resource limits
  deploy/<service>/service.yaml      — ClusterIP Service
  deploy/secrets.yaml                — Secret template (fill in base64 values)
  deploy/kustomization.yaml          — Kustomize overlay listing all resources

Apply with:
  kubectl apply -k deploy/

With --helm, generates a Helm chart instead:
  deploy/helm/<appName>/Chart.yaml
  deploy/helm/<appName>/values.yaml
  deploy/helm/<appName>/templates/

Install with:
  helm install <appName> deploy/helm/<appName>/`,
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

			if helmChart {
				if err := compiler.GenerateHelm(ir, outputDir); err != nil {
					return fmt.Errorf("helm chart generation failed: %w", err)
				}
				appName := ir.Metadata.Name
				fmt.Printf("Generated Helm chart for %q → %s/deploy/helm/%s/\n", specPath, outputDir, appName)
				fmt.Printf("Install with: helm install %s %s/deploy/helm/%s/\n", appName, outputDir, appName)
				return nil
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
	cmd.Flags().BoolVar(&helmChart, "helm", false, "Generate a Helm chart instead of kustomize manifests")

	return cmd
}
