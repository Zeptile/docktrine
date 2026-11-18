package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/Zeptile/docktrine/internal/manifest"
	"github.com/spf13/cobra"
)

// apiJSON calls the API with an optional JSON body and decodes the response.
func apiJSON(method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	resp, err := makeRequest(method, apiURL+path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if err := handleError(resp); err != nil {
		return err
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func envPath(app, env string) string {
	return "/apps/" + url.PathEscape(app) + "/" + url.PathEscape(env)
}

var manifestFile string

// addAppFlags lets a command target an app from docktrine.yaml or --app.
func addAppFlags(cmd *cobra.Command) {
	cmd.Flags().String("app", "", "App name (default: from "+manifest.FileName+")")
	cmd.Flags().String("branch", "", "Branch of a preview environment (default: current git branch)")
}

// targetEnvironment resolves the app and environment a command acts on.
// "preview" selects the preview instance of the given (or current) branch.
func targetEnvironment(cmd *cobra.Command, env string) (string, string, error) {
	app, _ := cmd.Flags().GetString("app")
	if app == "" {
		m, err := manifest.Load(manifestFile)
		if err != nil {
			return "", "", fmt.Errorf("pass --app or run inside a repo with %s: %w", manifest.FileName, err)
		}
		app = m.App
	}

	if env == manifest.PreviewEnvironment {
		branch, _ := cmd.Flags().GetString("branch")
		if branch == "" {
			var err error
			if branch, err = gitOutput("rev-parse", "--abbrev-ref", "HEAD"); err != nil {
				return "", "", fmt.Errorf("pass --branch: %w", err)
			}
		}
		env = "preview-" + manifest.Slugify(branch)
	}
	return app, env, nil
}

func gitOutput(args ...string) (string, error) {
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func confirm(prompt string) bool {
	fmt.Fprintf(os.Stderr, "%s [y/N] ", prompt)
	var answer string
	fmt.Scanln(&answer)
	return strings.EqualFold(answer, "y") || strings.EqualFold(answer, "yes")
}
