package commands

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/manifest"
	"github.com/spf13/cobra"
)

// secretScope resolves the app a secrets command targets. Environments are
// used as given: "preview" is itself the scope shared by all previews.
func secretScope(cmd *cobra.Command, env string) (string, string, error) {
	app, _ := cmd.Flags().GetString("app")
	if app == "" {
		m, err := manifest.Load(manifestFile)
		if err != nil {
			return "", "", fmt.Errorf("pass --app or run inside a repo with %s: %w", manifest.FileName, err)
		}
		app = m.App
	}
	return app, env, nil
}

func init() {
	secretsCmd := &cobra.Command{
		Use:   "secrets",
		Short: "Manage environment secrets (values are stored encrypted on the server)",
		Long: `Secrets are injected as environment variables on the next deploy.
Secrets set on "preview" are shared by every preview of the app.`,
	}

	listSecretsCmd := &cobra.Command{
		Use:   "list <environment>",
		Short: "List secret names",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, env, err := secretScope(cmd, args[0])
			if err != nil {
				return err
			}
			var secrets []database.SecretInfo
			if err := apiJSON("GET", envPath(app, env)+"/secrets", nil, &secrets); err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "KEY\tUPDATED")
			for _, s := range secrets {
				fmt.Fprintf(w, "%s\t%s\n", s.Key, ago(s.UpdatedAt))
			}
			return w.Flush()
		},
	}

	setSecretsCmd := &cobra.Command{
		Use:   "set <environment> KEY=value [KEY=value...]",
		Short: "Set one or more secrets",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, env, err := secretScope(cmd, args[0])
			if err != nil {
				return err
			}
			values := map[string]string{}
			for _, pair := range args[1:] {
				key, value, ok := strings.Cut(pair, "=")
				if !ok || key == "" {
					return fmt.Errorf("expected KEY=value, got %q", pair)
				}
				values[key] = value
			}
			var result struct {
				Message string `json:"message"`
			}
			if err := apiJSON("PUT", envPath(app, env)+"/secrets", map[string]any{"values": values}, &result); err != nil {
				return err
			}
			fmt.Println(result.Message)
			return nil
		},
	}

	unsetSecretCmd := &cobra.Command{
		Use:   "unset <environment> KEY",
		Short: "Delete a secret",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, env, err := secretScope(cmd, args[0])
			if err != nil {
				return err
			}
			if err := apiJSON("DELETE", envPath(app, env)+"/secrets/"+args[1], nil, nil); err != nil {
				return err
			}
			fmt.Printf("Deleted %s from %s/%s\n", args[1], app, env)
			return nil
		},
	}

	for _, c := range []*cobra.Command{listSecretsCmd, setSecretsCmd, unsetSecretCmd} {
		c.Flags().String("app", "", "App name (default: from "+manifest.FileName+")")
		secretsCmd.AddCommand(c)
	}
	rootCmd.AddCommand(secretsCmd)
}
