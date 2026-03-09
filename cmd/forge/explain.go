package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jsell-rh/trusted-software-foundry/foundry/compiler"
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
  - Tenancy, authorization, graph topology
  - Distributed services, event streaming
  - Distributed state, workflow engine
  - Lifecycle hooks

AI agents use explain to verify their edits produced the intended application
before running forge compile.`,
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
				fmt.Printf("  %-24s %s\n", name, version)
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

			if ir.Tenancy != nil {
				fmt.Printf("\nTenancy: strategy=%s field=%s", ir.Tenancy.Strategy, ir.Tenancy.Field)
				if ir.Tenancy.Header != "" {
					fmt.Printf(" header=%s", ir.Tenancy.Header)
				}
				fmt.Println()
			}

			if ir.Authz != nil {
				fmt.Printf("\nAuthz: backend=%s", ir.Authz.Backend)
				if ir.Authz.SchemaFile != "" {
					fmt.Printf(" schema=%s", ir.Authz.SchemaFile)
				}
				fmt.Println()
				if len(ir.Authz.Relations) > 0 {
					fmt.Printf("  Relations (%d):\n", len(ir.Authz.Relations))
					for _, r := range ir.Authz.Relations {
						fmt.Printf("    %s.%s → %s\n", r.Resource, r.Relation, r.Subject)
					}
				}
			}

			if ir.Graph != nil {
				fmt.Printf("\nGraph: backend=%s", ir.Graph.Backend)
				if ir.Graph.GraphName != "" {
					fmt.Printf(" graph=%s", ir.Graph.GraphName)
				}
				fmt.Println()
				if len(ir.Graph.NodeTypes) > 0 {
					fmt.Printf("  Nodes (%d):", len(ir.Graph.NodeTypes))
					labels := make([]string, len(ir.Graph.NodeTypes))
					for i, n := range ir.Graph.NodeTypes {
						labels[i] = n.Label
					}
					fmt.Printf(" %s\n", strings.Join(labels, ", "))
				}
				if len(ir.Graph.EdgeTypes) > 0 {
					fmt.Printf("  Edges (%d):", len(ir.Graph.EdgeTypes))
					edges := make([]string, len(ir.Graph.EdgeTypes))
					for i, e := range ir.Graph.EdgeTypes {
						edges[i] = fmt.Sprintf("%s(%s→%s)", e.Label, e.From, e.To)
					}
					fmt.Printf(" %s\n", strings.Join(edges, ", "))
				}
			}

			if len(ir.Services) > 0 {
				fmt.Printf("\nServices (%d):\n", len(ir.Services))
				for _, svc := range ir.Services {
					portStr := ""
					if svc.Port > 0 {
						portStr = fmt.Sprintf(" port:%d", svc.Port)
					}
					fmt.Printf("  %-20s role:%-10s%s components:[%s]\n",
						svc.Name, svc.Role, portStr, strings.Join(svc.Components, ", "))
				}
			}

			if ir.Events != nil {
				fmt.Printf("\nEvents: backend=%s", ir.Events.Backend)
				if ir.Events.BrokerURL != "" {
					fmt.Printf(" broker=%s", ir.Events.BrokerURL)
				}
				fmt.Println()
				for _, t := range ir.Events.Topics {
					ops := strings.Join(t.Operations, ",")
					fmt.Printf("  %-40s partitions:%d ops:[%s]\n", t.Name, t.Partitions, ops)
				}
			}

			if ir.State != nil {
				fmt.Printf("\nState: backend=%s\n", ir.State.Backend)
				for _, k := range ir.State.Keys {
					ttl := ""
					if k.TTLSeconds > 0 {
						ttl = fmt.Sprintf(" ttl:%ds", k.TTLSeconds)
					}
					fmt.Printf("  %-40s strategy:%s%s\n", k.Name, k.Strategy, ttl)
				}
			}

			if ir.Workflows != nil {
				fmt.Printf("\nWorkflows: namespace=%s queue=%s\n", ir.Workflows.Namespace, ir.Workflows.WorkerQueue)
				for _, w := range ir.Workflows.Workflows {
					trigger := ""
					if w.Trigger != "" {
						trigger = fmt.Sprintf(" trigger:%s", w.Trigger)
					}
					fmt.Printf("  %-30s%s activities:[%s]\n", w.Name, trigger,
						strings.Join(w.Activities, ", "))
				}
			}

			if len(ir.Hooks) > 0 {
				fmt.Printf("\nHooks (%d):\n", len(ir.Hooks))
				for _, h := range ir.Hooks {
					routes := ""
					if len(h.Routes) > 0 {
						routes = fmt.Sprintf(" routes:%s", strings.Join(h.Routes, ","))
					}
					topic := ""
					if h.Topic != "" {
						topic = fmt.Sprintf(" topic:%s", h.Topic)
					}
					fmt.Printf("  %-30s point:%-15s%s%s impl:%s\n",
						h.Name, h.Point, routes, topic, h.Implementation)
				}
			}

			return nil
		},
	}

	return cmd
}
