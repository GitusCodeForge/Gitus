package ssh

import (
	"fmt"
	"maps"
	"os"
	"os/user"
	"path"
	"regexp"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/shellparse"
)

type SSHKeyManagingContext struct {
	configFilePath string
	keyFilePath string
	Managed map[string]map[string]string
	notManaged []string
}

var reParseKey = regexp.MustCompile("\\s*command=\"gitus -config .* ssh ([^ ]*) ([^ ]*)\"\\s*(.*)")	
func parseKey(s string) (bool, string, string, string, error) {
	// we expect all keys managed by gitus should have the prefix
	//     command="gitus -config {config-path} ssh {username} {keyname}"
	r := reParseKey
	k := r.FindSubmatch([]byte(s))
	if len(k) <= 0 { return false, "", "", "", nil }
	return true, string(k[1]), string(k[2]), string(k[3]), nil
}

// create a new context but does not read the managed keys from the authorized_keys file.
func NewContext(cfg *gitus.GitusConfig) (*SSHKeyManagingContext, error) {
	u, err := user.Lookup(cfg.GitUser)
	if err != nil { return nil, err }
	p := path.Join(u.HomeDir, ".ssh", "authorized_keys")
	f, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) { return nil, err }
	notManaged := make([]string, 0)
	for k := range strings.SplitSeq(string(f), "\n") {
		kstr := strings.TrimSpace(k)
		if len(kstr) <= 0 { continue }
		if strings.HasPrefix(kstr, "#") { continue }
		chk, _, _, _, err := parseKey(k)
		if err != nil { return nil, err }
		if !chk { notManaged = append(notManaged, k); continue }
	}
	return &SSHKeyManagingContext{
		configFilePath: cfg.FilePath,
		keyFilePath: p,
		Managed: nil,
		notManaged: notManaged,
	}, nil
	
}

func ToContext(cfg *gitus.GitusConfig) (*SSHKeyManagingContext, error) {
	u, err := user.Lookup(cfg.GitUser)
	if err != nil { return nil, err }
	p := path.Join(u.HomeDir, ".ssh", "authorized_keys")
	f, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) { return nil, err }
	managed := make(map[string]map[string]string, 0)
	currentName := ""
	currentManaged := make(map[string]string, 0)
	notManaged := make([]string, 0)
	for k := range strings.SplitSeq(string(f), "\n") {
		kstr := strings.TrimSpace(k)
		if len(kstr) <= 0 { continue }
		if strings.HasPrefix(kstr, "#") { continue }
		chk, userName, keyName, key, err := parseKey(k)
		if err != nil { return nil, err }
		if !chk { notManaged = append(notManaged, k); continue }
		if currentName == "" { currentName = userName }
		if userName != currentName {
			managed[currentName] = currentManaged
			d, ok := managed[userName]
			if ok {
				currentManaged = d
			} else {
				currentManaged = make(map[string]string, 0)
			}
			currentManaged[keyName] = key
		} else {
			currentManaged[keyName] = key
		}
	}
	_, ok := managed[currentName]
	if !ok {
		managed[currentName] = currentManaged
	} else {
		maps.Copy(managed[currentName], currentManaged)
	}
	return &SSHKeyManagingContext{
		configFilePath: cfg.FilePath,
		keyFilePath: p,
		Managed: managed,
		notManaged: notManaged,
	}, nil
}

func (ctx *SSHKeyManagingContext) Sync() error {
	f, err := os.OpenFile(ctx.keyFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil { return err }
	defer f.Close()
	quotedConfig := shellparse.Quote(ctx.configFilePath)
	for userName, pack := range ctx.Managed {
		for keyName, key := range pack {
			_, err := fmt.Fprintf(f, "command=\"gitus -config \\\"%s\\\" ssh %s %s\" %s", quotedConfig, userName, shellparse.Quote(keyName), key)
			if err != nil { return err }
			_, err = f.WriteString("\n")
			if err != nil { return err }
		}
		f.WriteString("\n")
	}
	_, err = f.WriteString("\n")
	if err != nil { return err }
	for _, item := range ctx.notManaged {
		_, err := f.WriteString(item)
		if err != nil { return err }
		_, err = f.WriteString("\n")
		if err != nil { return err }
	}
	return nil
}

func (ctx *SSHKeyManagingContext) AddAuthorizedKey(username string, keyname string, key string) {
	if ctx.Managed == nil {
		ctx.Managed = make(map[string]map[string]string)
	}
	pack, ok := ctx.Managed[username]
	if !ok {
		pack = make(map[string]string, 0)
		ctx.Managed[username] = pack
	}
	pack[keyname] = key
}

func (ctx *SSHKeyManagingContext) RemoveAuthorizedKey(username string, keyname string) {
	if ctx.Managed == nil { return }
	pack, ok := ctx.Managed[username]
	if !ok { return }
	delete(pack, keyname)
}

func (ctx *SSHKeyManagingContext) GetAuthorizedKey(username string, keyname string) string {
	if ctx.Managed == nil { return "" }
	pack, ok := ctx.Managed[username]
	if !ok { return "" }
	s, ok := pack[keyname]
	if !ok { return "" }
	return s
}

