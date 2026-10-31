package gitlib

import (
	"bytes"
	"errors"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// NOTE: diff is good candidate for caching. they take resources to
// compute and they don't change.


const (
	DIFF_OLD_MODE uint8 = 1
	DIFF_NEW_MODE uint8 = 2
	DIFF_DELETED_FILE_MODE uint8 = 3
	DIFF_NEW_FILE_MODE uint8 = 4
	DIFF_COPY_FROM uint8 = 5
	DIFF_COPY_TO uint8 = 6
	DIFF_RENAME_FROM uint8 = 7
	DIFF_RENAME_TO uint8 = 8
	DIFF_SIMILARITY_INDEX uint8 = 9
	DIFF_DISSIMILARITY_INDEX uint8 = 10
	DIFF_INDEX uint8 = 11
)

const (
	APPEND uint8 = 1
	DELETE uint8 = 2
	CHANGE uint8 = 3
	SAME uint8 = 4
)

var ErrInvalidFormat = errors.New("Invalid format")

type AnnotatedLine struct {
	Type uint8 `json:"type"`
	F1LineNum int64 `json:"f1"`
	F2LineNum int64 `json:"f2"`
	Line string `json:"line"`
}

// the fact that go refuses to do tagged union - not even C-style
// union - is fucking killing me...
type DiffItemHeaderItem struct {
	Type uint8 `json:"type"`
	Args []string `json:"args"`
}

type DiffItemPatch struct {
	LStart int64 `json:"lStart"`
	LLineCount int64 `json:"lLineCount"`
	RStart int64 `json:"rStart"`
	RLineCount int64 `json:"rLineCount"`
	ContextLine string `json:"ctx"`
	LineList []AnnotatedLine `json:"lines"`
}

type DiffItem struct {
	File1 string `json:"file1"`
	File2 string `json:"file2"`
	Header []*DiffItemHeaderItem `json:"header"`
	PatchList []*DiffItemPatch `json:"patchList"`
}

type Diff struct {
	CommitHash string `json:"commit"`
	ItemList []*DiffItem `json:"item"`
}

// shhhh....... (finger across lips)
var reGitDiffHeader = regexp.MustCompile(`(((?:(?:old|new|deleted file|new file) mode)|(?:copy (?:from|to))|(?:rename (?:from|to))|(?:dis)?similarity index) (.*))|(?:index ([^.]+)\.\.([^.]+)(?: (.*))?)`)
func parseGitDiffHeaderItem(s string) *DiffItemHeaderItem {
	matchres := reGitDiffHeader.FindStringSubmatch(s)
	if len(matchres) <= 0 { return nil }
	if len(matchres[1]) > 0 {
		var cmdType uint8
		switch matchres[1] {
		case "old mode": cmdType = DIFF_OLD_MODE
		case "new mode": cmdType = DIFF_NEW_MODE
		case "deleted file mode": cmdType = DIFF_DELETED_FILE_MODE
		case "new file mode": cmdType = DIFF_NEW_FILE_MODE
		case "copy from": cmdType = DIFF_COPY_FROM
		case "copy to": cmdType = DIFF_COPY_TO
		case "rename from": cmdType = DIFF_RENAME_FROM
		case "rename to": cmdType = DIFF_RENAME_TO
		case "similarity index": cmdType = DIFF_SIMILARITY_INDEX
		case "dissimilarity index": cmdType = DIFF_DISSIMILARITY_INDEX
		}
		return &DiffItemHeaderItem{
			Type: cmdType,
			Args: matchres[2:3],
		}
	} else {
		return &DiffItemHeaderItem{
			Type: DIFF_INDEX,
			Args: matchres[3:],
		}
	}
}


var reGitDiffItemFile1Header = regexp.MustCompile("--- (.*)")
var reGitDiffItemFile2Header = regexp.MustCompile(`\+\+\+ (.*)`)
var reGitDiffItemLineHeader = regexp.MustCompile(`@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)`)

func parseGitDiff(br *bytes.Buffer) (*Diff, error) {
	tlr := newTrueLineReader(br)
	l, err := tlr.readLine()
	if err != nil { return nil, err }
	// NOTE: if there is indeed a diff, the first line would be the
	// commit's id. we can safely start to read the diffs of files
	// w/o unreading the line we've just read.
	commitHash := strings.TrimSpace(l)
	itemList := make([]*DiffItem, 0)
	for {
		l, err = tlr.readLine()
		if errors.Is(err, io.EOF) { break }
		if err != nil { return nil, err }
		// NOTE: we skip the "diff --git" line.
		// parse header
		headerItemList := make([]*DiffItemHeaderItem, 0)
		for {
			l, err = tlr.readLine()
			if errors.Is(err, io.EOF) { break }
			if err != nil { return nil, err }
			p := parseGitDiffHeaderItem(l)
			if p == nil {
				tlr.unreadLine(l)
				break
			}
			headerItemList = append(headerItemList, p)
		}

		// parse file headers
		l, err = tlr.readLine()
		if errors.Is(err, io.EOF) { break }
		if err != nil { return nil, err }
		matchres := reGitDiffItemFile1Header.FindStringSubmatch(l)
		if len(matchres) <= 0 { return nil, ErrInvalidFormat }
		file1 := matchres[1]
		l, err = tlr.readLine()
		if errors.Is(err, io.EOF) { break }
		if err != nil { return nil, err }
		matchres = reGitDiffItemFile2Header.FindStringSubmatch(l)
		if len(matchres) <= 0 { return nil, ErrInvalidFormat }
		file2 := matchres[1]

		// parse patch
		patchList := make([]*DiffItemPatch, 0)
		for {
			l, err = tlr.readLine()
			if errors.Is(err, io.EOF) { break }
			if err != nil { return nil, err }
			matchres := reGitDiffItemLineHeader.FindStringSubmatch(l)
			if len(matchres) <= 0 {
				tlr.unreadLine(l)
				break
			}
			lStartStr := matchres[1]
			lLineCountStr := matchres[2]
			rStartStr := matchres[3]
			rLineCountStr := matchres[4]
			lStart, _ := strconv.ParseInt(lStartStr, 10, 64)
			lLineCount := int64(1)
			if lLineCountStr != "" { lLineCount, _ = strconv.ParseInt(lLineCountStr, 10, 64) }
			rStart, _ := strconv.ParseInt(rStartStr, 10, 64)
			rLineCount := int64(1)
			if rLineCountStr != "" { rLineCount, _ = strconv.ParseInt(rLineCountStr, 10, 64) }
			context := matchres[5]
			pLines := make([]AnnotatedLine, 0)
			f1LineNumberCounter := lStart
			f2LineNumberCounter := rStart
			for {
				l, err = tlr.readLine()
				if errors.Is(err, io.EOF) { break }
				if err != nil { return nil, err }
				if l[0] == '\\' {
					// "\ No newline at end of file" is not a content line.
					continue
				}
				if l[0] != ' ' && l[0] != '-' && l[0] != '+' {
					tlr.unreadLine(l)
					break
				}
				lineContent := l[1:]
				var lineType uint8
				switch l[0] {
				case ' ':
					lineType = SAME
					f1LineNumberCounter += 1
					f2LineNumberCounter += 1
				case '+':
					lineType = APPEND
					f2LineNumberCounter += 1
				case '-':
					lineType = DELETE
					f1LineNumberCounter += 1
				}
				pLines = append(pLines, AnnotatedLine{
					Type: lineType,
					F1LineNum: f1LineNumberCounter,
					F2LineNum: f2LineNumberCounter,
					Line: lineContent,
				})
			}
			patchList = append(patchList, &DiffItemPatch{
				LStart: lStart,
				LLineCount: lLineCount,
				RStart: rStart,
				RLineCount: rLineCount,
				ContextLine: context,
				LineList: pLines,
			})
		}
		itemList = append(itemList, &DiffItem{
			File1: file1,
			File2: file2,
			Header: headerItemList,
			PatchList: patchList,
		})
	}
	return &Diff{
		CommitHash: commitHash,
		ItemList: itemList,
	}, nil
}

// returns the diff by invoking the "git diff" command.
// i should probably implement my own diff and rely as little on the
// git executable as possible at some point.
var ErrDubiousOwnership = errors.New("dubious ownership")
func (gr LocalGitRepository) GetDiff(commitId string) (*Diff, error) {
	cmd := exec.Command("git", "diff-tree", commitId, "-p")
	cmd.Dir = gr.GitDirectoryPath
	stderrBuf := new(bytes.Buffer)
	cmd.Stderr = stderrBuf
	stdoutBuf := new(bytes.Buffer)
	cmd.Stdout = stdoutBuf
	err := cmd.Run()
	if err != nil {
		if strings.Contains(stderrBuf.String(), "dubious ownership") {
			return nil, ErrDubiousOwnership
		}
		return nil, err
	}
	diff, err := parseGitDiff(stdoutBuf)
	return diff, nil
}


