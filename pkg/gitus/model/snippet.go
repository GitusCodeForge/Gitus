package model

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	SNIPPET_PUBLIC uint8 = 1
	SNIPPET_INTERNAL uint8 = 2
	SNIPPET_SHARED_LINK uint8 = 3
	SNIPPET_SHARED_USER uint8 = 4
	SNIPPET_PRIVATE uint8 = 5
	SNIPPET_SHARED_LINK_INTERNAL uint8 = 6
)

type Snippet struct {
	Name string
	BelongingUser string
	Description string
	Time int64
	FileList map[string]string
	Status uint8
	SharedUser map[string]bool
}

func (s *Snippet) FullName() string {
	return fmt.Sprintf("%s:%s", s.BelongingUser, s.Name)
}

var ErrInvalidSnippetRelPath = errors.New("Invalid relative path for snippet")

// retrieve file from disk.
func (s *Snippet) Retrieve(basePath string, filePath string) error {
	fullPath := path.Clean(path.Join(basePath, s.BelongingUser, s.Name, filePath))
	relCheck, err := filepath.Rel(basePath, fullPath)
	if err != nil { return ErrInvalidSnippetRelPath }
	if strings.HasPrefix(relCheck, "..") { return ErrInvalidSnippetRelPath }
	f, err := os.ReadFile(fullPath)
	if err != nil { return err }
	if s.FileList == nil {
		s.FileList = make(map[string]string, 0)
	}
	s.FileList[filePath] = string(f)
	return nil
}

// retrieve all file from disk.
func (s *Snippet) RetrieveAllFile(basePath string) error {
	rootPath := path.Join(basePath, s.BelongingUser, s.Name)
	p, err := os.ReadDir(rootPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			os.MkdirAll(rootPath, os.ModeDir | 0644)
		} else { return err }
	}
	if s.FileList == nil { s.FileList = make(map[string]string, 0) }
	for _, k := range p {
		if !k.IsDir() {
			f, err := os.ReadFile(path.Join(rootPath, k.Name()))
			if err != nil { return err }
			s.FileList[k.Name()] = string(f)
		}
	}
	return nil
}

// retrieve file list. NOTE THAT this will populate .FileList but
// will NOT read the content of the files. whoever needs the file
// content should always use .Retrieve and .RetrieveAllFile.
func (s *Snippet) CalculateFileList(basePath string) error {
	rootPath := path.Join(basePath, s.BelongingUser, s.Name)
	p, err := os.ReadDir(rootPath)
	if err != nil { return err }
	if s.FileList == nil { s.FileList = make(map[string]string, 0) }
	for _, k := range p {
		if !k.IsDir() { s.FileList[k.Name()] = "" }
	}
	return nil
}

// save file to disk.
func (s *Snippet) SyncFile(basePath string, p string) error {
	source, ok := s.FileList[p]
	if !ok { return nil }
	targetPath := path.Clean(path.Join(basePath, s.BelongingUser, s.Name, p))
	p, err := filepath.Rel(basePath, targetPath)
	if err != nil { return ErrInvalidSnippetRelPath }
	if strings.HasPrefix(p, "..") { return ErrInvalidSnippetRelPath }
	f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil { return err }
	f.WriteString(source)
	f.Sync()
	f.Close()
	return nil
}

// save all file to disk.
func (s *Snippet) SyncAllFile(basePath string) error {
	for k := range s.FileList {
		err := s.SyncFile(basePath, k)
		if err != nil { return err }
	}
	return nil
}

func (s *Snippet) SetFile(p string, content string) {
	if s.FileList == nil { s.FileList = make(map[string]string, 0) }
	s.FileList[p] = content
}

func (s *Snippet) DeleteFile(basePath string, p string) error {
	delete(s.FileList, p)
	targetPath := path.Clean(path.Join(basePath, s.BelongingUser, s.Name, p))
	relCheck, err := filepath.Rel(basePath, targetPath)
	if err != nil { return ErrInvalidSnippetRelPath }
	if strings.HasPrefix(relCheck, "..") { return ErrInvalidSnippetRelPath }
	err = os.RemoveAll(targetPath)
	if err != nil { return err }
	return nil
}

