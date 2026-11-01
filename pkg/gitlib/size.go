package gitlib

import (
	"fmt"
	"io/fs"
	"path/filepath"
)

// SizeInBytes reports how much disk space this repository occupies: the sum
// of the sizes of the files under the git directory. The quarantine directory
// a receiving process writes the incoming objects to lives under objects/, so
// during the pre-receive hook the result already includes the objects of the
// push that is being decided on.
func (gr LocalGitRepository) SizeInBytes() (int64, error) {
	return DirectorySizeInBytes(gr.GitDirectoryPath)
}

// DirectorySizeInBytes sums the sizes of the regular files under p. Symlinks
// are not followed, so a link inside the repository cannot make the count
// depend on something outside of it.
func DirectorySizeInBytes(p string) (int64, error) {
	var total int64
	err := filepath.WalkDir(p, func(cur string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		return 0, err
	}
	return total, nil
}

// SizeLimitError describes a repository whose size is over the configured
// limit. CheckSizeLimit returns it and the pre-receive hook prints it to the
// person pushing.
type SizeLimitError struct {
	Size  int64
	Limit int64
}

func (e *SizeLimitError) Error() string {
	return fmt.Sprintf("the repository would be %s after this push, over the limit of %s; the push was refused",
		FormatBytes(e.Size), FormatBytes(e.Limit))
}

// FormatBytes renders a size for the message a git client shows.
func FormatBytes(n int64) string {
	units := []struct {
		name string
		size int64
	}{
		{"TiB", 1 << 40},
		{"GiB", 1 << 30},
		{"MiB", 1 << 20},
		{"KiB", 1 << 10},
	}
	for _, u := range units {
		if n >= u.size {
			return fmt.Sprintf("%.1f %s", float64(n)/float64(u.size), u.name)
		}
	}
	return fmt.Sprintf("%d B", n)
}

// CheckSizeLimit returns a *SizeLimitError when size is over limit bytes. A
// limit of 0 disables the check. size is the repository size including the
// objects a push brought to the quarantine directory.
func CheckSizeLimit(size int64, limit int64) error {
	if limit > 0 && size > limit {
		return &SizeLimitError{Size: size, Limit: limit}
	}
	return nil
}
