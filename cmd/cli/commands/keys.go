package commands

import (
	"fmt"
	"os"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

type apiKeyInfo struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	Owner      string     `json:"owner"`
	Current    bool       `json:"current"`
}

func listKeys() ([]apiKeyInfo, error) {
	var keys []apiKeyInfo
	err := apiJSON("GET", "/keys", nil, &keys)
	return keys, err
}

// findKey resolves a key by name, or by ID if no name matches.
func findKey(ref string) (*apiKeyInfo, error) {
	keys, err := listKeys()
	if err != nil {
		return nil, err
	}
	for i := range keys {
		if keys[i].Name == ref {
			return &keys[i], nil
		}
	}
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		for i := range keys {
			if keys[i].ID == id {
				return &keys[i], nil
			}
		}
	}
	return nil, fmt.Errorf("no API key named %q (see `docktrine keys list`)", ref)
}

func init() {
	keysCmd := &cobra.Command{
		Use:   "keys",
		Short: "Manage API keys",
		Long: `Each person or system (laptop, CI) should have its own key: deploy
history shows the key's name, and a key can be revoked on its own.
A key acts with the role of the user who created it; admins see and
manage everyone's keys.`,
	}

	listKeysCmd := &cobra.Command{
		Use:   "list",
		Short: "List API keys",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			keys, err := listKeys()
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tNAME\tOWNER\tKEY\tCREATED\tLAST USED")
			for _, k := range keys {
				name := k.Name
				if k.Current {
					name += " (this key)"
				}
				lastUsed := "never"
				if k.LastUsedAt != nil {
					lastUsed = ago(*k.LastUsedAt)
				}
				owner := k.Owner
				if owner == "" {
					owner = "(admin key)"
				}
				fmt.Fprintf(w, "%d\t%s\t%s\t%s…\t%s\t%s\n", k.ID, name, owner, k.Prefix, ago(k.CreatedAt), lastUsed)
			}
			return w.Flush()
		},
	}

	createKeyCmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an API key for a person or system, e.g. alex-laptop or ci",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var key struct {
				Name string `json:"name"`
				Key  string `json:"key"`
			}
			if err := apiJSON("POST", "/keys", map[string]string{"name": args[0]}, &key); err != nil {
				return err
			}
			// The key alone on stdout, so it can be captured by a script
			fmt.Fprintf(os.Stderr, "Created API key %q. Store it now, it won't be shown again:\n", key.Name)
			fmt.Println(key.Key)
			return nil
		},
	}

	renameKeyCmd := &cobra.Command{
		Use:   "rename <name|id> <new-name>",
		Short: "Rename an API key",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			k, err := findKey(args[0])
			if err != nil {
				return err
			}
			if err := apiJSON("PATCH", fmt.Sprintf("/keys/%d", k.ID), map[string]string{"name": args[1]}, nil); err != nil {
				return err
			}
			fmt.Printf("Renamed %q to %q\n", k.Name, args[1])
			return nil
		},
	}

	revokeKeyCmd := &cobra.Command{
		Use:   "revoke <name|id>",
		Short: "Revoke an API key; requests using it fail from now on",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			k, err := findKey(args[0])
			if err != nil {
				return err
			}
			yes, _ := cmd.Flags().GetBool("yes")
			prompt := fmt.Sprintf("Revoke API key %q (%s…)?", k.Name, k.Prefix)
			if k.Current {
				prompt = fmt.Sprintf("%q is the key this CLI is using; you'll need another key afterwards. Revoke it?", k.Name)
			}
			if !yes && !confirm(prompt) {
				return fmt.Errorf("cancelled")
			}
			if err := apiJSON("DELETE", fmt.Sprintf("/keys/%d", k.ID), nil, nil); err != nil {
				return err
			}
			fmt.Printf("Revoked %q\n", k.Name)
			return nil
		},
	}
	revokeKeyCmd.Flags().BoolP("yes", "y", false, "Don't ask for confirmation")

	keysCmd.AddCommand(listKeysCmd, createKeyCmd, renameKeyCmd, revokeKeyCmd)
	rootCmd.AddCommand(keysCmd)
}
