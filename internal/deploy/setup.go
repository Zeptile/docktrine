package deploy

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/manifest"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// setupImage runs setup commands: the docker CLI with compose, on Alpine.
// Scripts like Sentry's install.sh want bash and GNU tools, which are added
// when the container starts.
const setupImage = "docker:cli"

// setupScript prepares the container and runs the command, in bash, from
// DOCKTRINE_SETUP so it needs no quoting. It stops at the first command
// that fails, so a failed install script fails the deploy.
const setupScript = `set -e
if ! command -v bash >/dev/null 2>&1 || ! sort --version-sort </dev/null >/dev/null 2>&1; then
  echo "Installing bash and coreutils for the setup command"
  apk add --no-cache --quiet bash coreutils
fi
exec bash -e -c "$DOCKTRINE_SETUP"`

// A setup command, Sentry's install among them, can take a long while:
// it pulls every image, builds some and migrates databases.
const setupDeployTimeout = 90 * time.Minute

// setupReason says why the setup command has to run on this deploy, or ""
// when the last successful deploy already ran it for this commit and command.
func (d *Deployer) setupReason(spec *manifest.Spec, deploymentID int64) (string, error) {
	setup := spec.Setup
	switch {
	case setup.Force:
		return "it was asked for on this deploy", nil
	case setup.Always:
		return "it is set to run on every deploy", nil
	}

	env, err := d.db.GetEnvironment(spec.App, spec.Environment)
	if err != nil || env == nil {
		return "this is the first deploy", err
	}
	history, err := d.db.ListDeployments(env.ID, 100)
	if err != nil {
		return "", err
	}
	for _, dep := range history {
		if dep.ID == deploymentID || dep.Status != database.DeploymentSucceeded {
			continue
		}
		prev := dep.Spec
		if prev == nil || prev.Setup == nil || prev.Source == nil {
			return "it hasn't run for this environment yet", nil
		}
		if prev.Setup.Command != setup.Command {
			return "the setup command changed", nil
		}
		if prev.Source.Commit != spec.Source.Commit {
			return fmt.Sprintf("the stack changed from %s to %s", short(prev.Source.Commit), short(spec.Source.Commit)), nil
		}
		return "", nil
	}
	return "this is the first deploy", nil
}

func short(commit string) string {
	return commit[:min(7, len(commit))]
}

// runSetup runs the setup command on the node, in dir, which is the stack's
// directory there, and copies its output into the deployment log.
func runSetup(ctx context.Context, cli *client.Client, t target, dir string, log *deployLog) error {
	if err := ensureImage(ctx, cli, setupImage); err != nil {
		return err
	}

	name := t.project() + "-setup"
	// A setup container left by a deploy that was cut short
	cli.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})

	created, err := cli.ContainerCreate(ctx,
		&container.Config{
			Image:      setupImage,
			Entrypoint: []string{"sh", "-c", setupScript},
			Cmd:        []string{},
			WorkingDir: dir,
			// The environment's variables and secrets, so the command can use
			// them, e.g. to create a first user; then what it needs to run
			Env: append(envList(t.env),
				"DOCKTRINE_SETUP="+t.spec.Setup.Command,
				// The compose project Docktrine runs the stack as, whatever
				// the stack's .env calls it
				"COMPOSE_PROJECT_NAME="+t.project(),
				"CI=true",
			),
			Labels: map[string]string{LabelApp: t.spec.App, LabelRole: "setup"},
		},
		&container.HostConfig{
			// The same path as on the node, so the compose files the command
			// runs mount the same directories Docktrine's do
			Binds: []string{dir + ":" + dir, "/var/run/docker.sock:/var/run/docker.sock"},
		},
		nil, nil, name)
	if err != nil {
		return fmt.Errorf("creating the setup container: %w", err)
	}
	defer removeContainer(cli, created.ID)

	wait, waitErr := cli.ContainerWait(ctx, created.ID, container.WaitConditionNextExit)
	if err := cli.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("starting the setup container: %w", err)
	}

	logs, err := cli.ContainerLogs(ctx, created.ID, container.LogsOptions{ShowStdout: true, ShowStderr: true, Follow: true})
	if err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer logs.Close()
		pr, pw := io.Pipe()
		go func() { pw.CloseWithError(demux(pw, logs)) }()
		scanner := bufio.NewScanner(pr)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		for scanner.Scan() {
			// Progress bars redraw a line with carriage returns; keep the last
			line := scanner.Text()
			if i := strings.LastIndex(line, "\r"); i >= 0 {
				line = line[i+1:]
			}
			line = strings.TrimSpace(ansi.ReplaceAllString(line, ""))
			if line != "" && !pullProgress.MatchString(line) {
				log.Printf("  | %s", line)
			}
		}
	}()

	select {
	case res := <-wait:
		<-done
		if res.Error != nil {
			return fmt.Errorf("the setup command: %s", res.Error.Message)
		}
		if res.StatusCode != 0 {
			return fmt.Errorf("the setup command exited with %d; its output is above", res.StatusCode)
		}
		return nil
	case err := <-waitErr:
		return err
	case <-ctx.Done():
		return fmt.Errorf("the setup command didn't finish in time: %w", ctx.Err())
	}
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

// Non-interactive flags install scripts commonly take. A script's own help
// text naming one means it takes it.
var unattendedFlags = []string{
	"--skip-user-creation",
	"--no-report-self-hosted-issues",
	"--apply-automatic-config-updates",
	"--skip-commit-check",
	"--non-interactive",
	"--unattended",
}

// suggestSetup proposes a setup command for a checkout whose compose file
// is in projectDir: its install script, with the flags that keep it from
// asking questions.
func suggestSetup(projectDir string) string {
	for _, name := range []string{"install.sh", "setup.sh"} {
		path := filepath.Join(projectDir, name)
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			continue
		}
		// The script, and the scripts it sources from a directory next to it
		text := readText(path)
		for _, dir := range []string{"install", "scripts", "setup"} {
			matches, _ := filepath.Glob(filepath.Join(projectDir, dir, "*.sh"))
			for _, m := range matches {
				text += readText(m)
			}
		}
		command := "./" + name
		for _, flag := range unattendedFlags {
			if strings.Contains(text, flag) {
				command += " " + flag
			}
		}
		return command
	}
	return ""
}

func readText(path string) string {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 1<<20 {
		return ""
	}
	return string(data)
}
