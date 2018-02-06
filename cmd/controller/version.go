package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	commit    string
	tag       string
	buildDate string
)

func init() {
	RootCmd.AddCommand(VersionCmd)
}

var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Shows the version",
	Long:  "Shows the version",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Printf("Kubernetes-Vault controller %s (%s) built on %s\n", tag, commit, buildDate)
		return nil
	},
}
