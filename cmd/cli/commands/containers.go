package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
)

var (
	listCmd    *cobra.Command
	startCmd   *cobra.Command
	stopCmd    *cobra.Command
	restartCmd *cobra.Command
)

func handleError(resp *http.Response) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errorResponse map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err != nil {
			return fmt.Errorf("HTTP %d: %v", resp.StatusCode, err)
		}
		return fmt.Errorf("%v", errorResponse["error"])
	}
	return nil
}

func init() {	
	containersCmd := &cobra.Command{
		Use:   "containers",
		Short: "Manage Docker containers",
	}

	listCmd = &cobra.Command{
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

			if err := handleError(resp); err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}

			var containers []map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}

			for _, container := range containers {
				fmt.Printf("ID: %s\nName: %s\nStatus: %s\n", 
					container["id"], 
					container["name"], 
					container["status"])
				
				if ports, ok := container["ports"].([]interface{}); ok && len(ports) > 0 {
					fmt.Println("Ports:")
					for _, p := range ports {
						if port, ok := p.(map[string]interface{}); ok {
							ip := port["IP"]
							if ip == nil || ip == "" {
								ip = "0.0.0.0"
							}
							
							if publicPort, exists := port["PublicPort"]; exists && publicPort != 0 {
								fmt.Printf("  %v:%v → %v/%s\n",
									ip,
									publicPort,
									port["PrivatePort"],
									port["Type"])
							} else {
								fmt.Printf("  %v/%s\n",
									port["PrivatePort"],
									port["Type"])
							}
						}
					}
				}
				fmt.Println()
			}
		},
	}

	startCmd = &cobra.Command{
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
			defer resp.Body.Close()
			
			if err := handleError(resp); err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			
			fmt.Printf("Container %s started\n", args[0])
		},
	}

	stopCmd = &cobra.Command{
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
			defer resp.Body.Close()

			if err := handleError(resp); err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}

			fmt.Printf("Container %s stopped\n", args[0])
		},
	}

	restartCmd = &cobra.Command{
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
			defer resp.Body.Close()

			if err := handleError(resp); err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}

			fmt.Printf("Container %s restarted\n", args[0])
		},
	}

	restartCmd.Flags().Bool("pull-latest", false, "Pull latest image before restart")

	containersCmd.AddCommand(listCmd, startCmd, stopCmd, restartCmd)
	rootCmd.AddCommand(containersCmd)
} 