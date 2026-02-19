package gitus

import (
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
)

type GitusConfig struct {
	lock sync.RWMutex
	FilePath string
	// the version of the configuration file. currently only 0 is
	// allowed.
	Version int `json:"version"`
	// specify the root directory where all the `.git` directories
	// will reside.
	GitRoot string `json:"root"`
	GitUser string `json:"gitUser"`
	// whether to enable namespace or not.  this actually affects how
	// gitus store / search for existing .git repository: when this
	// field is set to true, the first level directories after GitRoot
	// will never be considered as git repository. e.g. this would be
	// the situation if useNamespace is false:
	//
	//   GitRoot/a.git   ---> valid repo (name "a")
	//   GitRoot/a/.git  ---> valid repo (name "a")
	//   GitRoot/a       ---> not a repo if it itself is not
	//                        a valid git folder or does not
	//                        contain a valid .git folder.
	//   GitRoot/a/b.git ---> "a" is not a valid repo name, same as "b" (!)
	//                        the repo at `b.git` can only be recognized
	//                        if GitRoot is set as `{oldGitRoot}/a`.
	//   GitRoot/a/b/.git ---> same as above.
	//
	// but if useNamespace is true, then this would be the case:
	//
	//   GitRoot/a.git   ---> not a recognized repo (!) and not a namespace.
	//                        gitus namespace name cannot contain period.
	//   GitRoot/a/.git  ---> namespace name "a" but not a recognized repo
	//                        since the name for repos in gitus is the
	//                        part before ".git" and this part cannot be
	//                        empty.
	//   GitRoot/a       ---> a namespace (name "a")
	//   GitRoot/a/b.git ---> namespace "a", repo "b".
	//                        (fullName would be "a:b")
	//   GitRoot/a/b/.git ---> same as above.
	//   GitRoot/xy/cde.git ---> namespace "xy", repo "cde".
	//                           (fullName would be "xy:cde").
	UseNamespace bool `json:"enableNamespace"`
	// see docs/operation-mode.org
	OperationMode string `json:"operationMode"`
	// when set to true, this field allow user registration.
	AllowRegistration bool `json:"enableUserRegistration"`
	// when set to true, email confirmation is required for registration.
	EmailConfirmationRequired bool `json:"emailConfirmationRequired"`
	// when set to true, all registration must be screened by the webmaster.
	ManualApproval bool `json:"requireManualApproval"`
	// documents that users would have to read and agree to before
	// registering a new account, e.g. terms of service. if "path"
	// starts with "http://" or "https://", the path would be rendered
	// as a link.  if it's not, it would be considered as na relpath to
	// `$staticAssetDirectory/rrdoc`, e.g.  "Terms Of Service":
	// "tos.md" links the file "$static/rrdoc/tos.md" to the title
	// "Terms Of Service".
	ReadingRequiredDocument []struct{Title string;Path string} `json:"readingRequiredDocument"`

	// git-related config.
	// NOTE(2025.12.30): we'll gradually move certain config options into here.
	GitConfig GitusGitConfig `json:"gitConfig"`

	// cosmetic things...
	
	// the name of the depot (i.e. the top level of the site)
	DepotName string `json:"depotName"`
	StaticAssetDirectory string `json:"staticAssetDirectory"`

	// http host name.
	HttpHostName string `json:"hostName"`
	// proper http host name (no trailing slash)
	properHttpHostName string

	// ssh host name.
	SshHostName string `json:"sshHostName"`
	// NOTE: the following two fields are for internal use only.
	// "proper" means the full URL with the scheme and no trailing slash.
	// "git" means the kind of address that is shown to the user as cloning
	// address. when a scheme part is necessary (i.e. in the case where
	// the port isn't 22) it *would* have a trailing slash. when the port
	// is 22 it *would* have a trailing colon `:`. this is setup this way
	// for the convenience of string concatenation: doing this would allow
	// us to directly concatenate it with the repository full name to get
	// the "correct" address usable by Git client.
	properSshHostName string
	gitSshHostName string

	BindAddress string `json:"bindAddress"`
	BindPort int `json:"bindPort"`

	// namespaces you need gitus to ignore during initial searching.
	// only valid when plain mode is enabled. (when plain mode is
	// disabled, all namespaces are visible by public by default,
	// even if they don't have any public repository and/or member.)
	IgnoreNamespace []string `json:"ignoreNamespace"`
	// repositories you need gitus to ignore during initial searching.
	// only valid when plain mode is enabled. this option is valid
	// whether you use namespace or not. when useNamespace is true,
	// you need to specify the "full name" of the repository ("full
	// name" i.e. `{namespace}:{repoName}`)
	IgnoreRepository []string `json:"ignoreRepository"`

	// the following database-related options are ignored when plain
	// mode is enabled,
	Database GitusDatabaseConfig `json:"database"`
	Session GitusSessionConfig `json:"session"`
	Mailer GitusMailerConfig `json:"mailer"`
	ReceiptSystem GitusReceiptSystemConfig `json:"receiptSystem"`

	// what should the instance display when the front page is visited.
	FrontPage GitusFrontPageConfig `json:"frontPage"`

	// global private/shutdown mode
	// supports the following values:
	// + "public" (unregistered users can view public repo)
	// + "private" (only registered users can view any repo)
	// + "shutdown"  (w/ allowed users) (only specified users can view any repo)
	// + "maintenance" maintenance mode
	GlobalVisibility string `json:"globalVisibility"`
	// usernames that are allowed to access the instance when the instance
	// is put in shutdown.
	FullAccessUser []string `json:"fullAccessUser"`
	// when the instance is put in shutdown mode, what content should we show
	// to the visitor.
	ShutdownMessage string `json:"shutdownMessage"`
	// shown when the instance is in maintenance mode.
	MaintenanceMessage string `json:"maintenanceMessage"`
	// shown when the instance is in plain mode & private mode.
	PrivateNoticeMessage string `json:"privateNoticeMessage"`

	// rate limiter
	// 0 turns off the rate limiter.
	MaxRequestInSecond float64 `json:"maxRequestInSecond"`

	// confirm code manager.
	// used in email 2fa.
	ConfirmCodeManager GitusConfirmCodeManagerConfig `json:"confirmCode"`

	// root directory for storing snippets.
	SnippetRoot string `json:"snippetRoot"`

	DefaultNewUserStatus model.GitusUserStatus `json:"defaultNewUserStatus"`
	DefaultNewUserNamespace string `json:"defaultNewUserNamespace"`

	// theme.
	// currently it's just a foreground color & a background color but there
	// could be more in the future.
	Theme GitusThemeConfig `json:"theme"`

	// "no-interactive-shell" message.
	// the message that would get displayed when people try to log in w/
	// a configured ssh public key.
	NoInteractiveShellMessage string `json:"noSshLoginMessage"`
}

const (
	GLOBAL_VISIBILITY_PUBLIC = "public"
	GLOBAL_VISIBILITY_PRIVATE = "private"
	GLOBAL_VISIBILITY_SHUTDOWN = "shutdown"
	GLOBAL_VISIBILITY_MAINTENANCE = "maintenance"
)

type GitusDatabaseConfig struct {
	// database type. currently only support "sqlite".
	Type string `json:"type"`
	// path to the database file. valid only when dbtype is sqlite;
	// has no effect otherwise.
	Path string `json:"path"`
	// TODO: this should be basing on the dir of the config file.
	properPath string
	// url to the database. valid only when dbtype is something that
	// is "hosted" as a server (unlike sqlite which is just one file).
	// has no effect when dbtype is sqlite.
	URL string `json:"url"`
	UserName string `json:"userName"`
	// name of the database. valid only when dbtype is something like
	// "postgre" or "mariadb". has no effect when dbtype is sqlite.
	DatabaseName string `json:"databaseName"`
	// password of the database. valid only when dbtype is something
	// like "postgre" or "mariadb". has no effect when dbtype is
	// sqlite.
	Password string `json:"password"`
	// table prefix of the database - in case you need to host
	// multiple gitus instance with the same database or you need
	// to make your gitus instance to share a database with other
	// applications.
	TablePrefix string `json:"tablePrefix"`
}

type GitusGitHTTPTransferProtocolDescriptor struct {
	// true if enabled.
	V1Dumb bool `json:"v1dumb"`
	V2 bool `json:"v2"`
}
type GitusGitConfig struct {
	HTTPCloneProtocol GitusGitHTTPTransferProtocolDescriptor `json:"httpCloneProtocol"`
}

type GitusSessionConfig struct {
	// session type. currently only support:
	// + "sqlite"
	// + redis-like dbs: "redis", "keydb", "valkey"
	//   + "valkey" is not tested, but should work fine.
	// + "memcached"
	// support for other types are also planned.
	Type string `json:"type"`
	// session database path. valid only when sessiontype is sqlite.
	Path string `json:"path"`
	// TODO: this should be basing on the dir of the config file.
	properPath string
	// session table prefix.
	// used as table prefix when type is "sqlite" and key prefix when
	// type is "redis"/"keydb"/"valkey"/"memcached".
	TablePrefix string `json:"tablePrefix"`
	// session host.
	// requirements for this value is as follows:
	// + "sqlite": not used
	// + "redis"/"keydb"/"valkey": in the format of "host:port"
	// + "memcached": in the format of "host:port"
	Host string `json:"host"`
	// username & password.
	// not used for "sqlite" and "memcached".
	UserName string `json:"userName"`
	Password string `json:"password"`
	// database number.
	// valid only when sessiontype is redis-like dbs, i.e.g "redis" or "keydb".
	// not used for "sqlite" and "memcached".
	DatabaseNumber int `json:"databaseNumber"`
}

type GitusMailerConfig struct {
	// email sender type. currently "gmail-plain" and "smtp" are supported.
	Type string `json:"type"`
	// smtp server & smtp port. technically not used if type is gmail-plain.
	// these fields are here for future use.
	SMTPServer string `json:"smtpServer"`
	SMTPPort int `json:"smtpPort"`
	SMTPAuth string `json:"smtpAuth"`
	User string `json:"user"`
	// email sender password. this would be stored in plain-text so one
	// should be using 
	Password string `json:"password"`
}

// NOTE: this is the same as GitusDatabaseConfig - i suspect that people
// would want to be able to search & filter specific kind of receipts and
// i couldn't figure out a good way to implement that w/ redis.
type GitusReceiptSystemConfig struct {
	// database type. currently only support "sqlite".
	Type string `json:"type"`
	// path to the database file. valid only when dbtype is sqlite;
	// has no effect otherwise.
	Path string `json:"path"`
	// TODO: this should be basing on the dir of the config file.
	properPath string
	// url to the database. valid only when dbtype is something that
	// is "hosted" as a server (unlike sqlite which is just one file).
	// has no effect when dbtype is sqlite.
	URL string `json:"url"`
	UserName string `json:"userName"`
	// name of the database. valid only when dbtype is something like
	// "postgre" or "mariadb". has no effect when dbtype is sqlite.
	DatabaseName string `json:"databaseName"`
	// password of the database. valid only when dbtype is something
	// like "postgre" or "mariadb". has no effect when dbtype is
	// sqlite.
	Password string `json:"password"`
	// table prefix of the database - in case you need to host
	// multiple gitus instance with the same database or you need
	// to make your gitus instance to share a database with other
	// applications.
	TablePrefix string `json:"tablePrefix"`
}

type GitusConfirmCodeManagerConfig struct {
	// type. currently only supports "in-memory".
	Type string `json:"type"`
	DefaultTimeoutMinute int `json:"defaultTimeoutMinute"`
}

type GitusFrontPageConfig struct {
	// + "all/namespace"
	// + "all/repository"
	// + "repository"
	// + "namespace"
	// + "static/markdown", "static/org", "static/text", "static/html"
	// if not set, it's "all/repository" by default.
	Type string `json:"type"`

	// effective only when Type is "namespace".
	Namespace string `json:"namespace"`

	// effective only when Type is "Repository".
	// should be "full name", i.e. "{namespace_name}:{repo_name}".
	Repository string `json:"repository"`

	// effective only when Type starts with "static".
	FileContent string `json:"fileContent"`
}

type GitusThemeConfig struct {
	ForegroundColor string `json:"foregroundColor"`
	BackgroundColor string `json:"backgroundColor"`
}

// proper http host name. no trailing slash.
func (cfg *GitusConfig) ProperHTTPHostName() string {
	// proper http host name. no trailing slash.
	return cfg.properHttpHostName
}

func (cfg *GitusConfig) ProperSSHHostName() string {
	return cfg.properSshHostName
}

func (cfg *GitusConfig) ProperDatabasePath() string {
	return cfg.Database.properPath
}

func (cfg *GitusConfig) ProperSessionPath() string {
	return cfg.Session.properPath
}

func (cfg *GitusConfig) ProperReceiptSystemPath() string {
	return cfg.ReceiptSystem.properPath
}

func (cfg *GitusConfig) GitSSHHostName() string {
	return cfg.gitSshHostName
}

func (cfg *GitusConfig) LockForSync() {
	cfg.lock.Lock()
}

func (cfg *GitusConfig) Unlock() {
	cfg.lock.Unlock()
}

func (cfg *GitusConfig) GetRRDocTitle(p string) string {
	for _, v := range cfg.ReadingRequiredDocument {
		if v.Path == p { return v.Title }
	}
	return ""
}

const (
	OP_MODE_PLAIN = "plain"
	OP_MODE_SIMPLE = "simple"
	OP_MODE_NORMAL = "normal"
)

func (cfg *GitusConfig) IsInPlainMode() bool {
	return cfg.OperationMode == OP_MODE_PLAIN
}

func CreateConfigFile(p string) error {
	f, err := os.OpenFile(
		p,
		os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_TRUNC,
		0644,
	)
	if err != nil { return err }
	defer f.Close()
	marshalRes, err := json.MarshalIndent(GitusConfig{
		Version: 0,
		GitRoot: "",
		GitUser: "git",
		UseNamespace: false,
		OperationMode: "plain",
		AllowRegistration: true,
		EmailConfirmationRequired: true,
		ManualApproval: true,
		DepotName: "Gitus",
		StaticAssetDirectory: "static/",
		BindAddress: "127.0.0.1",
		BindPort: 8000,
		IgnoreNamespace: nil,
		IgnoreRepository: nil,
		GlobalVisibility: "public",
		FullAccessUser: []string{"admin"},
		GitConfig: GitusGitConfig{
			HTTPCloneProtocol: GitusGitHTTPTransferProtocolDescriptor{
				V1Dumb: true,
				V2: true,
			},
		},
		Database: GitusDatabaseConfig{
			Type: "sqlite",
			Path: "",
			URL: "",
			UserName: "",
			DatabaseName: "",
			Password: "",
			TablePrefix: "gitus",
		},
		Session: GitusSessionConfig{
			Type: "sqlite",
			Path: "",
			TablePrefix: "",
			Host: "",
			UserName: "",
			Password: "",
			DatabaseNumber: 0,
		},
		Mailer: GitusMailerConfig{
			Type: "gmail-plain",
			SMTPServer: "",
			SMTPPort: 0,
			User: "",
			Password: "",
		},
		ReceiptSystem: GitusReceiptSystemConfig{
			Type: "sqlite",
			Path: "",
			URL: "",
			UserName: "",
			DatabaseName: "",
			Password: "",
			TablePrefix: "gitus_receipt_",
		},
		MaxRequestInSecond: 500,
		ConfirmCodeManager: GitusConfirmCodeManagerConfig{
			Type: "in-memory",
			DefaultTimeoutMinute: 5,
		},
		SnippetRoot: "",
		DefaultNewUserStatus: model.GitusUserStatus(model.NORMAL_USER),
		DefaultNewUserNamespace: "",
		FrontPage: GitusFrontPageConfig{
			Type: "all/repository",
			Namespace: "",
			Repository: "",
			FileContent: "",
		},
		Theme: GitusThemeConfig{
			ForegroundColor: "black",
			BackgroundColor: "white",
		},
		NoInteractiveShellMessage: "Direct shell access is forbidden on this host.",
	}, "", "    ")
	if err != nil { return err }
	f.Write(marshalRes)
	return nil
}

func (c *GitusConfig) RecalculateProperPath() error {
	// fix http host name & ssh host name...
	c.properHttpHostName = c.HttpHostName
	if strings.TrimSpace(c.HttpHostName) != "" {
		if !strings.HasPrefix(c.properHttpHostName, "http://") && !strings.HasPrefix(c.properHttpHostName, "https://") {
			c.properHttpHostName = "http://" + c.properHttpHostName
		}
		c.properHttpHostName = strings.TrimSuffix(c.properHttpHostName, "/")
	} else { c.properHttpHostName = "" }
	
	c.properSshHostName = c.SshHostName
	if strings.TrimSpace(c.SshHostName) != "" {
		if !strings.HasSuffix(c.properSshHostName, "ssh://") {
			c.properSshHostName = "ssh://" + c.properSshHostName
		}
		c.properSshHostName = strings.TrimSuffix(c.properSshHostName, "/")
		u, err := url.Parse(c.properSshHostName)
		if err != nil { return err }
		// git username override.
		actualU := &url.URL{
			Scheme: "ssh",
			User: url.User(c.GitUser),
			Host: u.Host,
			Path: "",
			RawPath: "",
			OmitHost: u.OmitHost,
			ForceQuery: false,
			RawQuery: "",
			Fragment: "",
			RawFragment: "",
		}
		c.properSshHostName = actualU.String()
		host := u.Host
		if strings.ContainsRune(host, ':') {
			h, _, _ := net.SplitHostPort(host)
			host = h
		}
		if u.Port() == "" || u.Port() == "22" {
			c.gitSshHostName = fmt.Sprintf("%s@%s:", c.GitUser, host)
		} else {
			c.gitSshHostName = actualU.String()
			if !strings.HasSuffix(c.gitSshHostName, "/") {
				c.gitSshHostName = c.gitSshHostName + "/"
			}
		}
	}

	configDir := path.Dir(c.FilePath)
	if c.Database.Type == "sqlite" {
		var rp string
		if path.IsAbs(c.Database.Path) {
			rp = c.Database.Path
		} else {
			rp = path.Join(configDir, c.Database.Path)
		}
		c.Database.properPath = rp
	}

	if c.Session.Type == "sqlite" {
		var sp string
		if path.IsAbs(c.Session.Path) {
			sp = c.Session.Path
		} else {
			sp = path.Join(configDir, c.Session.Path)
		}
		c.Session.properPath = sp
	}

	if c.ReceiptSystem.Type == "sqlite" {
		var rsp string
		if path.IsAbs(c.ReceiptSystem.Path) {
			rsp = c.ReceiptSystem.Path
		} else {
			rsp = path.Join(configDir, c.ReceiptSystem.Path)
		}
		c.ReceiptSystem.properPath = rsp
	}
	
	return nil
}

func LoadConfigFile(p string) (*GitusConfig, error) {
	s, err := os.ReadFile(p)
	if err != nil { return nil, err }
	var c GitusConfig
	err = json.Unmarshal(s, &c)
	if err != nil { return nil, err }
	c.FilePath = p
	err = c.RecalculateProperPath()
	if err != nil { return nil, err }
	return &c, nil
}

func (cfg *GitusConfig) Sync() error {
	p := cfg.FilePath
	s, err := json.MarshalIndent(cfg, "", "    ")
	if err != nil { return err }
	st, err := os.Stat(p)
	if err != nil && !os.IsNotExist(err) { return err }
	var f *os.File
	if os.IsNotExist(err) {
		f, err = os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	} else {
		f, err = os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, st.Mode())
	}
	if err != nil { return err }
	defer f.Close()
	_, err = f.Write(s)
	if err != nil { return err }
	err = f.Sync()
	if err != nil { return err }
	return nil
}

func (cfg *GitusConfig) ReadNamespaceSimpleModeConfig(name string) (*model.SimpleModeNamespaceConfig, error) {
	f := path.Join(cfg.GitRoot, "__gitus", "__repo_config", "gitus_sync", name, "config.json")
	return model.ReadNamespaceConfigFromFile(f)
}

func (cfg *GitusConfig) ReadRepositorySimpleModeConfig(namespace string, name string) (*model.SimpleModeRepositoryConfig, error) {
	var f string
	if cfg.UseNamespace {
		f = path.Join(cfg.GitRoot, "__gitus", "__repo_config", "gitus_sync", namespace, name, "config.json")
	} else {
		f = path.Join(cfg.GitRoot, "__repo_config", "gitus_sync", name, "config.json")
	}
	return model.ReadRepositoryConfigFromFile(f)
}

func (cfg *GitusConfig) GetAllRepositoryPlain() ([]*model.Repository, error) {
	if cfg.UseNamespace {
		m, err := cfg.GetAllNamespacePlain()
		if err != nil { return nil, err }
		res := make([]*model.Repository, 0)		
		for k := range m {
			r, err := cfg.GetAllRepositoryByNamespacePlain(k)
			if err != nil { return nil, err }
			for _, i := range r {
				i.Namespace = k
				res = append(res, i)
			}
		}
		return res, nil
	}
	gitPath := cfg.GitRoot
	res := make([]*model.Repository, 0)
	l, err := os.ReadDir(gitPath)
	if err != nil { return nil, err }
	for _, item := range l {
		repoName := item.Name()
		p := path.Join(gitPath, item.Name())
		if !gitlib.IsValidGitDirectory(p) {
			p = path.Join(gitPath, item.Name(), ".git")
		}
		if !gitlib.IsValidGitDirectory(p) {
			continue
		}
		if strings.HasSuffix(repoName, ".git") {
			repoName = repoName[:len(repoName)-len(".git")]
			if len(repoName) <= 0 { continue }
		}
		if cfg.OperationMode == OP_MODE_SIMPLE {
			m, err := cfg.ReadRepositorySimpleModeConfig("", repoName)
			if err != nil { continue }
			if m.Repository.Visibility == model.SIMPLE_MODE_VISIBILITY_PRIVATE {
				continue
			}
		}
		k := gitlib.NewLocalGitRepository(p)
		res = append(res, &model.Repository{
			Namespace: "",
			Name: repoName,
			Description: k.Description,
			AccessControlList: nil,
			Status: model.REPO_NORMAL_PUBLIC,
			Repository: k,
		})
	}
	return res, nil
}

func (cfg *GitusConfig) GetAllRepositoryByNamespacePlain(ns string) (map[string]*model.Repository, error) {
	gitPath := cfg.GitRoot
	res := make(map[string]*model.Repository, 0)
	nsPath := path.Join(gitPath, ns)
	l, err := os.ReadDir(nsPath)
	if err != nil { return nil, err }
	
	for _, item := range l {
		repoName := item.Name()
		p := path.Join(nsPath, item.Name())
		if !gitlib.IsValidGitDirectory(p) {
			p = path.Join(nsPath, item.Name(), ".git")
		}
		if !gitlib.IsValidGitDirectory(p) {
			continue
		}
		if strings.HasSuffix(repoName, ".git") {
			repoName = repoName[:len(repoName)-len(".git")]
			if len(repoName) <= 0 { continue }
		}
		if cfg.OperationMode == OP_MODE_SIMPLE {
			m, err := cfg.ReadRepositorySimpleModeConfig(ns, repoName)
			if err != nil { continue }
			if m.Repository.Visibility == model.SIMPLE_MODE_VISIBILITY_PRIVATE {
				continue
			}
		}
		k := gitlib.NewLocalGitRepository(p)
		res[repoName] = &model.Repository{
			Type: model.GuessRepositoryType(p),
			Namespace: ns,
			Name: repoName,
			Description: k.Description,
			AccessControlList: nil,
			Status: model.REPO_NORMAL_PUBLIC,
			Repository: k,
		}
	}
	return res, nil
}

func (cfg *GitusConfig) GetAllNamespacePlain() (map[string]*model.Namespace, error) {
	res := make(map[string]*model.Namespace, 0)
	if !cfg.UseNamespace {
		ns, err := model.NewNamespace("", cfg.GitRoot)
		if err != nil { return nil, err }
		// TODO: add privated repo as well.
		for _, item := range cfg.IgnoreRepository {
			k := strings.Split(item, ":")
			if len(k) >= 2 {
				if k[0] != "" { continue }
				delete(ns.RepositoryList, k[1])
			} else {
				delete(ns.RepositoryList, k[0])
			}
		}
		res[""] = ns
		return res, nil
	}
	l, err := os.ReadDir(cfg.GitRoot)
	if err != nil { return nil, err }
	for _, item := range l {
		namespaceName := item.Name()
		if !model.ValidNamespaceName(namespaceName) { continue }
		if cfg.OperationMode == OP_MODE_PLAIN {
			_, shouldIgnore := slices.BinarySearch(cfg.IgnoreNamespace, namespaceName)
			if shouldIgnore { continue }
		} else if cfg.OperationMode == OP_MODE_SIMPLE {
			m, err := cfg.ReadNamespaceSimpleModeConfig(namespaceName)
			if err != nil { continue }
			if m.Namespace.Visibility == model.SIMPLE_MODE_VISIBILITY_PRIVATE {
				continue
			}
		}
		p := path.Join(cfg.GitRoot, namespaceName)
		ns, err := model.NewNamespace(namespaceName, p)
		if err != nil { return nil, err }
		// (i'm worried that) this might be slow...
		if cfg.OperationMode == OP_MODE_PLAIN {
			for _, item := range cfg.IgnoreRepository {
				k := strings.Split(item, ":")
				if len(k) < 2 { continue }
				if k[0] != namespaceName { continue }
				delete(ns.RepositoryList, k[1])
			}
		} else {
			// TODO: fix this.
		}
		res[namespaceName] = ns
	}
	return res, nil
}

// TODO: find a better & more efficient way to do this.
func (cfg *GitusConfig) SearchAllNamespacePlain(pattern string) (map[string]*model.Namespace, error) {
	preres, err := cfg.GetAllNamespacePlain()
	if err != nil { return nil, err }
	res := make(map[string]*model.Namespace, 0)
	for k, v := range preres {
		if strings.Contains(v.Name, pattern) || strings.Contains(v.Title, pattern) {
			res[k] = v
		}
	}
	return res, nil
}

func (cfg *GitusConfig) SearchAllRepositoryPlain(pattern string) ([]*model.Repository, error) {
	preres, err := cfg.GetAllRepositoryPlain()
	if err != nil { return nil, err }
	res := make([]*model.Repository, 0)
	for _, v := range preres {
		if strings.Contains(v.Name, pattern) || strings.Contains(v.Namespace, pattern) {
			res = append(res, v)
		}
	}
	return res, nil
}

