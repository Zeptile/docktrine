package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)


func init() {	
	containersCmd := &cobra.Command{
		Use:   "containers",
		Short: "Manage Docker containers",
	}


	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all containers",
		Run: func(cmd *cobra.Command, args []string) {
			uri := fmt.Sprintf("%s/containers", apiURL)
			if server != "" {
				uri += fmt.Sprintf("?server=%s", url.QueryEscape(server))
			}
			
			resp, err := http.Get(uri)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			defer resp.Body.Close()

			var containers []map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}

			for _, container := range containers {
				fmt.Printf("ID: %s\nName: %s\nStatus: %s\n\n", 
					container["id"], 
					container["name"], 
					container["status"])
			}
		},
	}

	startCmd := &cobra.Command{
		Use:   "start [container-id]",
		Short: "Start a container",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			uri := fmt.Sprintf("%s/containers/start/%s", apiURL, args[0])
			if server != "" {
				uri += fmt.Sprintf("?server=%s", url.QueryEscape(server))
			}
			resp, err := http.Post(uri, "application/json", nil)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Printf("Container %s started\n", args[0])
			resp.Body.Close()
		},
	}

	stopCmd := &cobra.Command{
		Use:   "stop [container-id]",
		Short: "Stop a container",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			uri := fmt.Sprintf("%s/containers/stop/%s", apiURL, args[0])
			if server != "" {
				uri += fmt.Sprintf("?server=%s", url.QueryEscape(server))
			}
			resp, err := http.Post(uri, "application/json", nil)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Printf("Container %s stopped\n", args[0])
			resp.Body.Close()
		},
	}

	restartCmd := &cobra.Command{
		Use:   "restart [container-id]",
		Short: "Restart a container",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			uri := fmt.Sprintf("%s/containers/restart/%s", apiURL, args[0])
			if server != "" {
				uri += fmt.Sprintf("?server=%s", url.QueryEscape(server))
			}
			
			pullLatest, _ := cmd.Flags().GetBool("pull-latest")
			if pullLatest {
				uri += "&pull_latest=true"
			}
			
			resp, err := http.Post(uri, "application/json", nil)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			fmt.Printf("Container %s restarted\n", args[0])
			resp.Body.Close()
		},
	}

	restartCmd.Flags().Bool("pull-latest", false, "Pull latest image before restart")

	containersCmd.AddCommand(listCmd, startCmd, stopCmd, restartCmd)
	rootCmd.AddCommand(containersCmd)
} 