<center>
	<img src="./logo.png" alt="Aegis Logo" />
</center>

# Gitus

Gitus is a self-hosted code forge that mainly supports the Git distributed version control system.

+ Simple, functional UI;
+ **No JavaScript** - works all major browsers and a lot of minor ones, including [Ladybird](https://ladybird.org), [K-Meleon](http://kmeleonbrowser.org) and [NetSurf](https://www.netsurf-browser.org).
+ Proper web installer for easy installation;
+ Namespaces (which you can turn off at installation time);
+ Read/write access through SSH with per-namespace and/or per-repository user access control;
+ Issue tracking and pull requests;
+ Webhooks for external CI/CD (experimental feature; subject to change)
+ Support for snippets (à la GitHub snippets)
+ Multiple operation mode for different needs:
  + Browse-Only Mode for when you only need to have a web interface, similar to `git-instaweb`.
  + Host Mode for when you only need minimum features like creating repositories, push/pull by SSH and basic access control.
  + Forge Mode, which comes with all the usual features.
+ And many tiny things:
  + Labels for repositories;
  + UI for Git Blame;
  + UI for directly editing files;
  + 2-Factor Authentication
  + `Co-Authored-By`

## Build

Requires Go v1.24+

``` sh
make all
```

## Installation

(for extra details please visit [docs/installation.org](docs/installation.org))

Run the following command:

``` sh
./gitus -init
```

And choose to start the web installer. It will guide you through the configuration process.

After the configuration process, run:

``` sh
./gitus -config {config_file_path}
```

This would set up everything that's required. After the setup process is completed, run the same command again to start the "main" web server.

This process would show you the password for the admin user, which is randomly generated. You can override this password by running:

``` sh
./gitus -config {config_file_path} reset-admin
```

Currently Aegis only supports the following systems for its components; support for other systems are planned:

+ Main database:
  + PostgreSQL
  + SQLite3
+ Receipt system (see [docs/receipt.org](docs/receipt.org)):
  + PostgreSQL
  + SQLite3
+ Session store:
  + In-memory
  + SQLite
  + Redis & Redis-like (ValKey, KeyDB)
  + Memcached
+ Mailer:
  + SMTP
  + GMail (through App Password)


