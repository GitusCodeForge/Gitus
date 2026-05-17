package auxfuncs

import (
	"cmp"
	"math/rand"
	"os"
	"os/user"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

func SortedKeys[K cmp.Ordered, V any](m map[K]V) ([]K) {
	keys := make([]K, 0)
	for k, _ := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

const passchdict = "abcdefghijklmnopqrstuvwxyz0123456789-_"
func GenSym(n int) string {
	res := make([]byte, 0)
	for range n {
		res = append(res, passchdict[rand.Intn(len(passchdict))])
	}
	return string(res)
}
/** generating symbols using cryptographically safe RNG... */
func CryptoGenSym(n int) string {
	// TODO: implement this properly
	res := make([]byte, 0)
	for range n {
		res = append(res, passchdict[rand.Intn(len(passchdict))])
	}
	return string(res)
}

func ChangeLocationOwner(targetPath string, targetUser *user.User) error {
	uidInt, _ := strconv.Atoi(targetUser.Uid)
	gidInt, _ := strconv.Atoi(targetUser.Gid)
	err := os.Chown(targetPath, uidInt, gidInt)
	return err
}

func ChangeLocationOwnerByName(targetPath string, targetUserName string) error {
	u, err := user.Lookup(targetUserName)
	if err != nil { return err }
	uidInt, _ := strconv.Atoi(u.Uid)
	gidInt, _ := strconv.Atoi(u.Gid)
	err = os.Chown(targetPath, uidInt, gidInt)
	return err
}

var reCSVParseEscapeSequence = regexp.MustCompile(`\\(.)`)
func ParseCSV(s string) []string {
	start := 0
	slen := len(s)
	end := slen - 1
	for start < slen && unicode.IsSpace(rune(s[start])) {
		start += 1
	}
	for end >= 0 && unicode.IsSpace(rune(s[end])) {
		end -= 1
	}
	if end >= 0 && s[end] == '\\' {
		end += 1
	}
	r := s[start : end+1]
	res := make([]string, 0)
	i := 0
	start = 0
	for i <= len(r) {
		if i == len(r) {
			res = append(res, reCSVParseEscapeSequence.ReplaceAllString(r[start:i], `$1`))
			break
		}
		switch r[i] {
		case '\\':
			i += 2
		case ',':
			res = append(res, reCSVParseEscapeSequence.ReplaceAllString(r[start:i], `$1`))
			i += 1
			start = i
		default:
			i += 1
		}
	}
	return res
}

func EncodeCSV(a []string) string {
	// NOTE: this does not try to change the array passed in as argument.
	res := make([]string, len(a))
	for k, v := range a {
		res[k] = strings.ReplaceAll(v, ",", "\\,")
	}
	pre := strings.Join(res, ",")
	if unicode.IsSpace(rune(pre[0])) { pre = "\\" + pre }
	if unicode.IsSpace(rune(pre[len(pre)-1])) {
		pre = pre[:len(pre)-1] + "\\" + pre[len(pre)-1:]
	}
	return pre
}

