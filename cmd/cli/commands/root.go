package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	apiURL  string
	server  string
	apiKey  string
	rootCmd = &cobra.Command{
		Use:   "docktrine",
		Short: "Docktrine CLI - Manage Docker containers",
		Long:  `A CLI tool for managing Docker containers through the Docktrine API.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if !cmd.Flags().Changed("api-url") {
				if envURL := os.Getenv("DOCKTRINE_API_URL"); envURL != "" {
					apiURL = envURL
				}
			}
			
			if !cmd.Flags().Changed("api-key") {
				if envKey := os.Getenv("DOCKTRINE_API_KEY"); envKey != "" {
					apiKey = envKey
				}
			}
			
			if apiURL == "" {
				return fmt.Errorf("DOCKTRINE_API_URL environment variable or --api-url flag must be set")
			}
			
			if apiKey == "" {
				return fmt.Errorf("DOCKTRINE_API_KEY environment variable or --api-key flag must be set")
			}
			
			return nil
		},
	}
)

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&apiURL, "api-url", "", "Docktrine API URL (overrides DOCKTRINE_API_URL env var)")
	rootCmd.PersistentFlags().StringVar(&server, "server", "", "Docker server name to connect to")
	rootCmd.PersistentFlags().StringVar(&apiKey, "api-key", "", "API Key for authentication (overrides DOCKTRINE_API_KEY env var)")
} 