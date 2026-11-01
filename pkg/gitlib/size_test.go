package gitlib

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDirectorySizeInBytes(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "objects", "ab"), 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]int{
		"objects/ab/cdef": 1000,
		"packed-refs":     100,
		"hooks/update":    20,
	}
	for name, size := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, make([]byte, size), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// a symlink is not followed and adds nothing
	if err := os.Symlink(filepath.Join(dir, "packed-refs"), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	got, err := DirectorySizeInBytes(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(1120); got != want {
		t.Errorf("DirectorySizeInBytes = %d; want %d", got, want)
	}
}

func TestLocalGitRepositorySize(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "objects"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "objects", "blob"), make([]byte, 2048), 0644); err != nil {
		t.Fatal(err)
	}
	gr := LocalGitRepository{GitDirectoryPath: dir}
	if size, err := gr.SizeInBytes(); err != nil || size != 2048 {
		t.Fatalf("SizeInBytes = %d, %v", size, err)
	}
}

func TestCheckSizeLimit(t *testing.T) {
	if err := CheckSizeLimit(2048, 0); err != nil {
		t.Errorf("a limit of 0 refused a repository: %v", err)
	}
	if err := CheckSizeLimit(2048, 4096); err != nil {
		t.Errorf("a repository under the limit was refused: %v", err)
	}
	err := CheckSizeLimit(2048, 1024)
	var limitErr *SizeLimitError
	if !errors.As(err, &limitErr) {
		t.Fatalf("CheckSizeLimit(2048, 1024) returned %v; want a SizeLimitError", err)
	}
	if limitErr.Size != 2048 || limitErr.Limit != 1024 {
		t.Errorf("unexpected limit error: %+v", limitErr)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := map[int64]string{
		512:     "512 B",
		1024:    "1.0 KiB",
		1536:    "1.5 KiB",
		1 << 20: "1.0 MiB",
		3 << 30: "3.0 GiB",
	}
	for in, want := range cases {
		if got := FormatBytes(in); got != want {
			t.Errorf("FormatBytes(%d) = %q; want %q", in, got, want)
		}
	}
}
