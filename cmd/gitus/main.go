package main

import (
	gocontext "context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"path"
	"syscall"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/confirm_code"
	dbinit "github.com/GitusCodeForge/Gitus/pkg/gitus/db/init"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/mail"
	rsinit "github.com/GitusCodeForge/Gitus/pkg/gitus/receipt/init"
	ssinit "github.com/GitusCodeForge/Gitus/pkg/gitus/session/init"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/ssh"
	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/routes"
	"github.com/GitusCodeForge/Gitus/routes/controller"
	"github.com/GitusCodeForge/Gitus/templates"
)

func main() {
	argparse := flag.NewFlagSet("gitus", flag.ContinueOnError)
	argparse.Usage = func() {
		fmt.Fprintf(argparse.Output(), "Usage: gitus [flags] [config]\n")
		argparse.PrintDefaults()
	}
	initFlag := argparse.Bool("init", false, "Create an initial configuration file at the location specified with [config].")
	configArg := argparse.String("config", "", "Speicfy the path to the config fire.")
	argparse.Parse(os.Args[1:])

	// attempt to resolve config file path.
	// if the provided path is relative, resolve it against os.Executable.
	configPath := *configArg
	root, err := os.Executable()
	if err != nil {
		fmt.Printf("Failed to resolve absolute path for config file: %s\n", err.Error())
		os.Exit(1)
	}
	if !path.IsAbs(configPath) {
		configPath = path.Join(path.Dir(root), configPath)
	}

	// check if init. if init, we start web installer or generate
	// config. if we *don't* use the web installer, we don't perform
	// installation of databases because we don't know what kind of
	// config the user want; this is different from web installer
	// because we ask the user to provide required info during the
	// process.
	if *initFlag {
		if askYesNo("Start web installer?") {
			WebInstaller()
			os.Exit(0)
		}
		err := gitus.CreateConfigFile(configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create configuration file: %s\n", err.Error())
			os.Exit(1)
		}
		fmt.Printf("Configuration file created. (Please further edit it to fit your exact requirements.)\n")
		os.Exit(0)
	}

	mainCall := argparse.Args()
	// NOTE THAT certain activities does not need parts of Gitus
	// (e.g. "ssh" and "webhooks" does not require a working mailer
	// or session store). We've decided they should not report
	// error when we're not able to initialize them in this case.
	containsCommand := len(mainCall) > 0
	isWebServer := !containsCommand
	isSsh := containsCommand && mainCall[0] == "ssh"
	isWebHooks := containsCommand && mainCall[0] == "web-hooks"
	isUpdateTrigger := containsCommand && mainCall[0] == "update-trigger"
	isResetAdmin := containsCommand && mainCall[0] == "reset-admin"
	dbifNeeded := isWebServer || (containsCommand && (isSsh || isWebHooks || isUpdateTrigger || isResetAdmin))
	ssifNeeded := isWebServer
	keyctxNeeded := isWebServer || (containsCommand && isSsh)
	rsifNeeded := isWebServer
	mailerNeeded := isWebServer
	ccmNeeded := isWebServer

	config, err := gitus.LoadConfigFile(configPath)
	noConfig := err != nil
	// we use the same executable for the web server and the ssh
	// handling command. both use cases requires a proper config
	// file. as of v0.2, we've decided to reuse the `-config` command
	// line argument in the case of ssh (and similarily other possible
	// situations), so if we really don't have a config here we cannot
	// do anything.
	if noConfig {
		if isSsh {
			fmt.Print(gitlib.ToPktLine(fmt.Sprintf("ERR failed to load configuration file: %s\n", err.Error())))
		} else {
			fmt.Fprintf(os.Stderr, "Failed to load configuration file: %s\n", err.Error())
		}
		os.Exit(1)
	}
	
	masterTemplate := templates.LoadTemplate()
	context := routes.RouterContext{
		Config: config,
		MasterTemplate: masterTemplate,
	}

	// if it's in normal mode we need to setup database.
	if config.OperationMode == gitus.OP_MODE_NORMAL {
		if dbifNeeded {
			dbif, err := dbinit.InitializeDatabase(config)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to load database: %s\n", err.Error())
				os.Exit(1)
			}
			context.DatabaseInterface = dbif
		}

		if ssifNeeded {
			ssif, err := ssinit.InitializeDatabase(config)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to initialize session store: %s\n", err.Error())
				os.Exit(1)
			}
			context.SessionInterface = ssif
		}

		if keyctxNeeded {
			keyctx, err := ssh.ToContext(config)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create key managing context: %s\n", err.Error())
				fmt.Fprintf(os.Stderr, "You should try to fix the problem and run Gitus again, or else you might not be able to clone/push through SSH.\n")
				os.Exit(1)
			}
			context.SSHKeyManagingContext = keyctx
		}

		if rsifNeeded {
			rs, err := rsinit.InitializeReceiptSystem(config)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create receipt system interface: %s\n", err.Error())
				fmt.Fprintf(os.Stderr, "You should try to fix the problem and run Gitus again, or things like user registration & password resetting wouldn't work properly.\n")
				os.Exit(1)
			}
			context.ReceiptSystem = rs
		}

		if mailerNeeded {
			ml, err := mail.InitializeMailer(config)
			// TODO: somehow if err != nil `ml` still seems to be non-nil. fix this.
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create mailer interface: %s\n", err.Error())
				fmt.Fprintf(os.Stderr, "You should try to fix the problem and run Gitus again, or things thar depends on sending emails wouldn't work properly.\n")
				ml = nil
			}
			context.Mailer = ml
		}

		if ccmNeeded {
			ccm, err := confirm_code.InitializeConfirmCodeManager(config)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to create confirm code manager: %s\n", err.Error())
				fmt.Fprintf(os.Stderr, "You should try to fix the problem and run Gitus again, or things thar depends on sending emails wouldn't work properly.\n")
			}
			context.ConfirmCodeManager = ccm
		}

		ok, err := normalModeGitusReadyCheck(context)
		if !ok {
			fmt.Fprintf(os.Stderr, "Gitus Ready Check failed: %s\n", err.Error())
			// NOTE(2026.2.14): deprecation of cli installer
			// InstallGitus(context)
			WebInstaller()
			os.Exit(1)
		}
	}

	gitUser, err := user.Lookup(context.Config.GitUser)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to find Git user %s: %s\n", context.Config.GitUser, err.Error())
		fmt.Fprintf(os.Stderr, "You should try to fix the problem and run Gitus again, or else you might not be able to clone/push through SSH.\n")
		os.Exit(1)
	}
	context.GitUserHomeDirectory = gitUser.HomeDir

	// the features of these commands are meaningless in the use case of
	// plain mode, so the dispatching is done within this if branch.
	if len(mainCall) > 0 {
		switch mainCall[0] {
		case "install":
			// NOTE(2026.2.14): deprecation of cli installer
			// InstallGitus(context)
			WebInstaller()
			return
		case "reset-admin":
			if noConfig {
				fmt.Fprintf(os.Stderr, "No config file specified. Cannot continue.\n")
			} else {
				ResetAdmin(&context)
			}
			return
		case "ssh":
			if len(mainCall) < 3 {
				fmt.Print(gitlib.ToPktLine("Error format for `gitus ssh`."))
				return
			}
			HandleSSHLogin(&context, mainCall[1], mainCall[2])
			return
		case "no-login":
			fmt.Println(context.Config.NoInteractiveShellMessage)
			return
		case "simple-mode":
			if len(mainCall) < 3 {
				fmt.Print(gitlib.ToPktLine("Error format for `gitus simple-mode`."))
				return
			}
			HandleSimpleMode(&context, mainCall[1], mainCall[2])
			return
		case "web-hooks":
			if len(mainCall) < 7 {
				fmt.Print(gitlib.ToPktLine("Error format for `gitus web-hooks`."))
				return
			}
			switch mainCall[1] {
			case "send":
				HandleWebHook(&context, mainCall[2], mainCall[3], mainCall[4], mainCall[5], mainCall[6])
			default:
				fmt.Print(gitlib.ToPktLine(fmt.Sprintf("Error command for `gitus web-hooks`: %s.", mainCall[1])))
			}
			return
			// TODO(2026.2.23): un-comment or remove this after designing the CI system
			// case "update-trigger":
			// 	if len(mainCall) < 6 {
			// 		fmt.Print(gitlib.ToPktLine("Error format for `gitus-update-trigger`."))
			// 		return
			// 	}
			// 	HandleUpdateTrigger(&context, mainCall[1], mainCall[2], mainCall[3], mainCall[4], mainCall[5])
			// 	return
		}
	}

	staticPrefix := config.StaticAssetDirectory
	templates.UnpackStaticFileTo(staticPrefix)
	var fs = http.FileServer(http.Dir(staticPrefix))
	http.Handle("GET /favicon.ico", routes.WithLogHandler(fs))
	http.Handle("GET /static/", http.StripPrefix("/static/", routes.WithLogHandler(fs)))
	server := &http.Server{
		Addr: fmt.Sprintf("%s:%d", config.BindAddress, config.BindPort),
	}

	context.RateLimiter = routes.NewRateLimiter(config)
	
	controller.InitializeRoute(&context)

	go func() {
		log.Printf("Start serving at %s:%d\n", config.BindAddress, config.BindPort)
		err := server.ListenAndServe()
		if err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
		log.Println("Stopped serving new connections.")
	}()

	// apparently go kills absolutely everything when main returns -
	// all the goroutines and things would be just gone and not even
	// deferred calls are executed, which is insane if you think about
	// it. the `http.Server.Shutdown` method closes the http server
	// gracefully but `http.ListenAndServe` just serves and does not
	// return the server obj, a separate Server obj is needed to
	// close. putting the teardown part after `http.ListenAndServe`
	// doesn't seem to cut it because of SIGINT and the like. we wait
	// on a channel (which we set up beforehand to put up a notifying
	// message when SIGINT/others occur) so that in cases like those
	// we would still have a chance to wrap things up.
	// this is also used for the webinstaller since it's also a http
	// server as well.
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	shutdownCtx, shutdownRelease := gocontext.WithTimeout(gocontext.Background(), 10*time.Second)
	defer shutdownRelease()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("HTTP shutdown err: %v", err.Error())
	}

	if context.DatabaseInterface != nil {
		if err = context.DatabaseInterface.Dispose(); err != nil {
			log.Printf("Failed to dispose database interface: %s\n", err.Error())
		}
	}
	if context.SessionInterface != nil {
		if err = context.SessionInterface.Dispose(); err != nil {
			log.Printf("Failed to dispose session store: %s\n", err.Error())
		}
	}
	if context.ReceiptSystem != nil {
		if err = context.ReceiptSystem.Dispose(); err != nil {
			log.Printf("Failed to dispose receipt system: %s\n", err.Error())
		}
	}

	if context.Config.OperationMode == gitus.OP_MODE_SIMPLE {
		os.RemoveAll(path.Join(gitUser.HomeDir, "gitus.sock"))
	}
	
	log.Println("Graceful shutdown complete.")
}

