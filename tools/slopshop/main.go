package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	if err := newCommand(".", os.Stdout, os.Stderr).Execute(); err != nil {
		if !errors.Is(err, errInvalidManifest) {
			fmt.Fprintln(os.Stderr, "slopshop:", err)
		}
		os.Exit(1)
	}
}

func newCommand(root string, stdout, stderr io.Writer) *cobra.Command {
	var verbose bool
	command := &cobra.Command{
		Use:           "slopshop",
		Short:         "Generate and verify the Slopshop marketplace",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	command.SetOut(stdout)
	command.SetErr(stderr)
	command.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "show detailed progress")
	for _, item := range []struct{ name, description string }{
		{"generate", "Fetch external skills and generate marketplace files"},
		{"check", "Fetch external skills and check generated files without changing them"},
	} {
		command.AddCommand(&cobra.Command{
			Use: item.name, Short: item.description, Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runWithReporter(root, cmd.Name(), fetchSkill, newReporter(stdout, verbose))
			},
		})
	}
	var output string
	validate := &cobra.Command{
		Use:   "validate",
		Short: "Validate slopshop.hcl without changing files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return validateCommand(root, output, verbose, stdout, stderr)
		},
	}
	validate.Flags().StringVarP(&output, "output", "o", "rich", "diagnostic format (rich, json, plain)")
	command.AddCommand(validate)
	return command
}
