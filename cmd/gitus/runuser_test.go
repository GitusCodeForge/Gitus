package main

import (
	"os"
	"testing"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/log"
)

func TestPolicyForCommand(t *testing.T) {
	for _, command := range []string{"web-server", "ssh", "web-hooks", "update-trigger", "storage-check", "authorized-keys", "host-mode", "install-hooks"} {
		p := policyForCommand(command)
		if !p.refuseRoot || p.warnRoot {
			t.Fatalf("%s: policy = %+v; want refuseRoot", command, p)
		}
	}
	for _, command := range []string{"install", "no-login"} {
		p := policyForCommand(command)
		if p.refuseRoot || p.warnRoot {
			t.Fatalf("%s: policy = %+v; want allowed", command, p)
		}
	}
	p := policyForCommand("reset-admin")
	if p.refuseRoot || !p.warnRoot {
		t.Fatalf("reset-admin: policy = %+v; want warnRoot only", p)
	}
}

func TestAllowRootFromEnv(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes", "on"} {
		t.Setenv(allowRootEnv, v)
		if !allowRootFromEnv() {
			t.Fatalf("%s=%q was not recognized", allowRootEnv, v)
		}
	}
	for _, v := range []string{"", "0", "false", "no", "please"} {
		t.Setenv(allowRootEnv, v)
		if allowRootFromEnv() {
			t.Fatalf("%s=%q was recognized as true", allowRootEnv, v)
		}
	}
}

func TestCheckRunUserAsRoot(t *testing.T) {
	old := getEuid
	getEuid = func() int { return 0 }
	t.Cleanup(func() { getEuid = old })

	log.SetOutput(nil)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	cfg := &gitus.GitusConfig{GitUser: "git"}
	t.Setenv(allowRootEnv, "")

	for _, command := range []string{"web-server", "ssh", "web-hooks", "update-trigger", "storage-check", "authorized-keys", "host-mode", "install-hooks"} {
		if err := checkRunUser(cfg, command, false); err == nil {
			t.Fatalf("%s was allowed as root without -allow-root", command)
		}
	}
	for _, command := range []string{"install", "no-login", "reset-admin"} {
		if err := checkRunUser(cfg, command, false); err != nil {
			t.Fatalf("%s was refused as root: %s", command, err)
		}
	}

	if err := checkRunUser(cfg, "ssh", true); err != nil {
		t.Fatalf("-allow-root did not allow ssh: %s", err)
	}
	t.Setenv(allowRootEnv, "1")
	if err := checkRunUser(cfg, "ssh", false); err != nil {
		t.Fatalf("%s=1 did not allow ssh: %s", allowRootEnv, err)
	}
}
