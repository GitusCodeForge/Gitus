package gitlib

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/log"
)

// parser for git-blame output...

type PorcelainBlameCommitInfo struct {
	CommitId string
	AuthorInfo AuthorTime
	CommitterInfo AuthorTime
	Summary string
	Filename string
	OtherHeader map[string]string
}

type PorcelainBlameLine struct {
	CommitId string
	Value string
	FinalLineNumber int
}

type PorcelainBlame struct {
	CommitInfo map[string]*PorcelainBlameCommitInfo
	LineList [][]*PorcelainBlameLine
}

func parsePorcelainBlameHeaderLine(s string) (string, int, int, int) {
	i := 0
	j := i
	for j < len(s) && s[j] != ' ' { j += 1 }
	commitIdStr := strings.TrimSpace(s[i:j])
	j += 1
	i = j
	for j < len(s) && s[j] != ' ' { j += 1 }
	olStr := strings.TrimSpace(s[i:j])
	originalLineNumber, _ := strconv.Atoi(olStr)
	j += 1
	i = j
	for j < len(s) && s[j] != ' ' { j += 1 }
	flstr := strings.TrimSpace(s[i:j])
	groupsizeStr := strings.TrimSpace(s[j:])
	finalLineNumber, _ := strconv.Atoi(flstr)
	groupSize, err := strconv.Atoi(groupsizeStr)
	if err != nil { groupSize = -1 }
	return commitIdStr, originalLineNumber, finalLineNumber, groupSize
}

func parsePorcelainBlameLine(ci map[string]*PorcelainBlameCommitInfo, b *trueLineReader) ([]*PorcelainBlameLine, error) {
	s, err := b.readLine()
	if err != nil { return nil, err }
	commitId, _, finalLine, groupSize := parsePorcelainBlameHeaderLine(s)
	l, err := b.readLine()
	if err != nil { return nil, err }
	if l[0] == '\t' {
		line := l[1:]
		r := make([]*PorcelainBlameLine, 0)
		r = append(r, &PorcelainBlameLine{
			CommitId: commitId,
			Value: line,
			FinalLineNumber: finalLine,
		})
		for range groupSize-1 {
			s, err = b.readLine()
			if err != nil { break }
			commitId, _, finalLine, groupSize = parsePorcelainBlameHeaderLine(s)
			s, err = b.readLine()
			line = s[1:]
			r = append(r, &PorcelainBlameLine{
				CommitId: commitId,
				Value: line,
				FinalLineNumber: finalLine,
			})
		}
		return r, nil
	} else {
		b.unreadLine(l)
	}

	m := make(map[string]string, 0)
	for {
		line, err := b.readLine()
		if err != nil { break }
		if line[0] == '\t' { l = line[1:]; break }
		j := 0
		for j < len(line) && line[j] != ' ' { j += 1 }
		key := line[0:j]
		valueStr := strings.TrimSpace(line[j:])
		m[key] = valueStr
	}
	commitInfo := &PorcelainBlameCommitInfo{}
	commitInfo.CommitId = commitId
	commitInfo.Summary = m["summary"]
	commitInfo.Filename = m["filename"]
	authorName := m["author"]
	authorEmail := m["author-mail"][1:len(m["author-mail"])-1]
	authorTimestamp, _ := strconv.ParseInt(m["author-time"], 10, 64)
	authorTZ, _ := parseTimezoneOffset(m["author-tz"])
	authorT := time.Unix(authorTimestamp, 0).UTC().In(
		time.FixedZone("UTC"+m["author-tz"], authorTZ),
	)
	commitInfo.AuthorInfo = AuthorTime{
		AuthorName: authorName,
		AuthorEmail: authorEmail,
		Time: authorT,
	}
	committerName := m["committer"]
	committerEmail := m["committer-email"]
	committerTimestamp, _ := strconv.ParseInt(m["committer-time"], 10, 64)
	committerTZ, _ := parseTimezoneOffset(m["committer-tz"])
	committerT := time.Unix(committerTimestamp, 0).UTC().In(
		time.FixedZone("UTC"+m["committer-tz"], committerTZ),
	)
	commitInfo.CommitterInfo = AuthorTime{
		AuthorName: committerName,
		AuthorEmail: committerEmail,
		Time: committerT,
	}
	ci[commitId] = commitInfo
	r := make([]*PorcelainBlameLine, 0)
	r = append(r, &PorcelainBlameLine{
		CommitId: commitId,
		Value: l,
		FinalLineNumber: finalLine,
	})
	for range groupSize-1 {
		s, err = b.readLine()
		if err != nil { break }
		commitId, _, finalLine, groupSize = parsePorcelainBlameHeaderLine(s)
		s, err = b.readLine()
		line := s[1:]
		r = append(r, &PorcelainBlameLine{
			CommitId: commitId,
			Value: line,
			FinalLineNumber: finalLine,
		})
	}
	return r, nil
}

func parsePorcelainBlame(b *trueLineReader) (*PorcelainBlame, error) {
	ci := make(map[string]*PorcelainBlameCommitInfo, 0)
	l := make([][]*PorcelainBlameLine, 0)
	for {
		line, err := parsePorcelainBlameLine(ci, b)
		if err != nil { break }
		l = append(l, line)
	}
	return &PorcelainBlame{
		CommitInfo: ci,
		LineList: l,
	}, nil
}

func (gr *LocalGitRepository) Blame(c *CommitObject, p string) (*PorcelainBlame, error) {
	cmd := exec.Command("git", "blame", "--porcelain", c.Id, "--", p)
	cmd.Dir = gr.GitDirectoryPath
	stdoutBuf := new(bytes.Buffer)
	cmd.Stdout = stdoutBuf
	stderrBuf := new(bytes.Buffer)
	cmd.Stderr = stderrBuf
	err := cmd.Run()
	if err != nil {
		log.Warnf("git blame: %s", stderrBuf.String())
		return nil, err
	}
	pb, err := parsePorcelainBlame(newTrueLineReader(stdoutBuf))
	if err != nil { return nil, err }
	return pb, err
}



