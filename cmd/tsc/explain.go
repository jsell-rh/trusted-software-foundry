package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openshift-online/rh-trex-ai/tsc/compiler"
)

func explainCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "explain <spec.yaml>",
		Short: "Print a human-readable summary of what a TSC spec will build",
		Long: `explain parses a TSC IR spec and prints a structured summary:
  - Application identity (name, version)
  - Trusted component SBOM (name + version)
  - Resources: fields, operations, events
  - API surface (REST base path, gRPC port)
  - Auth, database, observability settings

AI agents use explain to verify their edits produced the intended application
before running tsc compile.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			specPath := args[0]

			ir, err := compiler.Parse(specPath)
			if err != nil {
				return fmt.Errorf("parsing spec: %w", err)
			}

			fmt.Printf("Application: %s v%s\n", ir.Metadata.Name, ir.Metadata.Version)
			if ir.Metadata.Description != "" {
				desc := strings.TrimSpace(ir.Metadata.Description)
				if len(desc) > 120 {
					desc = desc[:117] + "..."
				}
				fmt.Printf("Description: %s\n", desc)
			}

			fmt.Printf("\nComponents (%d):\n", len(ir.Components))
			for name, version := range ir.Components {
				fmt.Printf("  %-20s %s\n", name, version)
			}

			fmt.Printf("\nResources (%d):\n", len(ir.Resources))
			for _, res := range ir.Resources {
				ops := strings.Join(res.Operations, ", ")
				events := ""
				if res.Events {
					events = " [events]"
				}
				fmt.Printf("  %s (plural: %s) ops=[%s]%s\n", res.Name, res.Plural, ops, events)
				for _, f := range res.Fields {
					flags := ""
					if f.Required {
						flags += " required"
					}
					if f.Auto != "" {
						flags += " auto:" + f.Auto
					}
					if f.SoftDelete {
						flags += " soft_delete"
					}
					if f.MaxLength > 0 {
						flags += fmt.Sprintf(" max:%d", f.MaxLength)
					}
					fmt.Printf("    %-20s %-12s%s\n", f.Name, f.Type, flags)
				}
			}

			if ir.API != nil {
				fmt.Println("\nAPI:")
				if ir.API.REST != nil {
					fmt.Printf("  REST base_path: %s\n", ir.API.REST.BasePath)
				}
				if ir.API.GRPC != nil && ir.API.GRPC.Enabled {
					port := ir.API.GRPC.Port
					if port == 0 {
						port = 9000
					}
					fmt.Printf("  gRPC port: %d\n", port)
				}
			}

			if ir.Auth != nil {
				fmt.Printf("\nAuth: %s (required=%v)\n", ir.Auth.Type, ir.Auth.Required)
			}

			if ir.Database != nil {
				fmt.Printf("Database: %s (migrations=%s)\n", ir.Database.Type, ir.Database.Migrations)
			}

			if ir.Observ != nil {
				if ir.Observ.HealthCheck != nil {
					fmt.Printf("Health: :%d%s\n", ir.Observ.HealthCheck.Port, ir.Observ.HealthCheck.Path)
				}
				if ir.Observ.Metrics != nil {
					fmt.Printf("Metrics: :%d%s\n", ir.Observ.Metrics.Port, ir.Observ.Metrics.Path)
				}
			}

			return nil
		},
	}

	return cmd
}
