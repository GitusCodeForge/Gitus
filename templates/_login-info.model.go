//go:build ignore

package templates

type LoginInfoModel struct {
	LoggedIn bool
	UserName string
	UserFullName string
	UserEmail string
	UserSessionId string
	UserCSRFToken string
	IsOwner bool
	IsStrictOwner bool
	IsSettingMember bool
	IsAdmin bool
	IsSuperAdmin bool
}

