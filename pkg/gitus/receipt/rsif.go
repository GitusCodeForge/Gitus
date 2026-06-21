package receipt

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"time"
)

type Receipt struct {
	Id string `json:"id"`
	Command []string `json:"command"`
	IssueTime int64 `json:"issueTime"` // timestamp
	TimeoutMinute int64 `json:"timeoutMinute"`
}

type GitusReceiptSystemInterface interface {
	IsReceiptSystemUsable() (bool, error)
	Install() error
	Dispose() error
	RetrieveReceipt(rid string) (*Receipt, error)
	IssueReceipt(timeoutMinute int64, command []string) (string, error)
	CancelReceipt(rid string) error
	GetAllReceipt(pageNum int, pageSize int) ([]*Receipt, error)
	SearchReceipt(q string, pageNum int, pageSize int) ([]*Receipt, error)
	EditReceipt(id string, robj *Receipt) error
}

const passchdict = "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
func NewReceiptId() string {
	res := make([]byte, 0)
	rmax := big.NewInt(int64(len(passchdict)))
	for range 48 {
		n, _ := rand.Int(rand.Reader, rmax)
		res = append(res, passchdict[n.Uint64()])
	}
	return string(res)
}

func SerializeReceiptCommand(s []string) string {
	// we're only marshalling string slices, this shouldn't return any error...
	b, err := json.Marshal(s)
	if err != nil { panic(err) }
	return string(b)
}

func cmdArgEscape(s string) string {
	// escape for receipt command.  receipt command is separated by
	// commas, thus special care needs to be taken if the args
	// themselves contain commas.
	quoted := false
	if strings.ContainsRune(s, ',') { quoted = true }
	r := new(strings.Builder)
	if quoted { r.WriteRune('"') }
	for _, ch := range s {
		if quoted && ch == '"' {
			r.WriteString("\\\"")
		} else {
			r.WriteRune(ch)
		}
	}
	if quoted { r.WriteRune('"') }
	return r.String()
}
func NewReceiptCommand(s... string) string {
	res := make([]string, 0)
	for _, item := range s {
		res = append(res, cmdArgEscape(item))
	}
	return strings.Join(res, ",")
}
func ParseReceiptCommand(s string) []string {
	var res []string
	err := json.Unmarshal([]byte(s), &res)
	if err != nil { panic(err) }
	return res
}

func SerializeReceipt(r *Receipt) string {
	res, err := json.Marshal(r)
	if err != nil { panic(err) }
	return string(res)
}

func DeserializeReceipt(s string) *Receipt {
	r := new(Receipt)
	err := json.Unmarshal([]byte(s), r)
	if err != nil { panic(err) }
	return r
}

func (r *Receipt) Expired() bool {
	return (time.Now().Unix() - r.IssueTime) >= r.TimeoutMinute * 60
}

const (
	// confirm-registration,{username},{email},{passwordHash}
	CONFIRM_REGISTRATION = "confirm-registration"
	// reset-password,{username}
	RESET_PASSWORD = "reset-password"
	// verify-email,{username},{email}
	VERIFY_EMAIL = "verify-email"
)

var ErrUnsupportedSystemType = errors.New("Unsupported receipt system type")

