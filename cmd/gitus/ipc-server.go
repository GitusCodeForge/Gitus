package main

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"net"
	"os/user"
	"path"
	"strconv"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/auxfuncs"
	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/routes"
)

func ipcServerLog(msg string) {
	log.Printf("[IPC_SERV] %s\n", msg)
}
func ipcServerPanic(msg string) {
	log.Panicf("[IPC_SERV] %s\n", msg)
}

var ERR_IPC_SERVER_TYPE_NOT_SUPPORTED = errors.New("unsupported IPC server type")

func StartIPCServer(ctx *routes.RouterContext) (net.Listener, error) {
	u, err := user.Lookup(ctx.Config.GitUser)
	if err != nil {
		ipcServerPanic(fmt.Sprintf("Somehow we reached this stage without having a good Git user: %s\n", err))
		return nil, err
	}
	ipcServerType := ctx.Config.IPCServer.Type
	if ipcServerType == "" {
		ipcServerType = gitus.IPC_SERVER_TYPE_UNIX
	}
	if ipcServerType == gitus.IPC_SERVER_TYPE_UNIX {
		p := ctx.Config.IPCServer.UnixSocketPath
		if p == "" {
			p = "gitus.lock"
		}
		if !path.IsAbs(p) {
			p = path.Join(u.HomeDir, p)
		}
		p = path.Clean(p)
		listener, err := net.Listen("unix", p)
		if err != nil {
			ipcServerPanic(fmt.Sprintf("Failed to start the socket server due to reason: %s\n", err))
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

func handleIPCServerConnection(ctx *routes.RouterContext, conn net.Conn) {
	ipcServerLog(fmt.Sprintf("Start handling client: %s", conn.RemoteAddr()))
	defer conn.Close()
	reader := bufio.NewReader(conn)
	s, err := reader.ReadString(0)
	if err != nil {
		ipcServerLog(fmt.Sprintf("Failed to read command: %s", err))
		return
	}
	s = s[:len(s)-1]
	// format:  {cmd}:{arg1},{arg2},...
	c := strings.Split(s, ":")
	if len(c) < 2 {
		ipcServerLog(fmt.Sprintf("Invalid incoming command: %s", s))
		return
	}
	_, err = strconv.Atoi(strings.TrimSpace(c[0]))
	if err != nil {
		ipcServerLog(fmt.Sprintf("Invalid incoming command: %s", s))
		return
	}
	vscmd := auxfuncs.ParseCSV(c[1])
	if len(vscmd) <= 0 {
		ipcServerLog(fmt.Sprintf("Invalid incoming command: %s", s))
		return
	}

	// TODO: fill in this.
}



