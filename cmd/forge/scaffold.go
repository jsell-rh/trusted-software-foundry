package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func scaffoldCmd() *cobra.Command {
	var (
		appName    string
		appVersion string
		resources  []string
		outputFile string
	)

	cmd := &cobra.Command{
		Use:   "scaffold",
		Short: "Generate a starter app.foundry.yaml for a new application",
		Long: `scaffold generates a valid app.foundry.yaml with the 7 core trusted components
pinned and placeholder resource definitions. AI agents use this as a starting
point when creating a new application — edit the YAML, then run forge compile.
Add advanced components (foundry-auth-spicedb, foundry-kafka, etc.) as needed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if appName == "" {
				return fmt.Errorf("--name is required")
			}

			spec := renderScaffold(appName, appVersion, resources)

			if outputFile == "" || outputFile == "-" {
				fmt.Print(spec)
				return nil
			}

			if err := os.WriteFile(outputFile, []byte(spec), 0644); err != nil {
				return fmt.Errorf("writing %q: %w", outputFile, err)
			}
			fmt.Printf("Scaffolded %q → %s\n", appName, outputFile)
			fmt.Println("Next: edit the spec, then run: forge compile", outputFile, "-o ./out/")
			return nil
		},
	}

	cmd.Flags().StringVar(&appName, "name", "", "Application name (required, e.g. my-service)")
	cmd.Flags().StringVar(&appVersion, "version", "1.0.0", "Application version")
	cmd.Flags().StringSliceVar(&resources, "resource", nil, "Resource names to scaffold (e.g. --resource User --resource Post)")
	cmd.Flags().StringVarP(&outputFile, "output", "o", "-", "Output file path (default: stdout)")

	return cmd
}

func renderScaffold(name, version string, resourceNames []string) string {
	var sb strings.Builder

	sb.WriteString("apiVersion: foundry/v1\n")
	sb.WriteString("kind: Application\n\n")
	sb.WriteString("metadata:\n")
	sb.WriteString("  name: " + name + "\n")
	sb.WriteString("  version: " + version + "\n")
	sb.WriteString("  description: >\n")
	sb.WriteString("    TODO: describe what " + name + " does.\n\n")

	sb.WriteString("# Trusted component versions (pinned). This block is the application SBOM.\n")
	sb.WriteString("components:\n")
	sb.WriteString("  foundry-http:     v1.0.0\n")
	sb.WriteString("  foundry-postgres: v1.0.0\n")
	sb.WriteString("  foundry-auth-jwt: v1.0.0\n")
	sb.WriteString("  foundry-grpc:     v1.0.0\n")
	sb.WriteString("  foundry-health:   v1.0.0\n")
	sb.WriteString("  foundry-metrics:  v1.0.0\n")
	sb.WriteString("  foundry-events:   v1.0.0\n\n")

	sb.WriteString("# Data resources — what the application stores and manages.\n")
	sb.WriteString("resources:\n")

	if len(resourceNames) == 0 {
		resourceNames = []string{"Item"}
	}
	for _, res := range resourceNames {
		plural := strings.ToLower(res) + "s"
		sb.WriteString("  - name: " + res + "\n")
		sb.WriteString("    plural: " + plural + "\n")
		sb.WriteString("    fields:\n")
		sb.WriteString("      - name: id\n")
		sb.WriteString("        type: uuid\n")
		sb.WriteString("        required: true\n")
		sb.WriteString("        auto: created\n")
		sb.WriteString("      - name: name\n")
		sb.WriteString("        type: string\n")
		sb.WriteString("        required: true\n")
		sb.WriteString("        max_length: 255\n")
		sb.WriteString("      - name: created_at\n")
		sb.WriteString("        type: timestamp\n")
		sb.WriteString("        auto: created\n")
		sb.WriteString("      - name: updated_at\n")
		sb.WriteString("        type: timestamp\n")
		sb.WriteString("        auto: updated\n")
		sb.WriteString("      - name: deleted_at\n")
		sb.WriteString("        type: timestamp\n")
		sb.WriteString("        soft_delete: true\n")
		sb.WriteString("    operations: [create, read, update, delete, list]\n")
		sb.WriteString("    events: true\n\n")
	}

	sb.WriteString("api:\n")
	sb.WriteString("  rest:\n")
	sb.WriteString("    base_path: /api/v1\n")
	sb.WriteString("    version_header: true\n")
	sb.WriteString("  grpc:\n")
	sb.WriteString("    enabled: true\n")
	sb.WriteString("    port: 9000\n\n")

	sb.WriteString("auth:\n")
	sb.WriteString("  type: jwt\n")
	sb.WriteString("  jwk_url: \"${JWK_CERT_URL}\"\n")
	sb.WriteString("  required: true\n")
	sb.WriteString("  allow_mock: \"${OCM_MOCK_ENABLED}\"\n\n")

	sb.WriteString("database:\n")
	sb.WriteString("  type: postgres\n")
	sb.WriteString("  migrations: auto\n\n")

	sb.WriteString("observability:\n")
	sb.WriteString("  health_check:\n")
	sb.WriteString("    port: 8083\n")
	sb.WriteString("    path: /healthz\n")
	sb.WriteString("  metrics:\n")
	sb.WriteString("    port: 8080\n")
	sb.WriteString("    path: /metrics\n")

	return sb.String()
}
