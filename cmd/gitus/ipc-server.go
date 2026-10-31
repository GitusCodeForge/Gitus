package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/auxfuncs"
	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/routes"
)

func ipcServerLog(msg string) {
	log.Printf("[IPC_SERV] %s\n", msg)
}

// removeStaleIPCSocket deletes p when nothing is listening on it, and reports
// whether it did. A live socket is left alone.
func removeStaleIPCSocket(p string) bool {
	conn, err := net.DialTimeout("unix", p, 500*time.Millisecond)
	if err == nil {
		conn.Close()
		return false
	}
	if err = os.Remove(p); err != nil { return false }
	ipcServerLog(fmt.Sprintf("Removed stale socket %s", p))
	return true
}

var ERR_IPC_SERVER_TYPE_NOT_SUPPORTED = errors.New("unsupported IPC server type")

// ipcServerSocketPath is where the IPC socket lives. Both the server and the
// clients (which can be other instances of this executable, e.g. the push
// hooks) have to agree on it, hence the single helper.
func ipcServerSocketPath(ctx *routes.RouterContext) string {
	p := ctx.Config.IPCServer.UnixSocketPath
	if p == "" {
		p = "gitus.lock"
	}
	return ctx.ResolvePathAgainstHome(p)
}

func StartIPCServer(ctx *routes.RouterContext) (net.Listener, error) {
	ipcServerType := ctx.Config.IPCServer.Type
	if ipcServerType == "" {
		ipcServerType = gitus.IPC_SERVER_TYPE_UNIX
	}
	if ipcServerType == gitus.IPC_SERVER_TYPE_UNIX {
		p := ipcServerSocketPath(ctx)
		listener, err := net.Listen("unix", p)
		if err != nil && removeStaleIPCSocket(p) {
			// A socket file left behind by an instance that did not shut down
			// cleanly makes Listen fail. Nothing is accepting on it, so drop it
			// and try once more.
			listener, err = net.Listen("unix", p)
		}
		if err != nil {
			// NOTE: main() only warns about this and keeps going; the server is
			// still perfectly usable without the IPC channel, it just cannot be
			// warmed up from the outside.
			ipcServerLog(fmt.Sprintf("Failed to start the socket server: %s", err.Error()))
			return nil, err
		}
		go func(){
			ipcServerLog("Start accepting connections")
			for {
				conn, err := listener.Accept()
				if err != nil {
					ipcServerLog(fmt.Sprintf("Failed to accept client: %s", err))
					break
				}
				go handleIPCServerConnection(ctx, conn)
			}
		}()
		
		return listener, nil
	} else {
		return nil, ERR_IPC_SERVER_TYPE_NOT_SUPPORTED
	}
}

const (
	// IPC_COMMAND_WARM_LAST_COMMIT_CACHE asks the running web server to resolve
	// the last commit of a directory listing ahead of time, so that the first
	// visitor of a freshly pushed commit does not have to walk the history.
	//
	// It is a hint, not a source of truth: the listing is always computed on
	// demand when it is missing, so a message that never arrives (web server
	// down, hook failed, push done by hand) only costs one slow page.
	//
	// The value is 2, and 1 is not reused: it was the never implemented
	// global-cache update command, and keeping 2 means a hook or CLI installed
	// by an older build still agrees with a newer server during an upgrade.
	IPC_COMMAND_WARM_LAST_COMMIT_CACHE = 2
)

func handleIPCServerConnection(ctx *routes.RouterContext, conn net.Conn) {
	ipcServerLog(fmt.Sprintf("Start handling client: %s", conn.RemoteAddr()))
	defer conn.Close()
	reader := bufio.NewReader(conn)
	s, err := reader.ReadString(0)
	if err != nil {
		// A peer that connects and hangs up without saying anything is normal
		// (it is also how a socket is probed for liveness).
		if !errors.Is(err, io.EOF) {
			ipcServerLog(fmt.Sprintf("Failed to read command: %s", err))
		}
		return
	}
	s = s[:len(s)-1]
	// format:  {cmd}:{arg1},{arg2},...
	// NOTE: only the first colon separates the command from its arguments; a
	// repository's full name may contain colons of its own.
	c := strings.SplitN(s, ":", 2)
	if len(c) < 2 {
		ipcServerLog(fmt.Sprintf("Invalid incoming command: %s", s))
		return
	}
	cmdType, err := strconv.Atoi(strings.TrimSpace(c[0]))
	if err != nil {
		ipcServerLog(fmt.Sprintf("Invalid incoming command: %s", s))
		return
	}
	vscmd := auxfuncs.ParseCSV(c[1])
	if len(vscmd) <= 0 {
		ipcServerLog(fmt.Sprintf("Invalid incoming command: %s", s))
		return
	}
	switch cmdType {
	case IPC_COMMAND_WARM_LAST_COMMIT_CACHE:
		// 2:{repo_full_name},{commit_id}
		if len(vscmd) < 2 {
			ipcServerLog(fmt.Sprintf("Invalid last-commit cache warm-up command: %s", s))
			return
		}
		warmLastCommitCache(ctx, vscmd[0], vscmd[1])
	}
}

// warmLastCommitCache resolves the root listing of a commit and lets the
// resolver memoize it. Errors are only logged: warming up is best effort.
func warmLastCommitCache(ctx *routes.RouterContext, repoFullName string, commitId string) {
	if ctx.DatabaseInterface == nil { return }
	ns, name := splitRepoFullName(ctx, repoFullName)
	repo, err := ctx.DatabaseInterface.GetRepositoryByName(ns, name)
	if err != nil {
		ipcServerLog(fmt.Sprintf("Failed to warm last-commit cache: cannot find repository %s: %s", repoFullName, err.Error()))
		return
	}
	rr, ok := repo.Repository.(*gitlib.LocalGitRepository)
	if !ok {
		ipcServerLog(fmt.Sprintf("Failed to warm last-commit cache: %s is not a git repository", repoFullName))
		return
	}
	o, err := rr.ReadObject(commitId)
	if err != nil {
		ipcServerLog(fmt.Sprintf("Failed to warm last-commit cache: cannot read commit %s: %s", commitId, err.Error()))
		return
	}
	cobj, ok := o.(*gitlib.CommitObject)
	if !ok {
		ipcServerLog(fmt.Sprintf("Failed to warm last-commit cache: %s is not a commit", commitId))
		return
	}
	_, err = rr.ResolveLastCommitAtDir(cobj, "")
	if err != nil {
		ipcServerLog(fmt.Sprintf("Failed to warm last-commit cache for %s@%s: %s", repoFullName, commitId, err.Error()))
		return
	}
	ipcServerLog(fmt.Sprintf("Warmed last-commit cache for %s@%s", repoFullName, commitId))
}

// splitRepoFullName splits the "namespace:name" form used in commands and URLs.
func splitRepoFullName(ctx *routes.RouterContext, repoFullName string) (string, string) {
	if !ctx.Config.UseNamespace { return "", repoFullName }
	r := strings.SplitN(repoFullName, ":", 2)
	if len(r) < 2 { return "", repoFullName }
	return r[0], r[1]
}

const ipcCommandTimeout = 3 * time.Second

// sendIPCCommand is the client side of the IPC channel. It is fire and forget:
// callers are expected to treat a failure as "the hint did not get through",
// not as an error of the operation they were performing.
func sendIPCCommand(ctx *routes.RouterContext, cmdType int, args ...string) error {
	t := ctx.Config.IPCServer.Type
	if t != "" && t != gitus.IPC_SERVER_TYPE_UNIX {
		return ERR_IPC_SERVER_TYPE_NOT_SUPPORTED
	}
	conn, err := net.DialTimeout("unix", ipcServerSocketPath(ctx), ipcCommandTimeout)
	if err != nil { return err }
	defer conn.Close()
	conn.SetWriteDeadline(time.Now().Add(ipcCommandTimeout))
	fields := make([]string, 0, len(args))
	for _, a := range args {
		fields = append(fields, escapeIPCArgument(a))
	}
	_, err = conn.Write([]byte(fmt.Sprintf("%d:%s\x00", cmdType, strings.Join(fields, ","))))
	return err
}

func escapeIPCArgument(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, ",", `\,`)
	return s
}
