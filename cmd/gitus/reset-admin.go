package main

import (
	"fmt"
	"os"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/log"
	dbinit "github.com/GitusCodeForge/Gitus/pkg/gitus/db/init"
	"github.com/GitusCodeForge/Gitus/routes"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

func ResetAdmin(ctx *routes.RouterContext) {
	if ctx.Config.OperationMode != gitus.OP_MODE_FORGE {
		// TODO: add more info about how to set things up in host mode.
		log.Errorf("Configuration not in forge mode.")
		return
	}
	dbif, err := dbinit.InitializeDatabase(ctx.Config)
	if err != nil {
		log.Errorf("Failed to connect to database while resetting admin: %s", err.Error())
		return
	}
	log.Info("This utility is here for changing the password of the admin user.")
	fmt.Printf("Please enter a new password: ")
	s, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		log.Errorf("Failed to read password while resetting admin: %s", err.Error())
		return
	}
	hashedS, err := bcrypt.GenerateFromPassword(s, ctx.Config.PasswordHashStrength)
	if err != nil {
		log.Errorf("Failed to hash password with bcrypt while resetting admin: %s", err.Error())
		return
	}
	err = dbif.UpdateUserPassword("admin", string(hashedS))
	if err != nil {
		log.Errorf("Failed to update password while resetting admin: %s", err.Error())
		return
	}
	log.Info("Done.")
}
