package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/spf13/cobra"
)

type ServerResponse struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Host        string    `json:"host"`
	Description string    `json:"description"`
	IsDefault   bool      `json:"is_default"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

var cachedServers []ServerResponse
var (
	listServersCmd   *cobra.Command
	addServerCmd     *cobra.Command
	removeServerCmd  *cobra.Command
)

func fetchServers() error {
	resp, err := http.Get(fmt.Sprintf("%s/servers", apiURL))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := handleError(resp); err != nil {
		return err
	}

	if err := json.NewDecoder(resp.Body).Decode(&cachedServers); err != nil {
		return err
	}

	return nil
}

func init() {
	serversCmd := &cobra.Command{
		Use:   "servers",
		Short: "Manage Docker servers",
	}

	listServersCmd = &cobra.Command{
		Use:   "list",
		Short: "List all servers",
		Run: func(cmd *cobra.Command, args []string) {
			resp, err := http.Get(fmt.Sprintf("%s/servers", apiURL))
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			defer resp.Body.Close()

			if err := handleError(resp); err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}

			var servers []ServerResponse
			if err := json.NewDecoder(resp.Body).Decode(&servers); err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}

			for _, server := range servers {
				fmt.Printf("Name: %s\nHost: %s\nDefault: %v\n", 
					server.Name, 
					server.Host, 
					server.IsDefault)
				if server.Description != "" {
					fmt.Printf("Description: %s\n", server.Description)
				}
				fmt.Println()
			}
		},
	}

	addServerCmd = &cobra.Command{
		Use:   "add",
		Short: "Add a new server",
		Run: func(cmd *cobra.Command, args []string) {
			name, _ := cmd.Flags().GetString("name")
			host, _ := cmd.Flags().GetString("host")
			desc, _ := cmd.Flags().GetString("description")
			isDefault, _ := cmd.Flags().GetBool("default")

			if name == "" || host == "" {
				fmt.Println("Error: name and host are required")
				return
			}

			payload := map[string]interface{}{
				"name":        name,
				"host":        host,
				"description": desc,
				"is_default":  isDefault,
			}

			jsonData, err := json.Marshal(payload)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}

			resp, err := http.Post(
				fmt.Sprintf("%s/servers", apiURL),
				"application/json",
				bytes.NewBuffer(jsonData),
			)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			defer resp.Body.Close()

			if err := handleError(resp); err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}

			if err := fetchServers(); err != nil {
				fmt.Printf("Warning: Failed to refresh server cache: %v\n", err)
			}

			fmt.Printf("Server '%s' added successfully\n", name)
		},
	}

	removeServerCmd = &cobra.Command{
		Use:   "remove [name]",
		Short: "Remove a server",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			name := args[0]
			
			req, err := http.NewRequest(
				"DELETE",
				fmt.Sprintf("%s/servers/%s", apiURL, name),
				nil,
			)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
			defer resp.Body.Close()

			if err := handleError(resp); err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}

			if err := fetchServers(); err != nil {
				fmt.Printf("Warning: Failed to refresh server cache: %v\n", err)
			}

			fmt.Printf("Server '%s' removed successfully\n", name)
		},
	}

	addServerCmd.Flags().String("name", "", "Server name")
	addServerCmd.Flags().String("host", "", "Server host (e.g., unix:///var/run/docker.sock)")
	addServerCmd.Flags().String("description", "", "Server description")
	addServerCmd.Flags().Bool("default", false, "Set as default server")

	serversCmd.AddCommand(listServersCmd, addServerCmd, removeServerCmd)
	rootCmd.AddCommand(serversCmd)
} 