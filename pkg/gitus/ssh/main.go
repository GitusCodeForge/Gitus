package ssh

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"os/user"
	"path"
	"regexp"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/shellparse"
	gossh "golang.org/x/crypto/ssh"
)

type SSHKeyManagingContext struct {
	configFilePath string
	keyFilePath string
	Managed map[string]map[string]string
	notManaged []string
	// config is kept live: an administrator can switch to
	// AuthorizedKeysCommand through the web panel without restarting.
	config *gitus.GitusConfig
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
		config: cfg,
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
		config: cfg,
	}, nil
}

func (ctx *SSHKeyManagingContext) Sync() error {
	if ctx.config != nil && ctx.config.SSHAuth.UsesAuthorizedKeysCommand() { return nil }
	// Pick up keys that somebody added to the file by hand since this context
	// was created; they are not managed by Gitus and must survive the rewrite.
	if err := ctx.ReloadNotManaged(); err != nil { return err }
	f, err := os.OpenFile(ctx.keyFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil { return err }
	defer f.Close()
	for userName, pack := range ctx.Managed {
		for keyName, key := range pack {
			_, err := fmt.Fprintln(f, AuthorizedKeyLine(ctx.configFilePath, userName, keyName, key))
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

// ReloadNotManaged re-reads the authorized_keys file and replaces the list of
// lines that are not managed by Gitus. Sync calls it before rewriting the file
// so manual additions made after startup are not lost.
func (ctx *SSHKeyManagingContext) ReloadNotManaged() error {
	if ctx.config == nil { return nil }
	fresh, err := NewContext(ctx.config)
	if err != nil { return err }
	ctx.notManaged = fresh.notManaged
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

// AuthorizedKeyLine is one authorized_keys entry that runs the gitus ssh
// handler with the Gitus user and key name baked in.
func AuthorizedKeyLine(configPath string, userName string, keyName string, keyText string) string {
	return fmt.Sprintf("command=\"gitus -config \\\"%s\\\" ssh %s %s\" %s",
		shellparse.Quote(configPath), userName, shellparse.Quote(keyName), strings.TrimSpace(keyText))
}

// MatchAuthorizedKey reports whether offered is the public key in keyText.
// offered can also be an OpenSSH SHA256/MD5 fingerprint (the %f token of
// AuthorizedKeysCommand), in which case the fingerprints are compared.
func MatchAuthorizedKey(keyText string, offered string) bool {
	stored, err := parseAuthorizedKey(keyText)
	if err != nil { return false }
	offered = strings.TrimSpace(offered)
	switch {
	case strings.HasPrefix(offered, "SHA256:"):
		return gossh.FingerprintSHA256(stored) == offered
	case strings.HasPrefix(offered, "MD5:"):
		return "MD5:"+gossh.FingerprintLegacyMD5(stored) == offered
	case strings.Count(offered, ":") == 15:
		// OpenSSH's older %f form omitted the MD5: prefix.
		return gossh.FingerprintLegacyMD5(stored) == offered
	}
	key, err := parseAuthorizedKey(offered)
	if err != nil { return false }
	return bytes.Equal(stored.Marshal(), key.Marshal())
}

func parseAuthorizedKey(s string) (gossh.PublicKey, error) {
	pub, _, _, _, err := gossh.ParseAuthorizedKey([]byte(strings.TrimSpace(s)))
	return pub, err
}
