package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

// serverOpsCommands are the `servers` subcommands that operate on a node.
func serverOpsCommands() []*cobra.Command {
	updateCmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update a server's host, ingress or default flag",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var current ServerResponse
			if err := apiJSON("GET", "/servers/"+args[0], nil, &current); err != nil {
				return err
			}

			body := map[string]any{
				"host":         current.Host,
				"description":  current.Description,
				"is_default":   current.IsDefault,
				"ingress_host": current.IngressHost,
				"ingress_port": current.IngressPort,
			}
			for flag, key := range map[string]string{"host": "host", "description": "description", "ingress-host": "ingress_host"} {
				if cmd.Flags().Changed(flag) {
					body[key], _ = cmd.Flags().GetString(flag)
				}
			}
			if cmd.Flags().Changed("ingress-port") {
				body["ingress_port"], _ = cmd.Flags().GetInt("ingress-port")
			}
			if cmd.Flags().Changed("default") {
				body["is_default"], _ = cmd.Flags().GetBool("default")
			}

			if err := apiJSON("PUT", "/servers/"+args[0], body, nil); err != nil {
				return err
			}
			fmt.Printf("Server %s updated\n", args[0])
			return nil
		},
	}
	updateCmd.Flags().String("host", "", "Docker host (unix://, tcp:// or ssh://user@host)")
	updateCmd.Flags().String("description", "", "Server description")
	updateCmd.Flags().String("ingress-host", "", "Host the gateway forwards this node's traffic to")
	updateCmd.Flags().Int("ingress-port", 0, "Port the gateway forwards this node's traffic to")
	updateCmd.Flags().Bool("default", false, "Set as default server")

	ingressCmd := &cobra.Command{
		Use:   "ingress <name>",
		Short: "Install or update the node ingress (Traefik) on a server",
		Long: `Runs Traefik on the node, listening on --port. Point the gateway at it
(directly, or through an SSH tunnel), then record where the gateway reaches it:

  docktrine servers update <name> --ingress-host <host> --ingress-port <port>`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			port, _ := cmd.Flags().GetInt("port")
			bind, _ := cmd.Flags().GetString("bind")
			var result struct {
				Message string `json:"message"`
			}
			if err := apiJSON("POST", "/servers/"+args[0]+"/ingress", map[string]any{"port": port, "bind": bind}, &result); err != nil {
				return err
			}
			fmt.Println(result.Message)
			return nil
		},
	}
	ingressCmd.Flags().Int("port", 8880, "Port the ingress listens on, on the node")
	ingressCmd.Flags().String("bind", "", "Address to bind the port to (e.g. 127.0.0.1 for tunnel-only access)")

	healthCmd := &cobra.Command{
		Use:   "health <name>",
		Short: "Check that a server is reachable and show its Docker status",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var health map[string]any
			if err := apiJSON("GET", "/servers/"+args[0]+"/health", nil, &health); err != nil {
				return err
			}
			if health["reachable"] != true {
				return fmt.Errorf("%s is unreachable: %v", args[0], health["error"])
			}
			fmt.Printf("%s: Docker %v on %v\n", args[0], health["server_version"], health["os"])
			fmt.Printf("  CPUs: %v, memory: %.1f GiB\n", health["cpus"], health["memory_bytes"].(float64)/(1<<30))
			fmt.Printf("  Containers: %v running / %v total, images: %v\n",
				health["containers_running"], health["containers"], health["images"])
			fmt.Printf("  Ingress: %v\n", health["ingress"])
			return nil
		},
	}

	pruneCmd := &cobra.Command{
		Use:   "prune <name>",
		Short: "Remove unused images from a server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			all, _ := cmd.Flags().GetBool("all")
			var result struct {
				ImagesDeleted  int    `json:"images_deleted"`
				SpaceReclaimed uint64 `json:"space_reclaimed"`
			}
			if err := apiJSON("POST", fmt.Sprintf("/servers/%s/prune?all=%t", args[0], all), nil, &result); err != nil {
				return err
			}
			fmt.Printf("Removed %d image(s), reclaimed %.1f MiB\n", result.ImagesDeleted, float64(result.SpaceReclaimed)/(1<<20))
			return nil
		},
	}
	pruneCmd.Flags().Bool("all", false, "Remove every image not used by a container, not just dangling ones")

	return []*cobra.Command{updateCmd, ingressCmd, healthCmd, pruneCmd}
}
