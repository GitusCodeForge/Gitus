package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"crypto/rand"
	"os"
	"os/exec"
	"os/user"
	"path"
	"strconv"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/templates"
	"golang.org/x/crypto/bcrypt"
)

const passchdict = "abcdefghijklmnopqrstuvwxyz0123456789!@#$%_-"
func mkpass() string {
	res := make([]byte, 0)
	dlen := big.NewInt(int64(len(passchdict)))
	for range 16 {
		r, _ := rand.Int(rand.Reader, dlen)
		res = append(res, passchdict[r.Uint64()])
	}
	return string(res)
}

func whereIs(cmdname string) (string, error) {
	cmd := exec.Command("whereis", "-b", cmdname)
	out, err := cmd.Output()
	if err != nil { return "", err }
	s := strings.Split(string(out), ":")
	preres := strings.TrimSpace(s[1])
	return preres, nil
}

func createOtherOwnedFile(p string, uids string, gids string) error {
	uid, _ := strconv.Atoi(uids)
	gid, _ := strconv.Atoi(gids)
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		if os.IsExist(err) {
			os.Remove(p)
			f, err = os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
			if err != nil { return err }
		} else {
			return err
		}
	}
	f.WriteString("")
	f.Close()
	err = os.Chown(p, uid, gid)
	if err != nil { return err }
	return nil
}

func createOtherOwnedDirectory(p string, uids string, gids string) error {
	uid, _ := strconv.Atoi(uids)
	gid, _ := strconv.Atoi(gids)
	err := os.MkdirAll(p, os.ModeDir|0755)
	if err != nil && !os.IsExist(err) { return err }
	err = os.Chown(p, uid, gid)
	if err != nil { return err }
	return nil
}

func normalModeGitusReadyCheck(ctx routes.RouterContext) (bool, error) {
	dbif := ctx.DatabaseInterface
	ssif := ctx.SessionInterface
	cfg := ctx.Config
	if dbif != nil {
		b, err := dbif.IsDatabaseUsable()
		if err != nil { return b, err }
		if !b { return false, errors.New("Database not usable") }
		_, err = os.ReadDir(cfg.GitRoot)
		if err != nil {
			if os.IsNotExist(err) { return false, errors.New("Git root does not exist") }
			return false, err
		}
	}
	if ssif != nil {
		b, err := ssif.IsSessionStoreUsable()
		if err != nil { return b, err }
		if !b { return false, errors.New("Session store not usable") }
	}
	if ctx.ReceiptSystem != nil {
		b, err := ctx.ReceiptSystem.IsReceiptSystemUsable()
		if err != nil { return b, err }
		if !b { return false, errors.New("Receipt system not usable") }
	}
	gitUserName := strings.TrimSpace(ctx.Config.GitUser)
	if len(gitUserName) <= 0 {
		u, _ := user.Current()
		ctx.Config.GitUser = u.Name
		fmt.Printf("You haven't configure any Git User. We've decided to set the field with your user name instead. If you don't want this behaviour, please change the configuration file after this.\n")
		ctx.Config.Sync()
	} else {
		_, err := user.Lookup(ctx.Config.GitUser)
		if err != nil {
			return false, fmt.Errorf("%s cannot be found in passwd", ctx.Config.GitUser)
		}
	}
	return true, nil
}

func askYesNo(prompt string) bool {
	fmt.Printf("%s [y/n] ", prompt)
	result := false
	for {
		var answer string
		_, err := fmt.Scan(&answer)
		if err != nil { log.Panic(err) }
		if answer == "y" || answer == "Y" {
			result = true
			break
		} else if answer == "n" || answer == "N" {
			result = false
			break
		} else {
			fmt.Print("Please enter y or n... [y/n] ")
		}
	}
	return result
}

func askString(prompt string, defaultResult string) (string, error) {
	fmt.Printf("%s ", prompt)
	fmt.Printf("[%s] ", defaultResult)
	res := make([]byte, 0)
	buf := make([]byte, 1)
	for {
		_, err := io.ReadFull(os.Stdin, buf)
		if err != nil {
			if err == io.EOF { break }
			return "", err
		}
		if buf[0] == byte('\n') { break }
		res = append(res, buf[0])
	}
	if len(res) <= 0 { return defaultResult, nil }
	return string(res), nil
}

func gitUserSetupCheckPrompt() {
	fmt.Println()
	fmt.Printf("You need to check if the Git user is set up properly:\n")
	fmt.Printf("1.  Make sure that both the Git user and the user running Gitus has full read/write permission of the Git Root specified in the config file\n")
	fmt.Printf("2.  Make sure that the user running Gitus has full read/write permission of the `.ssh/authorized_keys` file under the home directory of the Git user. \n")
	fmt.Printf("3.  Make sure the git shell commands are properly set up. This includes:\n")
	fmt.Printf("    1.  A `git-shell-command` directory exists under the home directory of the Git user.\n")
	fmt.Printf("    2.  A `no-interactive-login` file exists under the `git-shell-command` directory. This file needs to be executable. This is used to stop the original git-shell from providing things. It can be a simple shell script that calls the command `gitus no-login`.\n")
	fmt.Printf("    3.  The `gitus` executable should be under the `git-shell-command` directory as well.\n")
	fmt.Printf("4.  Make sure that the Gitus user can access the static assets directory\n")
}

func gitUserCheck(ctx routes.RouterContext) bool {
	gitUser, err := user.Lookup(ctx.Config.GitUser)
	br := bufio.NewReader(os.Stdin)
	if err != nil {
		r := askYesNo(fmt.Sprintf("User %s does not exist. Create it?", ctx.Config.GitUser))
		if !r {
			gitUserSetupCheckPrompt()
			return false
		}
		// find git-shell.
		gitShellPath, err := whereIs("git-shell")
		if err != nil {
			fmt.Printf("Failed to search for git-shell: %s\n", err.Error())
			gitUserSetupCheckPrompt(); return false
		}
		if len(gitShellPath) <= 0 {
			fmt.Printf("Failed to find the path of the git-shell executable.\n")
			gitUserSetupCheckPrompt(); return false
		}
		fmt.Printf("Please input the path of the home directory we're going to create for the Git user: [/home/%s] ", ctx.Config.GitUser)
		line, _, err := br.ReadLine()
		if err != nil {
			fmt.Printf("Failed to read line: %s\n", err.Error())
			gitUserSetupCheckPrompt()
			return false
		}
		p := strings.TrimSpace(string(line))
		if len(p) <= 0 {
			p = fmt.Sprintf("/home/%s", ctx.Config.GitUser)
		}
		err = os.MkdirAll(p, os.ModeDir|0755)
		if err != nil {
			fmt.Printf("Failed to create home directory: %s\n", err.Error())
			gitUserSetupCheckPrompt()
			return false
		}
		// find useradd
		useraddPath, err := whereIs("useradd")
		if err != nil {
			fmt.Printf("Failed to search for useradd: %s\n", err.Error())
			gitUserSetupCheckPrompt(); return false
		}
		if len(useraddPath) <= 0 {
			fmt.Printf("Failed to search for useradd.")
			gitUserSetupCheckPrompt(); return false
		}
		cmd3 := exec.Command(useraddPath, "-d", p, "-m", "-s", gitShellPath, ctx.Config.GitUser)
		err = cmd3.Run()
		if err != nil {
			fmt.Printf("Failed to run useradd: %s\n", err.Error())
			gitUserSetupCheckPrompt(); return false
		}
	}
	homeDir := gitUser.HomeDir
	if homeDir == "" {
		fmt.Printf("Cannot find the home directory for the Git user. ")
		res := askYesNo("Should I set it up for you?")
		if !res { gitUserSetupCheckPrompt(); return false }
		fmt.Printf("Please input the path of the home directory we're going to create for the Git user: [/home/%s] ", ctx.Config.GitUser)
		line, _, err := br.ReadLine()
		if err != nil {
			fmt.Printf("Failed to read line: %s\n", err.Error())
			gitUserSetupCheckPrompt()
			return false
		}
		p := strings.TrimSpace(string(line))
		if len(p) <= 0 {
			p = fmt.Sprintf("/home/%s", ctx.Config.GitUser)
		}
		err = os.MkdirAll(p, os.ModeDir|0755)
		if err != nil {
			fmt.Printf("Failed to create home directory: %s\n", err.Error())
			gitUserSetupCheckPrompt()
			return false
		}
		homeDir = p
	}
	err = createOtherOwnedDirectory(homeDir, gitUser.Uid, gitUser.Gid)
	if err != nil {
		fmt.Printf("Cannot set the true owner of the Git user's home directory.")
		gitUserSetupCheckPrompt(); return false
	}

	gitShellCommandPath := path.Join(homeDir, "git-shell-commands")
	err = createOtherOwnedDirectory(gitShellCommandPath, gitUser.Uid, gitUser.Gid)
	if err != nil {
		fmt.Printf("Failed to create the git-shell-commands directory: %s\n", err.Error())
		gitUserSetupCheckPrompt(); return false
	}

	sshPath := path.Join(homeDir, ".ssh")
	err = createOtherOwnedDirectory(sshPath, gitUser.Uid, gitUser.Gid)
	if err != nil {
		fmt.Printf("Failed to create the .ssh directory: %s\n", err.Error())
		gitUserSetupCheckPrompt(); return false
	}

	authorizedKeysPath := path.Join(homeDir, ".ssh", "authorized_keys")
	err = createOtherOwnedFile(authorizedKeysPath, gitUser.Uid, gitUser.Gid)
	if err != nil {
		fmt.Printf("Failed to create the authorized_keys file: %s\n", err.Error())
		gitUserSetupCheckPrompt(); return false
	}

	// copy gitus executable... for handling git over ssh.
	s, err := os.Executable()
	if err != nil {
		fmt.Printf("Failed to copy Gitus executable: %s\n", err.Error())
		gitUserSetupCheckPrompt(); return false
	}
	gitusPath := path.Join(homeDir, "git-shell-commands", "gitus")
	if gitusPath == s { return true }
	f, err := os.Open(s)
	if err != nil {
		fmt.Printf("Failed to copy Gitus executable: %s\n", err.Error())
		gitUserSetupCheckPrompt(); return false
	}
	defer f.Close()
	fout, err := os.OpenFile(gitusPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0754)
	if err != nil {
		fmt.Printf("Failed to copy Gitus executable: %s\n", err.Error())
		gitUserSetupCheckPrompt(); return false
	}
	defer fout.Close()
	_, err = io.Copy(fout, f)
	if err != nil {
		fmt.Printf("Failed to copy Gitus executable: %s\n", err.Error())
		gitUserSetupCheckPrompt(); return false
	}
	guUid, _ := strconv.Atoi(gitUser.Uid)
	guGid, _ := strconv.Atoi(gitUser.Gid)
	err = os.Chown(gitusPath, guUid, guGid)
	if err != nil {
		fmt.Printf("Failed to chown Gitus executable: %s\n", err.Error())
		gitUserSetupCheckPrompt(); return false
	}
	fmt.Printf("Done.\n")
	return true
}

// NOTE: you shouldn't check for plain mode here (instead - check it
// at the *caller* side!) since plain mode is fully "passive" and will
// not involve any database setup.
func InstallGitus(ctx routes.RouterContext) {
	if len(strings.TrimSpace(ctx.Config.GitUser)) <= 0 {
		fmt.Printf("Plain mode disabled but empty Git user name... this won't do. We'll assume the name of the Git user is `git`.\n")
		gitUserName := "git"
		res := askYesNo("Continue with `git`?")
		if !res {
			res = askYesNo("Specify the user name yourself? ")
			if !res {
				fmt.Printf("Please config a Git user name, or in the case you only need a frontend like git-instaweb, enable plain mode.\n")
				return
			}
			fmt.Print("Please input the Git user name of choice: ")
			_, err := fmt.Scan(&gitUserName)
			if err != nil {
				fmt.Printf("Failed to read Git user name: %s\n", err.Error())
				return
			}
			ctx.Config.GitUser = gitUserName
			err = ctx.Config.Sync()
			if err != nil {
				fmt.Printf("Failed to sync config: %s\n", err.Error())
				return
			}
		} else {
			ctx.Config.GitUser = "git"
			err := ctx.Config.Sync()
			if err != nil {
				fmt.Printf("Failed to sync config: %s\n", err.Error())
				return
			}
		}
	}
	if !gitUserCheck(ctx) {
		fmt.Printf("Failed to set up Git user. Please follow whatever instructions listed above and try again...")
		return
	}
	
	dbif := ctx.DatabaseInterface
	ssif := ctx.SessionInterface
	cfg := ctx.Config
	fmt.Println("If you've reached this point, it means the database is there but not ready, or you have invoked the install command manually.")


	fmt.Println("Checking specified Git root...")
	_, err := os.ReadDir(cfg.GitRoot)
	if os.IsNotExist(err) {
		fmt.Println("The root for storing Git repository according to the config file would be:")
		fmt.Printf("\t%s\n\n", cfg.GitRoot)
		fmt.Println("This folder does not exist yet; we will create it for you.")
		err = os.MkdirAll(cfg.GitRoot, os.ModeDir|0755)
		if err != nil {
			log.Panic(err)
		}
		fmt.Println("Git root creation done.")
	}

	// setting up database
	fmt.Println("Setting up database...")
	if len(cfg.Database.Type) <= 0 {
		fmt.Print("Cannot infer database interface since database type empty in config. Please fix it and try again.")
		os.Exit(1)
	}
	s, err := dbif.IsDatabaseUsable()
	if err != nil { log.Panic(err) }
	if !s {
		fmt.Println("Setting up tables...")
		err = dbif.InstallTables()
		if err != nil {
			log.Panic(err)
		}
	}	

	// setting up session store
	fmt.Println("Setting up session store...")
	if len(cfg.Session.Type) <= 0 {
		fmt.Print("Cannot infer session interface since session type empty in config. Please fix it and try again.")
		os.Exit(1)
	}
	s, err = ssif.IsSessionStoreUsable()
	if err != nil { log.Panic(err) }
	if !s {
		fmt.Println("Setting up session store...")
		err = ssif.Install()
		if err != nil {
			log.Panic(err)
		}
	}

	// setting up receipt system
	fmt.Println("Setting up receipt system...")
	if len(cfg.ReceiptSystem.Type) <= 0 {
		fmt.Print("Cannot infer receipt system interface since type is empty in config. Please fix it and try again.")
		os.Exit(1)
	}
	s, err = ctx.ReceiptSystem.IsReceiptSystemUsable()
	if err != nil { log.Panic(err) }
	if !s {
		fmt.Println("Setting up receipt...")
		err = ctx.ReceiptSystem.Install()
		if err != nil {
			log.Panic(err)
		}
	}
	
	// setting up admin user
	fmt.Println("Setting up admin user...")
	adminExists := false
	_, err = dbif.GetUserByName("admin")
	if err == db.ErrEntityNotFound {
		adminExists = false
	} else if err != nil {
		log.Panic(err)
	} else {
		adminExists = true
	}
	reinstallAdmin := true
	if adminExists {
		r := askYesNo("Admin user already exist; reinitialize?")
		reinstallAdmin = r
	}
	if reinstallAdmin {
		if adminExists {
			err = dbif.HardDeleteUserByName("admin")
			if err != nil { log.Panic(err) }
		}
		userPassword := mkpass()
		r, err := bcrypt.GenerateFromPassword([]byte(userPassword), bcrypt.DefaultCost)
		if err != nil {
			log.Panicf("Failed to generate password: %s\n", err.Error())
		}
		_, err = dbif.RegisterUser("admin", "", string(r), model.SUPER_ADMIN)
		if err != nil {
			log.Panicf("Failed to register user: %s\n", err.Error())
		}
		fmt.Print(`Admin user setup complete. Please use the reset-admin command to change the password:

    gitus -config [config-path] reset-admin

`)
	}

	fmt.Println("Setting up static assets used by the web UI...")
	
	// when we reached here, gitUser shouldn't be nil, since if it's nil we
	// would've created it with the code above.
	gitUser, _ := user.Lookup(ctx.Config.GitUser)
	staticPath := path.Join(gitUser.HomeDir, "gitus-static")
	err = templates.UnpackStaticFileTo(staticPath)
	if err != nil {
		fmt.Printf("Failed to setup static assets: %s\n", err.Error())
		fmt.Printf("You need to fix the problem or run the installer again, or set up the static assets manually.\n")
	}
	ctx.Config.StaticAssetDirectory = staticPath
	ctx.Config.Sync()

	// one last chown just to be sure.
	// we do things that we can do, but if we ever fail we don't bother and
	// leave the job of checking to the user.
	guUid, err := strconv.Atoi(gitUser.Uid)
	guGid, err := strconv.Atoi(gitUser.Gid)
	if ctx.Config.Database.Type == "sqlite" {
		err = os.Chown(ctx.Config.Database.Path, guUid, guGid)
	}
	if ctx.Config.Session.Type == "sqlite" {
		err = os.Chown(ctx.Config.Session.Path, guUid, guGid)
	}
	
	fmt.Println("Done. Please restart the program to start the server.")
	gitUserSetupCheckPrompt()
}

