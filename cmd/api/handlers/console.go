package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Zeptile/docktrine/cmd/api/middleware"
	"github.com/Zeptile/docktrine/internal/console"
	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/docker"
	"github.com/Zeptile/docktrine/internal/logger"
	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

// Shell kinds and the role each needs. A container shell can do what a
// deployer can already do by redeploying; a node shell is root on the host.
const (
	shellContainer = "container"
	shellNode      = "node"
)

var shellRoles = map[string]string{
	shellContainer: database.RoleDeployer,
	shellNode:      database.RoleAdmin,
}

// canOpenShell is checked again inside the socket handler, so a route
// wired without its role middleware still can't hand out a root shell.
func canOpenShell(p *middleware.Principal, kind string) bool {
	role, ok := shellRoles[kind]
	return ok && p != nil && database.RoleAtLeast(p.Role(), role)
}

// ShellUpgrade checks a shell request before the WebSocket handshake: it
// must be a handshake, and from this site.
func (h *Handler) ShellUpgrade(c *fiber.Ctx) error {
	if !websocket.IsWebSocketUpgrade(c) {
		return c.Status(fiber.StatusUpgradeRequired).JSON(fiber.Map{"error": "Open this with a WebSocket"})
	}
	return middleware.SameOrigin()(c)
}

var shellSocket = websocket.Config{
	HandshakeTimeout: 10 * time.Second,
	Subprotocols:     []string{middleware.WebSocketProtocol},
	ReadBufferSize:   4 * 1024,
	WriteBufferSize:  32 * 1024,
}

// ContainerShell godoc
// @Summary Open a shell in a container
// @Description WebSocket. Binary messages are terminal bytes both ways; text messages are JSON, {"type":"resize","cols":N,"rows":N} from the client and ready, exit, closed or error to it. Needs the deployer role.
// @Tags containers
// @Param id path string true "Container ID or name"
// @Param server query string false "Server name"
// @Param cols query int false "Terminal columns"
// @Param rows query int false "Terminal rows"
// @Success 101
// @Failure 403 {object} interface{}
// @Router /containers/{id}/shell [get]
func (h *Handler) ContainerShell() fiber.Handler {
	return websocket.New(func(conn *websocket.Conn) {
		h.serveShell(conn, shellContainer, conn.Query("server"), conn.Params("id"))
	}, shellSocket)
}

// NodeShell godoc
// @Summary Open a root shell on a server
// @Description WebSocket, with the same messages as a container shell. Runs in a privileged helper container that enters the host's namespaces, removed when the socket closes. Needs the admin role.
// @Tags servers
// @Param name path string true "Server name"
// @Param cols query int false "Terminal columns"
// @Param rows query int false "Terminal rows"
// @Success 101
// @Failure 403 {object} interface{}
// @Router /servers/{name}/shell [get]
func (h *Handler) NodeShell() fiber.Handler {
	return websocket.New(func(conn *websocket.Conn) {
		h.serveShell(conn, shellNode, conn.Params("name"), "")
	}, shellSocket)
}

func (h *Handler) serveShell(conn *websocket.Conn, kind, serverName, containerID string) {
	p, _ := conn.Locals("principal").(*middleware.Principal)
	if !canOpenShell(p, kind) {
		shellError(conn, "This needs the "+shellRoles[kind]+" role")
		return
	}
	who := p.Name()

	server, err := h.db.ResolveServer(serverName)
	if err != nil {
		shellError(conn, err.Error())
		return
	}
	cli, err := docker.Connect(server)
	if err != nil {
		shellError(conn, err.Error())
		return
	}
	defer cli.Close()

	cols, rows := console.ParseSize(conn.Query("cols"), conn.Query("rows"))
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	var sh console.Shell
	var target string
	if kind == shellNode {
		target = "on " + server.Name
		// Leftovers from an API that died mid-session
		if n, err := console.Sweep(ctx, cli, h.shellOwner()); err == nil && n > 0 {
			logger.Info(fmt.Sprintf("Removed %d leftover shell helpers on %s", n, server.Name))
		}
		sh, err = console.OpenNode(ctx, cli, cols, rows, map[string]string{
			console.LabelConsoleUser:     who,
			console.LabelConsoleInstance: h.shellOwner().Instance,
		})
	} else {
		var name string
		sh, name, err = console.OpenContainer(ctx, cli, containerID, cols, rows)
		target = fmt.Sprintf("in container %s on %s", firstNonEmpty(name, containerID), server.Name)
	}
	cancel()
	if err != nil {
		logger.Warn(fmt.Sprintf("Shell for %s %s failed to open: %v", who, target, err))
		shellError(conn, err.Error())
		return
	}

	opened := time.Now()
	logger.Info(fmt.Sprintf("Shell opened by %s %s [IP: %s]", who, target, conn.IP()))
	res := console.Bridge(conn, sh, console.Options{})
	logger.Info(fmt.Sprintf("Shell closed for %s %s after %s: %s",
		who, target, time.Since(opened).Round(time.Second), res))
}

// shellError tells the terminal why there's no shell, then hangs up.
func shellError(conn *websocket.Conn, msg string) {
	data, _ := json.Marshal(console.Message{Type: console.TypeError, Message: msg})
	conn.WriteMessage(websocket.TextMessage, data)
	conn.WriteControl(websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	conn.Close()
}

// StartShellJanitor removes node shell helpers left behind on any server,
// at startup and then hourly. Helpers end with their session, and with
// their stdin if the API dies; this catches whatever is left.
func (h *Handler) StartShellJanitor(ctx context.Context) {
	h.shellStarted = time.Now()
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			h.sweepShells()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (h *Handler) sweepShells() {
	servers, err := h.db.GetServers()
	if err != nil {
		logger.Error(err, "Failed to list servers for the shell sweep")
		return
	}
	for i := range servers {
		server := &servers[i]
		cli, err := docker.Connect(server)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		if n, err := console.Sweep(ctx, cli, h.shellOwner()); err == nil && n > 0 {
			logger.Info(fmt.Sprintf("Removed %d leftover shell helpers on %s", n, server.Name))
		}
		cancel()
		cli.Close()
	}
}

// shellOwner identifies this API's node shell helpers. Without a key (in
// tests) it claims none, and only age decides.
func (h *Handler) shellOwner() console.Owner {
	if h.box == nil {
		return console.Owner{}
	}
	return console.Owner{Instance: h.box.Fingerprint("node shells"), Started: h.shellStarted}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
