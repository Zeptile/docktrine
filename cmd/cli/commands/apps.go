package commands

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/manifest"
	"github.com/spf13/cobra"
)

type appSummary struct {
	Environment       database.Environment `json:"environment"`
	LatestDeployment  *database.Deployment `json:"latest_deployment"`
	CurrentDeployment *database.Deployment `json:"current_deployment"`
}

func init() {
	rootCmd.PersistentFlags().StringVar(&manifestFile, "file", manifest.FileName, "Path to the app manifest")

	initCmd := &cobra.Command{
		Use:   "init [app-name]",
		Short: "Create a " + manifest.FileName + " for the current repo",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := os.Stat(manifestFile); err == nil {
				return fmt.Errorf("%s already exists", manifestFile)
			}

			name := ""
			if len(args) > 0 {
				name = args[0]
			} else if wd, err := os.Getwd(); err == nil {
				name = manifest.Slugify(filepath.Base(wd))
			}

			compose := ""
			for _, candidate := range []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"} {
				if _, err := os.Stat(candidate); err == nil {
					compose = candidate
					break
				}
			}

			if err := os.WriteFile(manifestFile, []byte(manifest.Template(name, compose)), 0644); err != nil {
				return err
			}
			fmt.Printf("Created %s for app %q. Review the nodes and hostnames, then run: docktrine deploy dev\n", manifestFile, name)
			return nil
		},
	}

	deployCmd := &cobra.Command{
		Use:   "deploy <environment>",
		Short: "Build, push and deploy an environment from " + manifest.FileName,
		Long: `Builds the image (when the manifest has a build section), pushes it to the
central registry, and deploys it to the environment's node.

Use "preview" as the environment to deploy the current branch as a preview,
if the manifest defines one.`,
		Args: cobra.ExactArgs(1),
		RunE: runDeploy,
	}
	deployCmd.Flags().String("tag", "", "Image tag (default: git commit, with -dirty for uncommitted changes)")
	deployCmd.Flags().String("branch", "", "Branch for preview deployments (default: current git branch)")
	deployCmd.Flags().Bool("no-build", false, "Skip building and pushing; deploy an already pushed tag")
	deployCmd.Flags().Bool("detach", false, "Don't wait for the deployment to finish")

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show deployed environments",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var apps []appSummary
			if err := apiJSON("GET", "/apps", nil, &apps); err != nil {
				return err
			}

			filter, _ := cmd.Flags().GetString("app")
			all, _ := cmd.Flags().GetBool("all")
			if filter == "" && !all {
				if m, err := manifest.Load(manifestFile); err == nil {
					filter = m.App
				}
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "APP\tENVIRONMENT\tNODE\tSTATUS\tRUNNING\tDEPLOYED")
			for _, a := range apps {
				if filter != "" && a.Environment.App != filter {
					continue
				}
				status, deployed, running := "-", "-", "-"
				if a.LatestDeployment != nil {
					status = a.LatestDeployment.Status
				}
				if a.CurrentDeployment != nil {
					running = fmt.Sprintf("#%d %s", a.CurrentDeployment.ID, a.CurrentDeployment.Image)
					deployed = ago(a.CurrentDeployment.CreatedAt)
				}
				name := a.Environment.Name
				if a.Environment.ExpiresAt != nil {
					name += fmt.Sprintf(" (expires in %s)", time.Until(*a.Environment.ExpiresAt).Round(time.Minute))
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
					a.Environment.App, name, a.Environment.Server, status, running, deployed)
			}
			return w.Flush()
		},
	}
	statusCmd.Flags().String("app", "", "Only show this app (default: from "+manifest.FileName+")")
	statusCmd.Flags().Bool("all", false, "Show every app, even inside a repo")

	historyCmd := &cobra.Command{
		Use:   "history <environment>",
		Short: "Show the deployment history of an environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, env, err := targetEnvironment(cmd, args[0])
			if err != nil {
				return err
			}
			var detail struct {
				Environment database.Environment  `json:"environment"`
				Deployments []database.Deployment `json:"deployments"`
				Hostnames   []string              `json:"hostnames"`
				Containers  []struct {
					Name   string `json:"name"`
					State  string `json:"state"`
					Status string `json:"status"`
				} `json:"containers"`
				ContainersError string `json:"containers_error"`
			}
			if err := apiJSON("GET", envPath(app, env), nil, &detail); err != nil {
				return err
			}

			fmt.Printf("%s/%s on %s\n", app, env, detail.Environment.Server)
			if len(detail.Hostnames) > 0 {
				fmt.Printf("Hostnames: %s\n", strings.Join(detail.Hostnames, ", "))
			}
			if detail.ContainersError != "" {
				fmt.Printf("Containers: unavailable (%s)\n", detail.ContainersError)
			}
			for _, c := range detail.Containers {
				fmt.Printf("Container: %s  %s (%s)\n", c.Name, c.State, c.Status)
			}
			fmt.Println()

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "ID\tSTATUS\tIMAGE\tBY\tWHEN\tMESSAGE")
			for _, d := range detail.Deployments {
				marker := ""
				if detail.Environment.CurrentDeploymentID != nil && *detail.Environment.CurrentDeploymentID == d.ID {
					marker = " *"
				}
				fmt.Fprintf(w, "#%d%s\t%s\t%s\t%s\t%s\t%s\n",
					d.ID, marker, d.Status, d.Image, d.CreatedBy, ago(d.CreatedAt), d.Message)
			}
			return w.Flush()
		},
	}
	addAppFlags(historyCmd)

	logsCmd := &cobra.Command{
		Use:   "logs <environment>",
		Short: "Show the logs of an environment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, env, err := targetEnvironment(cmd, args[0])
			if err != nil {
				return err
			}
			follow, _ := cmd.Flags().GetBool("follow")
			tail, _ := cmd.Flags().GetInt("tail")

			path := fmt.Sprintf("%s%s/logs?tail=%d&follow=%t", apiURL, envPath(app, env), tail, follow)
			resp, err := makeRequest("GET", path, nil)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			if err := handleError(resp); err != nil {
				return err
			}
			_, err = io.Copy(os.Stdout, resp.Body)
			return err
		},
	}
	addAppFlags(logsCmd)
	logsCmd.Flags().BoolP("follow", "f", false, "Keep streaming new log lines")
	logsCmd.Flags().Int("tail", 200, "Number of lines to show from the end of the logs")

	rollbackCmd := &cobra.Command{
		Use:   "rollback <environment>",
		Short: "Redeploy the previous successful deployment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, env, err := targetEnvironment(cmd, args[0])
			if err != nil {
				return err
			}
			to, _ := cmd.Flags().GetInt64("to")

			var dep database.Deployment
			if err := apiJSON("POST", envPath(app, env)+"/rollback", map[string]int64{"deployment_id": to}, &dep); err != nil {
				return err
			}
			fmt.Printf("Rolling back %s/%s to %s (deployment #%d)\n", app, env, dep.Image, dep.ID)
			return followDeployment(dep.ID)
		},
	}
	addAppFlags(rollbackCmd)
	rollbackCmd.Flags().Int64("to", 0, "Deployment ID to roll back to (default: the previous successful one)")

	destroyCmd := &cobra.Command{
		Use:   "destroy <environment>",
		Short: "Remove an environment's containers and hostnames",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app, env, err := targetEnvironment(cmd, args[0])
			if err != nil {
				return err
			}
			purge, _ := cmd.Flags().GetBool("purge")
			yes, _ := cmd.Flags().GetBool("yes")

			what := "containers and hostnames"
			if purge {
				what += " AND VOLUMES (data will be lost)"
			}
			if !yes && !confirm(fmt.Sprintf("Destroy %s/%s: remove its %s?", app, env, what)) {
				return fmt.Errorf("aborted")
			}

			var result struct {
				Message string `json:"message"`
			}
			if err := apiJSON("DELETE", fmt.Sprintf("%s?purge=%t", envPath(app, env), purge), nil, &result); err != nil {
				return err
			}
			fmt.Println(result.Message)
			return nil
		},
	}
	addAppFlags(destroyCmd)
	destroyCmd.Flags().Bool("purge", false, "Also delete the environment's volumes")
	destroyCmd.Flags().BoolP("yes", "y", false, "Don't ask for confirmation")

	rootCmd.AddCommand(initCmd, deployCmd, statusCmd, historyCmd, logsCmd, rollbackCmd, destroyCmd)
}

func runDeploy(cmd *cobra.Command, args []string) error {
	m, err := manifest.Load(manifestFile)
	if err != nil {
		return err
	}
	// Paths in the manifest (build context, compose file) are relative to it.
	if dir := filepath.Dir(manifestFile); dir != "." {
		if err := os.Chdir(dir); err != nil {
			return err
		}
	}

	envName := args[0]
	branch, _ := cmd.Flags().GetString("branch")
	if envName == manifest.PreviewEnvironment && branch == "" {
		if branch, err = gitOutput("rev-parse", "--abbrev-ref", "HEAD"); err != nil {
			return fmt.Errorf("pass --branch: %w", err)
		}
	}

	image := m.Image
	if m.Build != nil {
		tag, _ := cmd.Flags().GetString("tag")
		if tag == "" {
			if tag, err = gitTag(); err != nil {
				return err
			}
		}

		var cfg struct {
			Registry string `json:"registry"`
		}
		if err := apiJSON("GET", "/config", nil, &cfg); err != nil {
			return err
		}
		if cfg.Registry == "" {
			return fmt.Errorf("the Docktrine server has no registry configured (DOCKTRINE_REGISTRY)")
		}
		image = fmt.Sprintf("%s/%s:%s", cfg.Registry, m.App, tag)

		if noBuild, _ := cmd.Flags().GetBool("no-build"); !noBuild {
			if err := buildAndPush(m.Build, image); err != nil {
				return err
			}
		}
	}

	spec, err := m.Resolve(envName, branch, image)
	if err != nil {
		return err
	}

	var dep database.Deployment
	if err := apiJSON("POST", "/deployments", spec, &dep); err != nil {
		return err
	}
	fmt.Printf("Deployment #%d of %s/%s started\n", dep.ID, spec.App, spec.Environment)

	if detach, _ := cmd.Flags().GetBool("detach"); detach {
		return nil
	}
	if err := followDeployment(dep.ID); err != nil {
		return err
	}
	for _, host := range spec.Hostnames {
		fmt.Printf("  https://%s\n", host)
	}
	return nil
}

func gitTag() (string, error) {
	sha, err := gitOutput("rev-parse", "--short=12", "HEAD")
	if err != nil {
		return "", fmt.Errorf("can't derive an image tag from git, pass --tag: %w", err)
	}
	if dirty, _ := gitOutput("status", "--porcelain"); dirty != "" {
		sha += "-dirty-" + time.Now().UTC().Format("20060102150405")
	}
	return sha, nil
}

func buildAndPush(build *manifest.Build, image string) error {
	dockerfile := build.Dockerfile
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}
	context := build.Context
	if context == "" {
		context = "."
	}
	platform := build.Platform
	if platform == "" {
		platform = "linux/amd64"
	}

	args := []string{"buildx", "build", "--platform", platform, "-f", dockerfile, "-t", image, "--push"}
	for k, v := range build.Args {
		args = append(args, "--build-arg", k+"="+v)
	}
	args = append(args, context)

	fmt.Printf("Building and pushing %s\n", image)
	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build failed (logged in to the registry? try `docker login`): %w", err)
	}
	return nil
}

// followDeployment prints new deployment log lines until it finishes, and
// fails if the deployment failed, so scripts and CI can rely on the exit code.
func followDeployment(id int64) error {
	printed := 0
	for {
		var dep database.Deployment
		if err := apiJSON("GET", fmt.Sprintf("/deployments/%d", id), nil, &dep); err != nil {
			return err
		}
		if len(dep.Log) > printed {
			fmt.Print(dep.Log[printed:])
			printed = len(dep.Log)
		}

		switch dep.Status {
		case database.DeploymentSucceeded:
			if dep.Message != "" {
				fmt.Printf("Note: %s\n", dep.Message)
			}
			return nil
		case database.DeploymentFailed:
			return fmt.Errorf("deployment #%d failed: %s", id, dep.Message)
		}
		time.Sleep(time.Second)
	}
}

func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
