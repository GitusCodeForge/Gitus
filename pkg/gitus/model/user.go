package model

import "net/mail"

type GitusUserStatus int

const (
	NORMAL_USER GitusUserStatus = 1
	NORMAL_USER_APPROVAL_NEEDED GitusUserStatus = 2
	NORMAL_USER_CONFIRM_NEEDED GitusUserStatus = 3
	ADMIN GitusUserStatus = 4
	SUPER_ADMIN GitusUserStatus = 5
	BANNED GitusUserStatus = 7
	NORMAL_USER_NO_NEW_NAMESPACE GitusUserStatus = 8
)

func ValidUserName(s string) bool {
	for _, k := range s {
		if !(('0' <= k && k <= '9') || ('A' <= k && k <= 'Z') || ('a' <= k && k <= 'z') || k == '_' || k == '-') { return false }
	}
	return true
}

// ValidEmail reports whether s is a bare email address. A display name
// ("Alice <alice@example.com>") is rejected: the value is stored verbatim and
// used as a mail recipient and as GIT_AUTHOR_EMAIL, so it has to be exactly an
// address.
func ValidEmail(s string) bool {
	addr, err := mail.ParseAddress(s)
	if err != nil { return false }
	return addr.Name == "" && addr.Address == s
}

// GitusUserLoginInfo is the subset of a user that UseLoginInfo needs on every
// authenticated request. Fetching a full GitusUser there means decoding the
// 2FA config and the website preference JSON on every page load for nothing.
type GitusUserLoginInfo struct {
	Name string `json:"name"`
	Title string `json:"title"`
	Email string `json:"email"`
	Status GitusUserStatus `json:"status"`
}

type GitusUser struct {
	// user name. 
	Name string `json:"name"`
	// user "title"
	Title string `json:"title"`
	// user email.
	Email string `json:"email"`
	// user bio.
	Bio string `json:"bio"`
	Website string `json:"website"`
	// password hash.
	PasswordHash string `json:"passwordHash"`
	RegisterTime int64 `json:"regTime"`
	Status GitusUserStatus `json:"status"`
	// AuthKey []GitusAuthKey `json:"authKey"`
	// SigningKey []GitusSigningKey `json:"signKey"`

	TFAConfig GitusUser2FAConfig `json:"2fa"`
	WebsitePreference GitusUserWebsitePreference `json:"preference"`
}

type GitusUser2FAConfig struct {
	Email struct{
		Enable bool `json:"enable"`
	} `json:"email"`
	// OTP is the authenticator app (HOTP/TOTP) second factor. The secret is
	// stored in the user row, so it travels with the usual user backup.
	OTP GitusUser2FAOTPConfig `json:"otp"`
}

// GitusUser2FAOTPConfig is the authenticator app configuration of a user.
type GitusUser2FAOTPConfig struct {
	Enable bool `json:"enable"`
	// Pending is set between "start setup" and the first code the user
	// confirms: the secret exists but is not enforced yet.
	Pending bool `json:"pending"`
	// "totp" (default) or "hotp".
	Type string `json:"type"`
	Secret string `json:"secret"`
	Digits int `json:"digits"`
	Period int `json:"period"`
	// Counter is the next HOTP counter to accept; unused for TOTP.
	Counter uint64 `json:"counter"`
}

// AnySecondFactor reports whether the account requires a second factor at
// login.
func (cfg *GitusUser2FAConfig) AnySecondFactor() bool {
	return cfg.Email.Enable || cfg.OTP.Enable
}

// OTPIsTOTP reports whether the authenticator factor is time based; anything
// that is not an explicit "hotp" is treated as TOTP.
func (cfg *GitusUser2FAConfig) OTPIsTOTP() bool {
	return cfg.OTP.Type != "hotp"
}

type GitusUserWebsitePreference struct {
	ForegroundColor string `json:"foregroundColor"`
	BackgroundColor string `json:"backgroundColor"`
	// ignore user custom fgcolor/bgcolor config if true.
	UseSiteWideThemeConfig bool `json:"useSiteWideThemeConfig"`
	// whether to load ui w/ components that requires javascript or
	// load ui with zero javascript requirements.
	UseJavascript bool `json:"useJavascript"`
}



