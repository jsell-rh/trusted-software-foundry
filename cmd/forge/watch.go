package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/jsell-rh/trusted-software-foundry/foundry/compiler"
)

func watchCmd() *cobra.Command {
	var (
		outputDir   string
		foundryPath string
		interval    time.Duration
	)

	cmd := &cobra.Command{
		Use:   "watch <spec.yaml>",
		Short: "Watch a Foundry spec and recompile on change",
		Long: `forge watch polls the spec file for changes and recompiles automatically.

On each detected change, forge recompiles the spec into the output directory.
Press Ctrl+C to stop watching.

Uses mtime polling (no external dependencies). Interval defaults to 1s.

Example:
  forge watch app.foundry.yaml --output ./out
  forge watch app.foundry.yaml --output ./out --interval 500ms`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			specPath := args[0]

			if outputDir == "" {
				return fmt.Errorf("--output is required")
			}

			// Verify spec file exists before entering the watch loop.
			if _, err := os.Stat(specPath); err != nil {
				return fmt.Errorf("spec file %q: %w", specPath, err)
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Watching %s (interval: %s) → %s\n", specPath, interval, outputDir)
			fmt.Fprintf(out, "Press Ctrl+C to stop.\n")

			// Initial compile.
			if err := recompile(specPath, outputDir, foundryPath, out); err != nil {
				fmt.Fprintf(out, "compile error: %v\n", err)
			}

			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			var lastMod time.Time
			if info, err := os.Stat(specPath); err == nil {
				lastMod = info.ModTime()
			}

			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					fmt.Fprintf(out, "\nStopping watcher.\n")
					return nil
				case <-ticker.C:
					info, err := os.Stat(specPath)
					if err != nil {
						fmt.Fprintf(out, "stat %s: %v\n", specPath, err)
						continue
					}
					if info.ModTime().After(lastMod) {
						lastMod = info.ModTime()
						fmt.Fprintf(out, "[%s] Change detected — recompiling...\n",
							time.Now().Format("15:04:05"))
						if err := recompile(specPath, outputDir, foundryPath, out); err != nil {
							fmt.Fprintf(out, "compile error: %v\n", err)
						}
					}
				}
			}
		},
	}

	cmd.Flags().StringVarP(&outputDir, "output", "o", "", "Output directory for generated project (required)")
	cmd.Flags().StringVar(&foundryPath, "foundry-path", "", "Local path to trusted-software-foundry checkout")
	cmd.Flags().DurationVar(&interval, "interval", time.Second, "Polling interval (e.g. 500ms, 2s)")

	return cmd
}

// recompile runs the full compiler pipeline for specPath → outputDir.
func recompile(specPath, outputDir, foundryPath string, out interface{ Write([]byte) (int, error) }) error {
	c := compiler.New(compiler.NewStubRegistry(), "", foundryPath)
	if err := c.Compile(specPath, outputDir); err != nil {
		return err
	}
	fmt.Fprintf(out, "[%s] Compiled OK → %s\n", time.Now().Format("15:04:05"), outputDir)
	return nil
}
