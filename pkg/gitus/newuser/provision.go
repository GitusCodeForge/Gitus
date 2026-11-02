package newuser

import (
	"errors"
	"strings"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/log"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
)

// FinalStatus is the status a new account ends up with once any approval or
// confirmation gate has been passed. The "approval needed" and "confirmation
// needed" statuses are gates, not resting states, so they become NORMAL_USER;
// every other configured default is kept as it is, including
// NORMAL_USER_NO_NEW_NAMESPACE and BANNED.
func FinalStatus(cfg *gitus.GitusConfig) model.GitusUserStatus {
	status := cfg.DefaultNewUserStatus
	if status == 0 {
		status = model.NORMAL_USER
	}
	switch status {
	case model.NORMAL_USER_APPROVAL_NEEDED, model.NORMAL_USER_CONFIRM_NEEDED:
		return model.NORMAL_USER
	}
	return status
}

// ValidDefaultStatus reports whether status can be used as the default for new
// accounts. Administrators and super administrators are deliberately left
// out: the site-wide default must not hand out privileges.
func ValidDefaultStatus(status model.GitusUserStatus) bool {
	switch status {
	case model.NORMAL_USER,
		model.NORMAL_USER_APPROVAL_NEEDED,
		model.NORMAL_USER_CONFIRM_NEEDED,
		model.NORMAL_USER_NO_NEW_NAMESPACE,
		model.BANNED:
		return true
	}
	return false
}

// ProvisionNewUser applies the site-wide defaults to an account that has just
// been created (or has just finished the approval/confirmation gates):
//
//   - a NORMAL_USER gets a personal namespace named after the account, when
//     namespaces are enabled. NORMAL_USER_NO_NEW_NAMESPACE deliberately does
//     not: that status exists to stop the account from making namespaces;
//   - the account is added to the configured default namespace as a member
//     that may add repositories, so it has somewhere to put them.
//
// The default namespace membership does not overwrite anything: when the
// namespace does not exist the membership is skipped with a warning, because
// the account itself has already been created by the time this runs.
func ProvisionNewUser(dbif db.GitusDatabaseInterface, cfg *gitus.GitusConfig, username string, status model.GitusUserStatus) error {
	if dbif == nil || cfg == nil {
		return nil
	}
	if cfg.UseNamespace && status == model.NORMAL_USER {
		if err := ensurePersonalNamespace(dbif, username); err != nil {
			return err
		}
	}
	return AddToDefaultNamespace(dbif, cfg, username)
}

// AddToDefaultNamespace makes username a member of the site's default
// namespace, with the right to add repositories but nothing else. It does
// nothing when no default namespace is configured.
func AddToDefaultNamespace(dbif db.GitusDatabaseInterface, cfg *gitus.GitusConfig, username string) error {
	if !cfg.UseNamespace {
		return nil
	}
	nsName := strings.TrimSpace(cfg.DefaultNewUserNamespace)
	if len(nsName) <= 0 {
		return nil
	}
	if _, err := dbif.GetNamespaceByName(nsName); err != nil {
		if errors.Is(err, db.ErrEntityNotFound) {
			log.Warnf("Default new-user namespace %q no longer exists; %s was not added to it", nsName, username)
			return nil
		}
		return err
	}
	member := &model.ACLTuple{AddRepository: true}
	if err := dbif.SetNamespaceACL(nsName, username, member); err != nil {
		// The namespace can disappear between the check and the update.
		if errors.Is(err, db.ErrEntityNotFound) {
			log.Warnf("Default new-user namespace %q no longer exists; %s was not added to it", nsName, username)
			return nil
		}
		return err
	}
	return nil
}

// ensurePersonalNamespace creates the namespace named after the account. It
// leaves an existing namespace alone; registration checks the name against the
// existing users and namespaces first, and an administrator-created account
// may intentionally share a namespace name.
func ensurePersonalNamespace(dbif db.GitusDatabaseInterface, username string) error {
	if _, err := dbif.GetNamespaceByName(username); err == nil {
		return nil
	} else if !errors.Is(err, db.ErrEntityNotFound) {
		return err
	}
	if _, err := dbif.RegisterNamespace(username, username); err != nil {
		if errors.Is(err, db.ErrEntityAlreadyExists) {
			return nil
		}
		return err
	}
	return nil
}
