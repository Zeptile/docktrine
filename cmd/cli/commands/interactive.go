package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/c-bata/go-prompt"
	"github.com/spf13/cobra"
)

var currentServer string = server

func getPromptPrefix() string {
	serverName := currentServer
	if serverName == "" {
		serverName = "default"
	}
	return fmt.Sprintf("docktrine(%s)> ", serverName)
}

func init() {
	interactiveCmd := &cobra.Command{
		Use:   "interactive",
		Short: "Start interactive shell mode",
		PersistentPreRunE: rootCmd.PersistentPreRunE,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Docktrine Interactive Shell")
			fmt.Println("Type 'exit' to quit, 'help' for commands")
			fmt.Printf("Using API URL: %s\n", apiURL)
			
			p := prompt.New(
				executor,
				completer,
				prompt.OptionPrefix(getPromptPrefix()),
				prompt.OptionLivePrefix(func() (string, bool) {
					return getPromptPrefix(), true
				}),
				prompt.OptionTitle("Docktrine Interactive Shell"),
			)
			p.Run()
		},
	}

	rootCmd.AddCommand(interactiveCmd)
}

func executor(input string) {
	input = strings.TrimSpace(input)
	args := strings.Fields(input)

	if len(args) == 0 {
		return
	}

	switch args[0] {
	case "exit":
		fmt.Println("Goodbye!")
		os.Exit(0)
	case "server":
		if len(args) == 1 {
			if currentServer == "" {
				fmt.Println("Current server: default")
			} else {
				fmt.Println("Current server:", currentServer)
			}
			return
		}
		if len(args) == 2 {
			// Check if server exists in cache
			serverExists := false
			requestedServer := args[1]
			
			if requestedServer == "default" {
				currentServer = ""
				server = currentServer
				fmt.Println("Switched to default server")
				return
			}

			for _, s := range cachedServers {
				if s.Name == requestedServer {
					serverExists = true
					break
				}
			}

			if !serverExists {
				fmt.Printf("Error: server '%s' not found\n", requestedServer)
				return
			}

			currentServer = requestedServer
			server = currentServer
			fmt.Printf("Switched to server: %s\n", currentServer)
			return
		}
		fmt.Println("Usage: server [name]")
	case "help":
		fmt.Println("Available commands:")
		fmt.Println("  containers list              - List all containers")
		fmt.Println("  containers start <id>        - Start a container")
		fmt.Println("  containers stop <id>         - Stop a container")
		fmt.Println("  containers restart <id>      - Restart a container")
		fmt.Println("  server                       - Show current server")
		fmt.Println("  server <name>                - Switch to different server")
		fmt.Println("  servers list                 - List all servers")
		fmt.Println("  servers add                  - Add a new server")
		fmt.Println("  servers remove <name>        - Remove a server")
		fmt.Println("  help                         - Show this help")
		fmt.Println("  exit                         - Exit interactive mode")
		return
	case "containers":
		if len(args) < 2 {
			fmt.Println("Usage: containers <command> [args]")
			return
		}
		
		cmdArgs := args[1:]
		
		switch cmdArgs[0] {
		case "list":
			if currentServer != "" {
				server = currentServer
			}
			listCmd.Run(listCmd, []string{})
		case "start":
			if len(cmdArgs) < 2 {
				fmt.Println("Usage: containers start <container-id>")
				return
			}
			if currentServer != "" {
				server = currentServer
			}
			startCmd.Run(startCmd, cmdArgs[1:])
		case "stop":
			if len(cmdArgs) < 2 {
				fmt.Println("Usage: containers stop <container-id>")
				return
			}
			if currentServer != "" {
				server = currentServer
			}
			stopCmd.Run(stopCmd, cmdArgs[1:])
		case "restart":
			if len(cmdArgs) < 2 {
				fmt.Println("Usage: containers restart <container-id>")
				return
			}
			if currentServer != "" {
				server = currentServer
			}

			pullLatestInput := prompt.Input("Pull latest image before restart? (y/N): ", func(d prompt.Document) []prompt.Suggest {
				return []prompt.Suggest{
					{Text: "y", Description: "Yes"},
					{Text: "n", Description: "No"},
				}
			})
			pullLatest := strings.ToLower(strings.TrimSpace(pullLatestInput)) == "y"

			cmd := restartCmd
			cmd.Flags().Set("pull-latest", fmt.Sprintf("%v", pullLatest))
			cmd.Run(cmd, cmdArgs[1:])
		default:
			fmt.Printf("Unknown command: %s\n", cmdArgs[0])
		}
	case "servers":
		if len(args) < 2 {
			fmt.Println("Usage: servers <command> [args]")
			return
		}
		
		cmdArgs := args[1:]
		switch cmdArgs[0] {
		case "list":
			listServersCmd.Run(listServersCmd, []string{})
		case "add":
			name := prompt.Input("Enter server name: ", func(d prompt.Document) []prompt.Suggest {
				return []prompt.Suggest{}
			})
			name = strings.TrimSpace(name)

			host := prompt.Input("Enter server host: ", func(d prompt.Document) []prompt.Suggest {
				return []prompt.Suggest{
					{Text: "unix:///var/run/docker.sock", Description: "Local Docker socket"},
					{Text: "tcp://", Description: "Remote Docker daemon"},
				}
			})
			host = strings.TrimSpace(host)

			desc := prompt.Input("Enter server description (optional): ", func(d prompt.Document) []prompt.Suggest {
				return []prompt.Suggest{}
			})
			desc = strings.TrimSpace(desc)

			isDefaultInput := prompt.Input("Set as default server? (y/N): ", func(d prompt.Document) []prompt.Suggest {
				return []prompt.Suggest{
					{Text: "y", Description: "Yes"},
					{Text: "n", Description: "No"},
				}
			})
			isDefault := strings.ToLower(strings.TrimSpace(isDefaultInput)) == "y"

			if name == "" || host == "" {
				fmt.Println("Error: name and host are required")
				return
			}

			cmd := addServerCmd
			cmd.Flags().Set("name", name)
			cmd.Flags().Set("host", host)
			cmd.Flags().Set("description", desc)
			cmd.Flags().Set("default", fmt.Sprintf("%v", isDefault))
			
			cmd.Run(cmd, []string{})
		case "remove":
			if len(cmdArgs) < 2 {
				fmt.Println("Usage: servers remove <name>")
				return
			}
			removeServerCmd.Run(removeServerCmd, cmdArgs[1:])
		default:
			fmt.Printf("Unknown command: %s\n", cmdArgs[0])
		}
	default:
		fmt.Printf("Unknown command: %s\n", args[0])
	}
}

func completer(d prompt.Document) []prompt.Suggest {
	suggestions := []prompt.Suggest{
		{Text: "containers", Description: "Container management commands"},
		{Text: "server", Description: "Show or switch server"},
		{Text: "help", Description: "Show help"},
		{Text: "exit", Description: "Exit interactive mode"},
		{Text: "servers", Description: "Server management commands"},
	}

	if strings.HasPrefix(d.TextBeforeCursor(), "containers ") {
		return []prompt.Suggest{
			{Text: "list", Description: "List all containers"},
			{Text: "start", Description: "Start a container"},
			{Text: "stop", Description: "Stop a container"},
			{Text: "restart", Description: "Restart a container"},
		}
	}

	if strings.HasPrefix(d.TextBeforeCursor(), "server ") {
		var serverSuggestions []prompt.Suggest
		for _, s := range cachedServers {
			desc := s.Description
			if desc == "" {
				desc = s.Host
			}
			if s.IsDefault {
				desc += " (default)"
			}
			serverSuggestions = append(serverSuggestions, prompt.Suggest{
				Text:        s.Name,
				Description: desc,
			})
		}
		return serverSuggestions
	}

	if strings.HasPrefix(d.TextBeforeCursor(), "servers ") {
		return []prompt.Suggest{
			{Text: "list", Description: "List all servers"},
			{Text: "add", Description: "Add a new server"},
			{Text: "remove", Description: "Remove a server"},
		}
	}

	return prompt.FilterHasPrefix(suggestions, d.GetWordBeforeCursor(), true)
} 