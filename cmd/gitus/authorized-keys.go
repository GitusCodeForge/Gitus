package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/ssh"
	"github.com/GitusCodeForge/Gitus/routes"
)

// HandleAuthorizedKeys implements the sshd AuthorizedKeysCommand. sshd calls
// it with the SSH username and the public key (or fingerprint) the client
// offered; it prints the matching authorized_keys lines on stdout.
//
// With the builtin provider Gitus looks the key up in its own database, across
// all users, because the SSH username is the shared "git" account. With the
// command provider the configured executable does the lookup and its stdout is
// forwarded unchanged.
func HandleAuthorizedKeys(ctx *routes.RouterContext, username string, auth string) error {
	if ctx.Config.SSHAuth.UsesExternalProvider() {
		return runExternalSSHAuthProvider(ctx, username, auth)
	}
	if ctx.DatabaseInterface == nil {
		return errors.New("no database interface available")
	}
	keys, err := ctx.DatabaseInterface.GetAllAuthKey()
	if err != nil {
		return fmt.Errorf("failed to read SSH keys: %s", err)
	}
	for _, key := range keys {
		if ssh.MatchAuthorizedKey(key.KeyText, auth) {
			fmt.Println(ssh.AuthorizedKeyLine(ctx.Config.FilePath, key.UserName, key.KeyName, key.KeyText))
		}
	}
	return nil
}

// runExternalSSHAuthProvider runs the configured provider with the fixed args
// followed by the SSH username and the offered key/fingerprint. The provider's
// stdout is the authorized_keys output sshd expects. A non-zero exit or a
// timeout fails authentication.
func runExternalSSHAuthProvider(ctx *routes.RouterContext, username string, auth string) error {
	command := strings.TrimSpace(ctx.Config.SSHAuth.Command)
	if !filepath.IsAbs(command) {
		return errors.New("sshAuth.command must be an absolute path")
	}
	ctxTimeout, cancel := context.WithTimeout(context.Background(), ctx.Config.SSHAuth.EffectiveTimeout())
	defer cancel()
	args := append([]string{}, ctx.Config.SSHAuth.Args...)
	args = append(args, username, auth)
	cmd := exec.CommandContext(ctxTimeout, command, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		if ctxTimeout.Err() == context.DeadlineExceeded {
			return fmt.Errorf("ssh auth provider timed out after %s", ctx.Config.SSHAuth.EffectiveTimeout())
		}
		return err
	}
	return nil
}
