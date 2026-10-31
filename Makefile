# `go run` compiles the devtools helper and then executes it from a temporary
# directory. On systems where /tmp is mounted noexec (and on any system where
# the user's TMPDIR points at such a mount) that fails with
#
#   fork/exec /tmp/go-build.../exe/embed-static: permission denied
#
# so keep Go's temporary build files inside the checkout instead.
GOTMPDIR ?= $(CURDIR)/.gotmp
export GOTMPDIR

clean:
	if [ -f "gitus" ]; then rm ./gitus; fi

gitus-web-server:
	mkdir -p "$(GOTMPDIR)"
	go run ./devtools/generate-template.go templates
	go run ./devtools/generate-footer-template.go
	go run ./devtools/embed-static.go ./static templates
	go build ./cmd/gitus

all:
	make gitus-web-server
