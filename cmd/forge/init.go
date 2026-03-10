package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func initCmd() *cobra.Command {
	var resources []string
	var version string

	cmd := &cobra.Command{
		Use:   "init <name>",
		Short: "Initialize a new Foundry project directory",
		Long: `init creates a new directory with everything needed to start a Trusted Software Foundry project:

  <name>/
    app.foundry.yaml    ← starter IR spec (edit this)
    hooks/
      README.md         ← hook implementation guide
    .gitignore

After init, edit app.foundry.yaml, then:
  forge lint <name>/app.foundry.yaml
  forge compile <name>/app.foundry.yaml --foundry-path . -o <name>/out/
  cd <name>/out && go build -o app .`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			projectDir := name

			// Don't overwrite an existing directory.
			if _, err := os.Stat(projectDir); err == nil {
				return fmt.Errorf("directory %q already exists — choose a different name or remove it", projectDir)
			}

			if err := os.MkdirAll(filepath.Join(projectDir, "hooks"), 0755); err != nil {
				return fmt.Errorf("creating project directory: %w", err)
			}

			// Write app.foundry.yaml
			spec := renderScaffold(name, version, resources)
			specPath := filepath.Join(projectDir, "app.foundry.yaml")
			if err := os.WriteFile(specPath, []byte(spec), 0644); err != nil {
				return fmt.Errorf("writing app.foundry.yaml: %w", err)
			}

			// Write hooks/README.md — guide for implementing hooks
			hooksGuide := renderHooksGuide(name)
			if err := os.WriteFile(filepath.Join(projectDir, "hooks", "README.md"), []byte(hooksGuide), 0644); err != nil {
				return fmt.Errorf("writing hooks/README.md: %w", err)
			}

			// Write .gitignore
			gitignore := "out/\n*.test\n*.log\n"
			if err := os.WriteFile(filepath.Join(projectDir, ".gitignore"), []byte(gitignore), 0644); err != nil {
				return fmt.Errorf("writing .gitignore: %w", err)
			}

			fmt.Printf("Initialized Foundry project: %s/\n\n", projectDir)
			fmt.Printf("  %s/app.foundry.yaml   ← edit this spec\n", projectDir)
			fmt.Printf("  %s/hooks/             ← custom logic goes here\n\n", projectDir)
			fmt.Println("Next steps:")
			fmt.Printf("  1. Edit %s/app.foundry.yaml\n", projectDir)
			fmt.Printf("  2. forge lint %s/app.foundry.yaml\n", projectDir)
			fmt.Printf("  3. forge compile %s/app.foundry.yaml --foundry-path /path/to/trusted-software-foundry -o %s/out/\n", projectDir, projectDir)
			fmt.Printf("  4. cd %s/out && go build -o app . && ./app\n", projectDir)
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&resources, "resource", nil, "Resource names to scaffold (e.g. --resource User --resource Post)")
	cmd.Flags().StringVar(&version, "version", "1.0.0", "Application version")
	return cmd
}

func renderHooksGuide(appName string) string {
	var sb strings.Builder
	sb.WriteString("# Hook Implementations for " + appName + "\n\n")
	sb.WriteString("This directory contains custom hook implementations that are injected into\n")
	sb.WriteString("the generated application at well-defined points in the request lifecycle.\n\n")
	sb.WriteString("## How hooks work\n\n")
	sb.WriteString("1. Declare hooks in `app.foundry.yaml` under the `hooks:` block\n")
	sb.WriteString("2. Implement each hook as a Go function in this directory\n")
	sb.WriteString("3. Run `forge compile` — your files are copied into the generated project\n\n")
	sb.WriteString("## Injection points\n\n")
	sb.WriteString("| Point | Signature | When |\n")
	sb.WriteString("|-------|-----------|------|\n")
	sb.WriteString("| `pre-handler` | `(hctx, w, r)` | Before every HTTP handler |\n")
	sb.WriteString("| `post-handler` | `(hctx, req)` | After every HTTP handler |\n")
	sb.WriteString("| `pre-db` | `(hctx, op)` | Before every DB write |\n")
	sb.WriteString("| `post-db` | `(hctx, result)` | After every DB write |\n")
	sb.WriteString("| `pre-publish` | `(hctx, msg)` | Before publishing an event |\n")
	sb.WriteString("| `post-consume` | `(hctx, event)` | After consuming an event |\n\n")
	sb.WriteString("## Example\n\n")
	sb.WriteString("```yaml\n# app.foundry.yaml\nhooks:\n")
	sb.WriteString("  - name: audit-logger\n    point: pre-handler\n    implementation: hooks/audit_logger.go\n```\n\n")
	sb.WriteString("```go\n// hooks/audit_logger.go\npackage hooks\n\n")
	sb.WriteString("import (\n    \"net/http\"\n    \"github.com/jsell-rh/trusted-software-foundry/foundry/spec/foundry\"\n)\n\n")
	sb.WriteString("func AuditLoggerPreHandler(hctx *foundry.HookContext, w http.ResponseWriter, r *http.Request) error {\n")
	sb.WriteString("    hctx.Logger.Info(\"request\", \"method\", r.Method, \"path\", r.URL.Path)\n")
	sb.WriteString("    return nil\n}\n```\n\n")
	sb.WriteString("## If a hook file is missing\n\n")
	sb.WriteString("`forge compile` generates a stub in `hooks/stubs_generated.go`.\n")
	sb.WriteString("Replace the stub body with your real implementation and recompile.\n")
	return sb.String()
}
