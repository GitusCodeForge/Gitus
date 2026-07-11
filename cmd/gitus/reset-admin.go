package main

import (
	"fmt"
	"os"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	dbinit "github.com/GitusCodeForge/Gitus/pkg/gitus/db/init"
	"github.com/GitusCodeForge/Gitus/routes"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

func ResetAdmin(ctx *routes.RouterContext) {
	if ctx.Config.OperationMode != gitus.OP_MODE_NORMAL {
		// TODO: add more info about how to set things up in simple mode.
		fmt.Printf("Configuration not in normal mode.")
		return
	}
	dbif, err := dbinit.InitializeDatabase(ctx.Config)
	if err != nil {
		fmt.Printf("Failed to connect to database while resetting admin: %s\n", err.Error())
		return
	}
	fmt.Printf("This utility is here for changing the password of the admin user.\n")
	fmt.Printf("Please enter a new password: ")
	s, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Printf("Failed to read password while resetting admin: %s\n", err.Error())
		return
	}
	hashedS, err := bcrypt.GenerateFromPassword(s, ctx.Config.PasswordHashStrength)
	if err != nil {
		fmt.Printf("Failed to hash password with bcrypt while resetting admin: %s\n", err.Error())
		return
	}
	err = dbif.UpdateUserPassword("admin", string(hashedS))
	if err != nil {
		fmt.Printf("Failed to update password while resetting admin: %s\n", err.Error())
		return
	}
	fmt.Printf("Done.\n")
}
