package gitlib

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseSubmoduleDiff(t *testing.T) {
	src := `19fa1a23ffa5d015edb1bcd5234397fcacf05e9b
diff --git a/sub/dotfiles b/sub/dotfiles
new file mode 160000
index 0000000..8e50ff3
--- /dev/null
+++ b/sub/dotfiles
@@ -0,0 +1 @@
+Subproject commit 8e50ff3160a9bfda961e5df146b9e7fb65a3c6cc
`
	d, err := parseGitDiff(bytes.NewBufferString(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.ItemList) != 1 {
		t.Fatalf("got %d items", len(d.ItemList))
	}
	item := d.ItemList[0]
	if item.File1 != "/dev/null" || item.File2 != "b/sub/dotfiles" {
		t.Fatalf("file headers = %q -> %q", item.File1, item.File2)
	}
	if len(item.PatchList) != 1 {
		t.Fatalf("got %d patches", len(item.PatchList))
	}
	patch := item.PatchList[0]
	if patch.LLineCount != 0 || patch.RLineCount != 1 {
		t.Fatalf("line counts = %d/%d", patch.LLineCount, patch.RLineCount)
	}
	if len(patch.LineList) != 1 || patch.LineList[0].Type != APPEND {
		t.Fatalf("patch lines = %+v", patch.LineList)
	}
	if strings.TrimSpace(patch.LineList[0].Line) != "Subproject commit 8e50ff3160a9bfda961e5df146b9e7fb65a3c6cc" {
		t.Fatalf("line = %q", patch.LineList[0].Line)
	}
}

func TestParseDiffWithoutNewlineMarker(t *testing.T) {
	src := `abc123
diff --git a/a b/a
index 1111111..2222222 100644
--- a/a
+++ b/a
@@ -1 +1 @@
-old
+new
\ No newline at end of file
`
	d, err := parseGitDiff(bytes.NewBufferString(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.ItemList) != 1 || len(d.ItemList[0].PatchList) != 1 {
		t.Fatalf("unexpected diff: %+v", d)
	}
	if got := len(d.ItemList[0].PatchList[0].LineList); got != 2 {
		t.Fatalf("got %d content lines", got)
	}
}
