package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/c-bata/go-prompt"
	"github.com/spf13/cobra"
)

var currentServer string = server // Initialize with the global server variable

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
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Docktrine Interactive Shell")
			fmt.Println("Type 'exit' to quit, 'help' for commands")
			
			// Pre-fetch servers
			if err := fetchServers(); err != nil {
				fmt.Printf("Warning: Failed to fetch servers: %v\n", err)
			}
			
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
			currentServer = args[1]
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
			restartCmd.Run(restartCmd, cmdArgs[1:])
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
			if len(cmdArgs) < 5 {
				fmt.Println("Usage: servers add --name=<name> --host=<host> [--description=<desc>] [--default=<true|false>]")
				return
			}
			addServerCmd.Run(addServerCmd, cmdArgs[1:])
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