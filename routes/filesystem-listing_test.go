package routes

import (
	"os"
	"os/exec"
	"path"
	"testing"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
)

func makeBareRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil { t.Fatal(err) }
	cmd := exec.Command("git", "init", "--bare", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %s: %s", err.Error(), string(out))
	}
}

func filesystemListingFixture(t *testing.T) *RouterContext {
	t.Helper()
	root := t.TempDir()
	makeBareRepo(t, path.Join(root, "alpha", "one"))
	makeBareRepo(t, path.Join(root, "alpha", "two"))
	makeBareRepo(t, path.Join(root, "beta", "three"))
	cfg := &gitus.GitusConfig{GitRoot: root}
	cfg.UseNamespace = true
	ctx := &RouterContext{
		Config: cfg,
		FilesystemListingCache: NewFilesystemListingCache(),
	}
	return ctx
}

func TestFilesystemListingCache(t *testing.T) {
	ctx := filesystemListingFixture(t)

	if err := ctx.SyncAllNamespaceFromFilesystem(); err != nil { t.Fatal(err) }
	if len(ctx.GitNamespaceList) != 2 {
		t.Fatalf("expected 2 namespaces, got %d", len(ctx.GitNamespaceList))
	}
	alpha := ctx.GitNamespaceList["alpha"]
	if alpha == nil { t.Fatal("alpha is missing") }

	if err := ctx.SyncNamespaceFromFilesystem(alpha); err != nil { t.Fatal(err) }
	if len(alpha.RepositoryList) != 2 {
		t.Fatalf("expected 2 repositories in alpha, got %d", len(alpha.RepositoryList))
	}

	// A change on disk is not visible until the entry expires.
	makeBareRepo(t, path.Join(ctx.Config.GitRoot, "alpha", "four"))
	alpha2 := ctx.GitNamespaceList["alpha"]
	if err := ctx.SyncNamespaceFromFilesystem(alpha2); err != nil { t.Fatal(err) }
	if len(alpha2.RepositoryList) != 2 {
		t.Fatalf("the entry should still be cached, got %v", alpha2.RepositoryList)
	}

	// ... and is picked up once it has expired.
	ctx.FilesystemListingCache.ttl = 0
	alpha3 := ctx.GitNamespaceList["alpha"]
	if err := ctx.SyncNamespaceFromFilesystem(alpha3); err != nil { t.Fatal(err) }
	if len(alpha3.RepositoryList) != 3 {
		t.Fatalf("the repository list was not refreshed: %v", alpha3.RepositoryList)
	}
	if err := ctx.SyncAllNamespaceFromFilesystem(); err != nil { t.Fatal(err) }
	if len(ctx.GitNamespaceList) != 2 {
		t.Fatalf("the namespace list was not refreshed")
	}
}

func TestFilesystemListingCacheHandsOutCopies(t *testing.T) {
	ctx := filesystemListingFixture(t)
	ctx.FilesystemListingCache.ttl = time.Hour

	if err := ctx.SyncAllNamespaceFromFilesystem(); err != nil { t.Fatal(err) }
	ns := ctx.GitNamespaceList["alpha"]
	if err := ctx.SyncNamespaceFromFilesystem(ns); err != nil { t.Fatal(err) }
	ns.Title = "definitely not the real title"
	ns.RepositoryList["injected"] = ns.RepositoryList["one"]

	// A second read must not see the changes made to the first copy.
	if err := ctx.SyncAllNamespaceFromFilesystem(); err != nil { t.Fatal(err) }
	ns2 := ctx.GitNamespaceList["alpha"]
	if ns2.Title == "definitely not the real title" {
		t.Fatal("the cache handed out its own namespace object")
	}
	if err := ctx.SyncNamespaceFromFilesystem(ns2); err != nil { t.Fatal(err) }
	if _, ok := ns2.RepositoryList["injected"]; ok {
		t.Fatal("the cache handed out its own repository map")
	}
}

func TestFilesystemListingCacheIsShared(t *testing.T) {
	ctx := filesystemListingFixture(t)
	if err := ctx.SyncAllNamespaceFromFilesystem(); err != nil { t.Fatal(err) }
	local := ctx.NewLocal()
	if local.FilesystemListingCache != ctx.FilesystemListingCache {
		t.Fatal("NewLocal must share the listing cache")
	}
	// A context without a cache (e.g. built by a test) still works, it just
	// does not cache across requests.
	bare := &RouterContext{Config: ctx.Config}
	if err := bare.SyncAllNamespaceFromFilesystem(); err != nil { t.Fatal(err) }
	if len(bare.GitNamespaceList) != 2 {
		t.Fatalf("expected 2 namespaces without a shared cache, got %d", len(bare.GitNamespaceList))
	}
}
