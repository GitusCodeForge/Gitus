package sqlite

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	_ "github.com/mattn/go-sqlite3"
)

func (dbif *SqliteGitusDatabaseInterface) Dispose() error {
	return dbif.connection.Close()
}

func (dbif *SqliteGitusDatabaseInterface) GetUserByName(name string) (*model.GitusUser, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT user_name, user_title, user_email, user_bio, user_website, user_status, user_password_hash, user_2fa_config, user_website_preference
FROM %s_user
WHERE user_name = ?
`, pfx))
	if err != nil { return nil, err }
	var username, title, email, bio, website, ph, tfaConfig, websitePreference string
	var status int
	err = stmt.QueryRow(name).Scan(&username, &title, &email, &bio, &website, &status, &ph, &tfaConfig, &websitePreference)
	if err == sql.ErrNoRows { return nil, db.ErrEntityNotFound }
	if err != nil { return nil, err }
	var tfa model.GitusUser2FAConfig
	err = json.Unmarshal([]byte(tfaConfig), &tfa)
	if err != nil { return nil, err }
	var wPref model.GitusUserWebsitePreference
	err = json.Unmarshal([]byte(websitePreference), &wPref)
	if err != nil { return nil, err }
	return &model.GitusUser{
		Name: username,
		Title: title,
		Email: email,
		Bio: bio,
		Website: website,
		Status: model.GitusUserStatus(status),
		PasswordHash: ph,
		TFAConfig: tfa,
		WebsitePreference: wPref,
	}, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAuthKeyByName(userName string, keyName string) (*model.GitusAuthKey, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT key_text
FROM %s_user_authkey
WHERE user_name = ? AND key_name = ?
`, pfx))
	if err != nil { return nil, err }
	r := stmt.QueryRow(userName, keyName)
	err = r.Err()
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, db.ErrEntityNotFound
		}
		return nil, err
	}
	kt := ""
	err = r.Scan(&kt)
	if err != nil { return nil, err }
	return &model.GitusAuthKey{
		UserName: userName,
		KeyName: keyName,
		KeyText: kt,
	}, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllAuthKeyByUsername(name string) ([]model.GitusAuthKey, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT key_name, key_text
FROM %s_user_authkey
WHERE user_name = ?
`, pfx))
	if err != nil { return nil, err }
	r, err := stmt.Query(name)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]model.GitusAuthKey, 0)
	for r.Next() {
		var keyName, keyText string
		err = r.Scan(&keyName, &keyText)
		if err != nil { return nil, err }
		res = append(res, model.GitusAuthKey{
			UserName: name,
			KeyText: keyText,
			KeyName: keyName,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) RegisterAuthKey(username string, keyname string, keytext string) error {
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT 1 FROM %s_user_authkey WHERE user_name = ? AND key_name = ?
`, pfx))
	if err != nil { return err }
	r := stmt1.QueryRow(username, keyname)
	if r.Err() != nil { return r.Err() }
	var verdict string
	err = r.Scan(&verdict)
	if err != nil && err != sql.ErrNoRows { return err }
	if err == nil {
		return db.ErrEntityAlreadyExists
	}
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt2, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %s_user_authkey(user_name, key_name, key_text)
VALUES (?,?,?)
`, pfx))
	if err != nil { return err }
	_, err = stmt2.Exec(username, keyname, keytext)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) UpdateAuthKey(username string, keyname string, keytext string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_user_authkey SET key_text = ? WHERE user_name = ? AND key_name = ?
`, pfx))
	if err != nil { return err }
	defer stmt.Close()
	_, err = stmt.Exec(keytext, username, keyname)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) RemoveAuthKey(username string, keyname string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %s_user_authkey
WHERE user_name = ? AND key_name = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(username, keyname)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllSignKeyByUsername(name string) ([]model.GitusSigningKey, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT key_name, key_text
FROM %s_user_signkey
WHERE user_name = ?
`, pfx))
	if err != nil { return nil, err }
	r, err := stmt.Query(name)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]model.GitusSigningKey, 0)
	for r.Next() {
		var keyName, keyText string
		err = r.Scan(&keyName, &keyText)
		if err != nil { return nil, err }
		res = append(res, model.GitusSigningKey{
			UserName: name,
			KeyText: keyText,
			KeyName: keyName,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetSignKeyByName(userName string, keyName string) (*model.GitusSigningKey, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT key_text FROM %s_user_signkey
WHERE user_name = ? AND key_name = ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r := stmt.QueryRow(userName, keyName)
	if r.Err() != nil { return nil, r.Err() }
	var text string
	err = r.Scan(&text)
	if err != nil { return nil, err }
	return &model.GitusSigningKey{
		UserName: userName,
		KeyName: keyName,
		KeyText: text,
	}, nil
}

func (dbif *SqliteGitusDatabaseInterface) UpdateSignKey(username string, keyname string, keytext string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_user_signkey SET key_text = ? WHERE user_name = ? AND key_name = ?
`, pfx))
	if err != nil { return err }
	defer stmt.Close()
	_, err = stmt.Exec(keytext, username, keyname)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) RegisterSignKey(username string, keyname string, keytext string) error {
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT 1 FROM %s_user_signkey WHERE user_name = ? AND key_name = ?
`, pfx))
	if err != nil { return err }
	r := stmt1.QueryRow(username, keyname)
	if r.Err() != nil { return r.Err() }
	var verdict string
	err = r.Scan(&verdict)
	if err != nil && err != sql.ErrNoRows { return err }
	if err == nil {
		return db.ErrEntityAlreadyExists
	}
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt2, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %s_user_signkey(user_name, key_name, key_text)
VALUES (?,?,?)
`, pfx))
	if err != nil { return err }
	_, err = stmt2.Exec(username, keyname, keytext)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) RemoveSignKey(username string, keyname string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %s_user_signkey
WHERE user_name = ? AND key_name = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(username, keyname)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) RegisterUser(name string, email string, passwordHash string, status model.GitusUserStatus) (*model.GitusUser, error) {
	pfx := dbif.config.Database.TablePrefix
	t := time.Now().Unix()
	tx, err := dbif.connection.Begin()
	if err != nil { return nil, err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %s_user(user_name, user_title, user_email, user_bio, user_website, user_password_hash, user_reg_datetime, user_status, user_2fa_config, user_website_preference)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`, pfx))
	if err != nil { return nil, err }
	_, err = stmt.Exec(name, name, email, new(string), new(string), passwordHash, t, status, `{"email":{"enable":false}}`, `{"useSiteWideThemeConfig":true}`)
	if err != nil { tx.Rollback(); return nil, err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return nil, err }
	// we delete whatever we have inside the old ns.
	// this should be ok since when a user exists as "deleted"
	// state the actions above should've violated unique constraint
	// and triggered an error already. of course we should remove
	// "deleted" user regularly from the dbto prevent possible sabotage.
	userNsPath := path.Join(dbif.config.GitRoot, name)
	if !db.IsSubDir(dbif.config.GitRoot, userNsPath) {
		return nil, db.ErrInvalidLocation
	}
	err = os.RemoveAll(userNsPath)
	if err != nil && !os.IsNotExist(err) { tx.Rollback(); return nil, err }
	err = os.MkdirAll(userNsPath, os.ModeDir|0755)
	// TODO: chown.
	if err != nil { tx.Rollback(); return nil, err }
	return &model.GitusUser{
		Name: name,
		Title: name,
		Email: email,
		PasswordHash: passwordHash,
		Bio: "",
		Website: "",
		RegisterTime: t,
	}, nil
}

func (dbif *SqliteGitusDatabaseInterface) UpdateUserInfo(name string, uobj *model.GitusUser) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_user
SET
    user_title = ?, user_email = ?, user_bio = ?,
    user_website = ?, user_status = ?, user_2fa_config = ?,
    user_website_preference = ?
WHERE
    user_name = ?
`, pfx))
	if err != nil { return err }
	tfa, _ := json.Marshal(uobj.TFAConfig)
	wpref, _ := json.Marshal(uobj.WebsitePreference)
	_, err = stmt.Exec(uobj.Title, uobj.Email, uobj.Bio, uobj.Website, uobj.Status, string(tfa), string(wpref), name)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) UpdateUserPassword(name string, newPasswordHash string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_user
SET user_password_hash = ?
WHERE user_name = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(newPasswordHash, name)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) UpdateUserStatus(name string, newStatus model.GitusUserStatus) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_user
SET user_status = ?
WHERE user_name = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(newStatus, name)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) HardDeleteUserByName(name string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %s_user WHERE user_name = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(name)
	if err != nil { return err }
	userNsPath := path.Join(dbif.config.GitRoot, name)
	err = os.RemoveAll(userNsPath)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) GetNamespaceByName(name string) (*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %s_namespace
WHERE ns_name = ?
`, pfx))
	if err != nil { return nil, err }
	r := stmt.QueryRow(name)
	if r.Err() != nil { return nil, r.Err() }
	var title, desc, email, owner, acl string
	var reg_date int64
	var status int
	err = r.Scan(&title, &desc, &email, &owner, &reg_date, &status, &acl)
	if err == sql.ErrNoRows {
		return nil, db.ErrEntityNotFound
	}
	if err != nil { return nil, err }
	a, err := model.ParseACL(acl)
	if err != nil { return nil, err }
	return &model.Namespace{
		Name: name,
		Title: title,
		Description: desc,
		Email: email,
		Owner: owner,
		RegisterTime: reg_date,
		ACL: a,
		Status: model.GitusNamespaceStatus(status),
	}, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetRepositoryByName(nsName string, repoName string) (*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook, rowid
FROM %s_repository
WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return nil, err }
	r := stmt.QueryRow(nsName, repoName)
	if r.Err() != nil { return nil, r.Err() }
	var desc, owner, acl, forkOriginNs, forkOriginName, labelList, webhookstr string
	var status int
	var rowid int64
	var repoType uint8
	err = r.Scan(&repoType, &desc, &owner, &acl, &status, &forkOriginNs, &forkOriginName, &labelList, &webhookstr, &rowid)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, db.ErrEntityNotFound
		}
		return nil, err
	}
	var tags []string = nil
	if len(labelList) > 0 {
		tags = strings.Split(labelList[1:len(labelList)-1], "}{")
	}
	p := path.Join(dbif.config.GitRoot, nsName, repoName)
	res, err := model.NewRepository(nsName, repoName, gitlib.NewLocalGitRepository(p))
	res.AbsId = rowid
	res.Type = repoType
	res.Owner = owner
	res.Status = model.GitusRepositoryStatus(status)
	res.ForkOriginNamespace = forkOriginNs
	res.ForkOriginName = forkOriginName
	res.RepoLabelList = tags
	aclobj, err := model.ParseACL(acl)
	if err != nil { return nil, err }
	res.AccessControlList = aclobj
	webhookobj, err := model.ParseWebHookConfig(webhookstr)
	res.WebHookConfig = webhookobj
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) RegisterNamespace(name string, ownerUsername string) (*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return nil, err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %s_namespace(ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl)
VALUES (?,?,?,?,?,?,?,?)
`, pfx))
	t := time.Now().Unix()
	if err != nil { return nil, err }
	_, err = stmt.Exec(name, name, "", "", ownerUsername, t, model.NAMESPACE_NORMAL_PUBLIC, "")
	if err != nil {
		// NOTE: this is here since the error value cannot be tested with
		// errors.Is w/ any error value defined in sqlite3 - you can but
		// the result wouldn't be right.
		// that's golang for you, a language without a proper, sane way
		// of dealing with errors.
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, db.ErrEntityAlreadyExists
		}
		return nil, err
	}
	nsPath := path.Join(dbif.config.GitRoot, name)
	if !db.IsSubDir(dbif.config.GitRoot, nsPath) {
		return nil, db.ErrInvalidLocation
	}
	err = os.RemoveAll(nsPath)
	if err != nil { return nil, err }
	err = os.Mkdir(nsPath, os.ModeDir|0755)
	if err != nil { return nil, err }
	err = tx.Commit()
	if err != nil { return nil, err }
	return &model.Namespace{
		Name: name,
		Title: name,
		Description: "",
		Email: "",
		Owner: ownerUsername,
		RegisterTime: t,
		Status: model.NAMESPACE_NORMAL_PUBLIC,
		ACL: nil,
		RepositoryList: nil,
		LocalPath: nsPath,
	}, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllVisibleNamespace(username string) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	var rs *sql.Rows
	var err error
	if len(username) > 0 {
		stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %s_namespace
WHERE (ns_status = 1 OR ns_status = 3) OR (ns_owner = ? OR ns_acl LIKE ? ESCAPE ?)
`, pfx))
		if err != nil { return nil, err }
		rs, err = stmt1.Query(username, db.ToSqlSearchPattern(username), "\\")
		if err != nil { return nil, err }
	} else {
		stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %s_namespace
WHERE ns_status = 1
`, pfx,))
		if err != nil { return nil, err }
		rs, err = stmt1.Query(username, db.ToSqlSearchPattern(username), "\\")
		if err != nil { return nil, err }
	}
	defer rs.Close()
	res := make(map[string]*model.Namespace, 0)
	for rs.Next() {
		var name, title, desc, email, owner, acl string
		var regtime int64
		var status int64
		err = rs.Scan(&name, &title, &desc, &email, &owner, &regtime, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: regtime,
			ACL: a,
			Status: model.GitusNamespaceStatus(status),
		}
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllNamespace() (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %s_namespace
`, pfx))
	if err != nil { return nil, err }
	rs, err := stmt.Query()
	if err != nil { return nil, err }
	defer rs.Close()
	res := make(map[string]*model.Namespace, 0)
	for rs.Next() {
		var name, title, desc, email, owner, acl string
		var regtime int64
		var status int64
		err = rs.Scan(&name, &title, &desc, &email, &owner, &regtime, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: regtime,
			ACL: a,
			Status: model.GitusNamespaceStatus(status),
		}
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllNamespaceByOwner(name string) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %s_namespace
WHERE ns_status != 3 AND ns_status != 4 AND ns_owner = ?
`, pfx))
	if err != nil { return nil, err }
	rs, err := stmt.Query(name)
	if err != nil { return nil, err }
	defer rs.Close()
	res := make(map[string]*model.Namespace, 0)
	for rs.Next() {
		var name, title, desc, email, owner, acl string
		var regtime int64
		var status int64
		err = rs.Scan(&name, &title, &desc, &email, &owner, &regtime, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: regtime,
			ACL: a,
			Status: model.GitusNamespaceStatus(status),
		}
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllVisibleRepositoryFromNamespace(username string, ns string) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	var rs *sql.Rows
	var err error
	if len(username) > 0 {
		stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook, rowid
FROM %s_repository
WHERE repo_namespace = ?
AND ((repo_status = 1 OR repo_status = 4 OR repo_status = 5) OR (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?))
`, pfx))
		if err != nil { return nil, err }
		rs, err = stmt.Query(ns, username, db.ToSqlSearchPattern(username), "\\")
		if err != nil { return nil, err }
	} else {
		stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook, rowid
FROM %s_repository
WHERE repo_namespace = ?
AND (repo_status = 1 OR repo_status = 4)
`, pfx))
		if err != nil { return nil, err }
		rs, err = stmt.Query(ns)
		if err != nil { return nil, err }
	}
	defer rs.Close()
	res := make([]*model.Repository, 0)
	var name, desc, owner, acl, forkOriginName, forkOriginNs, labelList, webhookstr string
	var status, rowid int64
	for rs.Next() {
		err = rs.Scan(&name, &desc, &owner, &acl, &status, &forkOriginNs, &forkOriginName, &labelList, &webhookstr, &rowid)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, name)
		var tags []string = nil
		if len(labelList) > 0 {
			tags = strings.Split(labelList[1:len(labelList)-1], "}{")
		}
		webhookobj, err := model.ParseWebHookConfig(webhookstr)
		if err != nil { return nil, err }
		res = append(res, &model.Repository{
			AbsId: rowid,
			Namespace: ns,
			Name: name,
			Owner: owner,
			Description: desc,
			AccessControlList: a,
			Status: model.GitusRepositoryStatus(status),
			Repository: gitlib.NewLocalGitRepository(p),
			ForkOriginNamespace: forkOriginNs,
			ForkOriginName: forkOriginName,
			RepoLabelList: tags,
			WebHookConfig: webhookobj,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllRepositoryFromNamespace(ns string) (map[string]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook, rowid
FROM %s_repository
WHERE repo_namespace = ?
`, pfx))
	if err != nil { return nil, err }
	rs, err := stmt.Query(ns)
	if err != nil { return nil, err }
	defer rs.Close()
	res := make(map[string]*model.Repository, 0)
	for rs.Next() {
		var name, desc, acl, owner, forkOriginNs, forkOriginName, labelList, webhookstr string
		var status, rowid int64
		var rtype uint8
		err = rs.Scan(&rtype, &name, &desc, &owner, &acl, &status, &labelList, &webhookstr, &rowid)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, name)
		var tags []string = nil
		if len(labelList) > 0 {
			tags = strings.Split(labelList[1:len(labelList)-1], "}{")
		}
		webhookobj, err := model.ParseWebHookConfig(webhookstr)
		if err != nil { return nil, err }
		res[name] = &model.Repository{
			AbsId: rowid,
			Type: rtype,
			Namespace: ns,
			Name: name,
			Owner: owner,
			Description: desc,
			AccessControlList: a,
			Status: model.GitusRepositoryStatus(status),
			Repository: gitlib.NewLocalGitRepository(p),
			ForkOriginNamespace: forkOriginNs,
			ForkOriginName: forkOriginName,
			RepoLabelList: tags,
			WebHookConfig: webhookobj,
		}
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) UpdateNamespaceInfo(name string, nsobj *model.Namespace) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_namespace
SET ns_title = ?, ns_description = ?, ns_email = ?, ns_owner = ?, ns_status = ?
WHERE ns_name = ?
`, pfx))
	if err != nil { tx.Rollback(); return err }
	_, err = stmt.Exec(nsobj.Title, nsobj.Description, nsobj.Email, nsobj.Owner, nsobj.Status, name)
	if err != nil { tx.Rollback(); return err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) UpdateNamespaceOwner(name string, newOwner string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_namespace
SET ns_owner = ?
WHERE ns_name = ?
`, pfx))
	if err != nil { tx.Rollback(); return err }
	_, err = stmt.Exec(newOwner, name)
	if err != nil { tx.Rollback(); return err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) UpdateNamespaceStatus(name string, newStatus model.GitusNamespaceStatus) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_namespace
SET ns_status = ?
WHERE ns_name = ?
`, pfx))
	if err != nil { tx.Rollback(); return err }
	_, err = stmt.Exec(newStatus, name)
	if err != nil { tx.Rollback(); return err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) HardDeleteNamespaceByName(name string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %s_namespace WHERE ns_name = ?
`, pfx))
	if err != nil { tx.Rollback(); return err }
	_, err = stmt.Exec(name)
	if err != nil { tx.Rollback(); return err }
	nsPath := path.Join(dbif.config.GitRoot, name)
	err = os.RemoveAll(nsPath)
	if err != nil { tx.Rollback(); return err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) CreateRepository(ns string, name string, repoType uint8, owner string) (*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	if !model.ValidNamespaceName(ns) || !model.ValidRepositoryName(name) {
		return nil, db.ErrInvalidLocation
	}
	fullName := ns + ":" + name
	tx, err := dbif.connection.Begin()
	if err != nil { return nil, err }
	defer tx.Rollback()
	webhookobj := new(model.WebHookConfig)
	stmt1, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %s_repository(repo_type, repo_fullname, repo_namespace, repo_name, repo_description, repo_acl, repo_status, repo_owner, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
`, pfx))
	if err != nil { return nil, err }
	_, err = stmt1.Exec(repoType, fullName, ns, name, new(string), new(string), model.REPO_NORMAL_PUBLIC, owner, new(string), new(string), new(string), webhookobj.String())
	if err != nil { return nil, err }
	p := path.Join(dbif.config.GitRoot, ns, name)
	if !db.IsSubDir(dbif.config.GitRoot, p) {
		return nil, db.ErrInvalidLocation
	}
	err = os.RemoveAll(p)
	if err != nil { return nil, err }
	if err = os.MkdirAll(p, os.ModeDir|0755); err != nil {
		return nil, err
	}
	lr, err := model.CreateLocalRepository(repoType, ns, name, p)
	if err != nil { return nil, err }
	err = model.InitLocalRepository(lr)
	if err != nil { return nil, err }
	if err = tx.Commit(); err != nil { return nil, err }
	r, err := model.NewRepository(ns, name, lr)
	r.Type = repoType
	r.Owner = owner
	r.RepoLabelList = nil
	r.WebHookConfig = webhookobj
	if err != nil { return nil, err }
	return r, nil
}

func (dbif *SqliteGitusDatabaseInterface) SetUpCloneRepository(originNs string, originName string, targetNs string, targetName string, owner string) (*model.Repository, error) {
	// TODO: fix this for multi vcs support
	pfx := dbif.config.Database.TablePrefix
	targetFullName := targetNs + ":" + targetName
	tx, err := dbif.connection.Begin()
	if err != nil { return nil, err }
	defer tx.Rollback()
	webhookobj := new(model.WebHookConfig)
	stmt1, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %s_repository(repo_type, repo_fullname, repo_namespace, repo_name, repo_description, repo_acl, repo_status, repo_owner, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
`, pfx))
	if err != nil { return nil, err }
	_, err = stmt1.Exec(model.REPO_TYPE_GIT, targetFullName, targetNs, targetName, new(string), new(string), model.REPO_NORMAL_PUBLIC, owner, originNs, originName, new(string), webhookobj.String())
	if err != nil {
		// TODO: find a better way to do this...
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, db.ErrEntityAlreadyExists
		} else {
			return nil, err
		}
	}
	originP := path.Join(dbif.config.GitRoot, originNs, originName)
	targetP := path.Join(dbif.config.GitRoot, targetNs, targetName)
	if !db.IsSubDir(dbif.config.GitRoot, targetP) {
		return nil, db.ErrInvalidLocation
	}
	err = os.RemoveAll(targetP)
	if err != nil { return nil, err }
	originRepo, err := model.CreateLocalRepository(model.REPO_TYPE_GIT, originNs, originName, originP)
	if err != nil { return nil, err }
	targetRepo, err := model.CreateLocalForkOf(originRepo, targetNs, targetName, targetP)
	if err != nil { return nil, err }
	if err = tx.Commit(); err != nil { return nil, err }
	r, err := model.NewRepository(targetNs, targetName, targetRepo)
	r.Type = model.GetGitusType(targetRepo)
	r.Owner = owner
	r.RepoLabelList = nil
	r.WebHookConfig = webhookobj
	if err != nil { return nil, err }
	return r, nil
}

func (dbif *SqliteGitusDatabaseInterface) UpdateRepositoryInfo(ns string, name string, robj *model.Repository) error {
	// TODO: these two queries can probaby be combined into one. fix this later.
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, repo_type FROM %s_repository WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return err }
	v := stmt1.QueryRow(ns, name)
	if v.Err() != nil { return v.Err() }
	var rowid string
	var repoType uint8
	err = v.Scan(&rowid, &repoType)
	if err != nil { return err }
	if len(rowid) <= 0 { return db.ErrEntityNotFound }
	tx, err := dbif.connection.Begin()
	if err != nil { tx.Rollback(); return err }
	defer tx.Rollback()
	stmt2, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_repository
SET repo_description = ?, repo_owner = ?, repo_status = ?, repo_webhook = ?
WHERE rowid = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt2.Exec(robj.Description, robj.Owner, robj.Status, rowid, robj.WebHookConfig.String())
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	switch repoType {
	case model.REPO_TYPE_GIT:
		lgr := robj.Repository.(*gitlib.LocalGitRepository)
		lgr.Description = robj.Description
		// we don't deal with error here because it's not critical.
		lgr.SyncLocalDescription()
	}
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) UpdateRepositoryStatus(ns string, name string, newStatus model.GitusRepositoryStatus) error {
	// TODO: these two queries can probaby be combined into one. fix this later.
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid FROM %s_repository WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return err }
	v := stmt1.QueryRow(ns, name)
	if v.Err() != nil { return v.Err() }
	var rowid string
	err = v.Scan(&rowid)
	if err != nil { return err }
	if len(rowid) <= 0 { return db.ErrEntityNotFound }
	tx, err := dbif.connection.Begin()
	if err != nil { tx.Rollback(); return err }
	stmt2, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_repository
SET repo_status = ?
WHERE rowid = ?
`, pfx))
	if err != nil { tx.Rollback(); return err }
	_, err = stmt2.Exec(newStatus, rowid)
	if err != nil { tx.Rollback(); return err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) HardDeleteRepository(ns string, name string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %s_repository
WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { tx.Rollback(); return err }
	_, err = stmt.Exec(ns, name)
	if err != nil { tx.Rollback(); return err }
	p := path.Join(dbif.config.GitRoot, ns, name)
	err = os.RemoveAll(p)
	if err != nil { tx.Rollback(); return err }
	err = tx.Commit()
	if err != nil { tx.Rollback(); return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllUsers(pageNum int64, pageSize int64) ([]*model.GitusUser, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT user_name, user_title, user_email, user_bio, user_website, user_status, user_password_hash
FROM %s_user
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r, err := stmt.Query(pageSize, pageNum * pageSize)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]*model.GitusUser, 0)
	var username, title, email, bio, website, ph string
	var status int
	for r.Next() {
		err = r.Scan(&username, &title, &email, &bio, &website, &status, &ph)
		if err != nil { return nil, err }
		res = append(res, &model.GitusUser{
			Name: username,
			Title: title,
			Email: email,
			Bio: bio,
			Website: website,
			Status: model.GitusUserStatus(status),
			PasswordHash: ph,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllNamespaces(pageNum int64, pageSize int64) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %s_namespace
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r, err := stmt.Query(pageSize, pageNum * pageSize)
	if err != nil { return nil, err }
	defer r.Close()
	res := make(map[string]*model.Namespace, 0)
	var name, title, desc, email, owner, acl string
	var reg_date int64
	var status int
	for r.Next() {
		err = r.Scan(&name, &title, &desc, &email, &owner, &reg_date, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: reg_date,
			ACL: a,
			Status: model.GitusNamespaceStatus(status),
		}
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllRepositories(pageNum int64, pageSize int64) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_acl, repo_owner, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook
FROM %s_repository
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r, err := stmt.Query(pageSize, pageNum * pageSize)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]*model.Repository, 0)
	var ns, name, desc, acl, owner, forkOriginName, forkOriginNs, labelList, webhookstr string
	var status int
	var repoType uint8
	for r.Next() {
		err = r.Scan(&repoType, &ns, &name, &desc, &acl, &owner, &status, &forkOriginNs, &forkOriginName, &labelList, &webhookstr)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, name)
		lr, err := model.CreateLocalRepository(repoType, ns, name, p)
		if err != nil { return nil, err }
		var tags []string = nil
		if len(labelList) > 0 {
			tags = strings.Split(labelList[1:len(labelList)-1], "}{")
		}
		webhookobj, err := model.ParseWebHookConfig(webhookstr)
		if err != nil { return nil, err }
		res = append(res, &model.Repository{
			Type: repoType,
			Namespace: ns,
			Name: name,
			Owner: owner,
			Description: desc,
			AccessControlList: a,
			Status: model.GitusRepositoryStatus(status),
			Repository: lr,
			ForkOriginNamespace: forkOriginNs,
			ForkOriginName: forkOriginName,
			RepoLabelList: tags,
			WebHookConfig: webhookobj,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) CountAllUser() (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(
		fmt.Sprintf(`SELECT COUNT(*) FROM %s_user`, pfx),
	)
	if err != nil { return 0, err }
	defer stmt.Close()
	r := stmt.QueryRow()
	if r.Err() != nil { return 0, r.Err() }
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, r.Err() }
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) CountAllNamespace() (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(
		fmt.Sprintf(`SELECT COUNT(*) FROM %s_namespace`, pfx),
	)
	if err != nil { return 0, err }
	defer stmt.Close()
	r := stmt.QueryRow()
	if r.Err() != nil { return 0, r.Err() }
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, r.Err() }
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) CountAllRepositoriesSearchResult(q string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	searchPattern := strings.ReplaceAll(q, "\\", "\\\\")
	searchPattern = strings.ReplaceAll(searchPattern, "%", "\\%")
	searchPattern = strings.ReplaceAll(searchPattern, "_", "\\_")
	searchPattern = "%" + searchPattern + "%"
	stmt, err := dbif.connection.Prepare(
		fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
WHERE (repo_name LIKE ? ESCAPE ? OR repo_namespace LIKE ? ESCAPE ?)`, pfx),
	)
	if err != nil { return 0, err }
	defer stmt.Close()
	r := stmt.QueryRow(searchPattern, "\\", searchPattern, "\\")
	if r.Err() != nil { return 0, r.Err() }
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, r.Err() }
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) CountAllRepositories() (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(
		fmt.Sprintf(`SELECT COUNT(*) FROM %s_repository`, pfx),
	)
	if err != nil { return 0, err }
	defer stmt.Close()
	r := stmt.QueryRow()
	if r.Err() != nil { return 0, r.Err() }
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, r.Err() }
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) SearchForUser(k string, pageNum int64, pageSize int64) ([]*model.GitusUser, error) {
	pfx := dbif.config.Database.TablePrefix
	pattern := db.ToSqlSearchPattern(k)
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT user_name, user_title, user_email, user_bio, user_website, user_status, user_password_hash
FROM %s_user
WHERE user_name LIKE ? ESCAPE ? OR user_title LIKE ? ESCAPE ?
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r, err := stmt.Query(pattern, "\\", pattern, "\\", pageSize, pageNum * pageSize)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]*model.GitusUser, 0)
	var username, title, email, bio, website, ph string
	var status int
	for r.Next() {
		err = r.Scan(&username, &title, &email, &bio, &website, &status, &ph)
		if err != nil { return nil, err }
		res = append(res, &model.GitusUser{
			Name: username,
			Title: title,
			Email: email,
			Bio: bio,
			Website: website,
			Status: model.GitusUserStatus(status),
			PasswordHash: ph,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) SearchForNamespace(k string, pageNum int64, pageSize int64) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	pattern := strings.ReplaceAll(k, "\\", "\\\\")
	pattern = strings.ReplaceAll(pattern, "%", "\\%")
	pattern = strings.ReplaceAll(pattern, "_", "\\_")
	pattern = "%" + pattern + "%"
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %s_namespace
WHERE ns_name LIKE ? ESCAPE ? OR ns_title LIKE ? ESCAPE ?
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r, err := stmt.Query(pattern, "\\", pattern, "\\", pageSize, pageNum * pageSize)
	if err != nil { return nil, err }
	defer r.Close()
	res := make(map[string]*model.Namespace, 0)
	var name, title, desc, email, owner, acl string
	var reg_date int64
	var status int
	for r.Next() {
		err = r.Scan(&name, &title, &desc, &email, &owner, &reg_date, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: reg_date,
			ACL: a,
			Status: model.GitusNamespaceStatus(status),
		}
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) SearchForRepository(k string, pageNum int64, pageSize int64) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	pattern := strings.ReplaceAll(k, "\\", "\\\\")
	pattern = strings.ReplaceAll(pattern, "%", "\\%")
	pattern = strings.ReplaceAll(pattern, "_", "\\_")
	pattern = "%" + pattern + "%"
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_acl, repo_owner, repo_status, repo_fork_origin_name, repo_fork_origin_namespace, repo_label_list, repo_webhook, rowid
FROM %s_repository
WHERE (repo_namespace LIKE ? ESCAPE ? OR repo_name LIKE ? ESCAPE ?)
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r, err := stmt.Query(pattern, "\\", pattern, "\\", pageSize, pageNum * pageSize)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]*model.Repository, 0)
	var ns, name, desc, acl, owner, forkOriginName, forkOriginNs, labelList, webhookstr string
	var status int
	var rowid int64
	var repoType uint8
	for r.Next() {
		err = r.Scan(&repoType, &ns, &name, &desc, &acl, &owner, &status, &forkOriginName, &forkOriginNs, &labelList, &webhookstr, &rowid)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, name)
		lr, err := model.CreateLocalRepository(repoType, ns, name, p)
		if err != nil { return nil, err }
		var tags []string = nil
		if len(labelList) > 0 {
			tags = strings.Split(labelList[1:len(labelList)-1], "}{")
		}
		webhookobj, err := model.ParseWebHookConfig(webhookstr)
		if err != nil { return nil, err }
		res = append(res, &model.Repository{
			AbsId: rowid,
			Type: repoType,
			Namespace: ns,
			Name: name,
			Description: desc,
			AccessControlList: a,
			Status: model.GitusRepositoryStatus(status),
			Repository: lr,
			ForkOriginNamespace: forkOriginNs,
			ForkOriginName: forkOriginName,
			RepoLabelList: tags,
			WebHookConfig: webhookobj,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) SetNamespaceACL(nsName string, targetUserName string, aclt *model.ACLTuple) error {
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_acl FROM %s_namespace WHERE ns_name = ?
`, pfx))
	if err != nil { return err }
	defer stmt1.Close()
	r := stmt1.QueryRow(nsName)
	if r.Err() != nil { stmt1.Close(); return r.Err() }
	var aclStr string
	err = r.Scan(&aclStr)
	if err != nil { stmt1.Close(); return err }
	acl, err := model.ParseACL(aclStr)
	if err != nil { return err }
	if acl == nil { acl = model.NewACL() }
	if aclt != nil {
		acl.ACL[targetUserName] = aclt
	} else {
		_, ok := acl.ACL[targetUserName]
		if ok { delete(acl.ACL, targetUserName) }
	}
	aclStr, err = acl.SerializeACL()
	if err != nil { return err }
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt2, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_namespace SET ns_acl = ? WHERE ns_name = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt2.Exec(aclStr, nsName)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) SetRepositoryACL(nsName string, repoName string, targetUserName string, aclt *model.ACLTuple) error {
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_acl FROM %s_repository WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return err }
	defer stmt1.Close()
	r := stmt1.QueryRow(nsName, repoName)
	if r.Err() != nil { stmt1.Close(); return r.Err() }
	var aclStr string
	err = r.Scan(&aclStr)
	if err != nil { stmt1.Close(); return err }
	acl, err := model.ParseACL(aclStr)
	if err != nil { return err }
	if acl == nil { acl = model.NewACL() }
	if aclt != nil {
		acl.ACL[targetUserName] = aclt
	} else {
		_, ok := acl.ACL[targetUserName]
		if ok { delete(acl.ACL, targetUserName) }
	}
	aclStr, err = acl.SerializeACL()
	if err != nil { return err }
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt2, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_repository SET repo_acl = ? WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt2.Exec(aclStr, nsName, repoName)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllComprisingNamespace(username string) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	pattern := strings.ReplaceAll(username, "\\", "\\\\")
	pattern = strings.ReplaceAll(pattern, "%", "\\%")
	pattern = strings.ReplaceAll(pattern, "_", "\\_")
	pattern = "%" + pattern + "%"
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %s_namespace
WHERE ns_owner = ? OR ns_acl LIKE ? ESCAPE ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt1.Close()
	r, err := stmt1.Query(username, pattern, "\\")
	if err != nil { return nil, err }
	defer r.Close()
	res := make(map[string]*model.Namespace, 0)
	for r.Next() {
		var name, title, desc, email, owner, acl string
		var regtime int64
		var status int64
		err = r.Scan(&name, &title, &desc, &email, &owner, &regtime, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: regtime,
			ACL: a,
			Status: model.GitusNamespaceStatus(status),
		}
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllVisibleNamespacePaginated(username string, pageNum int64, pageSize int64) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	privateSelectClause := ""
	if len(username) > 0 {
		privateSelectClause = "OR (ns_owner = ? OR ns_acl LIKE ? ESCAPE ?)"
	}
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %s_namespace
WHERE ns_status = 1 %s
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx, privateSelectClause))
	if err != nil { return nil, err }
	defer stmt1.Close()
	var rs *sql.Rows
	if len(username) > 0 {
		pattern := strings.ReplaceAll(username, "\\", "\\\\")
		pattern = strings.ReplaceAll(pattern, "%", "\\%")
		pattern = strings.ReplaceAll(pattern, "_", "\\_")
		pattern = "%" + pattern + "%"
		rs, err = stmt1.Query(username, pattern, "\\", pageSize, pageNum*pageSize)
	} else {
		rs, err = stmt1.Query(pageSize, pageNum*pageSize)
	}
	if err != nil { return nil, err }
	defer rs.Close()
	res := make(map[string]*model.Namespace, 0)
	for rs.Next() {
		var name, title, desc, email, owner, acl string
		var regtime int64
		var status int64
		err = rs.Scan(&name, &title, &desc, &email, &owner, &regtime, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: regtime,
			ACL: a,
			Status: model.GitusNamespaceStatus(status),
		}
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllVisibleRepositoryPaginated(username string, pageNum int64, pageSize int64) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	var rs *sql.Rows
	var stmt *sql.Stmt
	var err error
	if len(username) > 0 {
		upat := db.ToSqlSearchPattern(username)
		stmt, err = dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook, rowid
FROM %s_repository repo
FULL JOIN (SELECT ns_name, ns_status FROM %s_namespace WHERE ns_status = 1 OR ns_status = 3 OR (ns_owner = ? OR ns_acl LIKE ? ESCAPE ?)) ns
ON repo.repo_namespace = ns.ns_name
WHERE ((ns_status = 1 OR ns_status = 3) AND ns.ns_name IS NOT NULL)
OR (repo_status = 1 OR repo_status = 4 OR repo_status = 5)
OR (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx, pfx))
		if err != nil { return nil, err }
		rs, err = stmt.Query(username, upat, "\\", username, upat, "\\", pageSize, pageNum*pageSize)
		if err != nil { return nil, err }
	} else {
		stmt, err = dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook, rowid
FROM %s_repository repo
FULL JOIN (SELECT ns_name, ns_status FROM %s_namespace WHERE ns_status = 1 OR ns_status = 3) ns
ON repo.repo_namespace = ns.ns_name
WHERE ((ns_status = 1 OR ns_status = 3) AND ns.ns_name IS NOT NULL)
OR (repo_status = 1 OR repo_status = 4)
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx, pfx))
		if err != nil { return nil, err }
		rs, err = stmt.Query(pageSize, pageNum*pageSize)
		if err != nil { return nil, err }
	}
	if err != nil { return nil, err }
	defer rs.Close()
	res := make([]*model.Repository, 0)
	var ns, name, desc, owner, acl, forkOriginNs, forkOriginName, labelList, webhookstr string
	var status, rowid int64
	var repoType uint8
	for rs.Next() {
		err = rs.Scan(&repoType, &ns, &name, &desc, &owner, &acl, &status, &forkOriginNs, &forkOriginName, &labelList, &webhookstr, &rowid)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, name)
		lr, err := model.CreateLocalRepository(repoType, ns, name, p)
		if err != nil { return nil, err }
		var tags []string = nil
		if len(labelList) > 0 {
			tags = strings.Split(labelList[1:len(labelList)-1], "}{")
		}
		webhookobj, err := model.ParseWebHookConfig(webhookstr)
		if err != nil { return nil, err }
		res = append(res, &model.Repository{
			AbsId: rowid,
			Type: repoType,
			Namespace: ns,
			Name: name,
			Owner: owner,
			Description: desc,
			AccessControlList: a,
			Status: model.GitusRepositoryStatus(status),
			Repository: lr,
			ForkOriginNamespace: forkOriginNs,
			ForkOriginName: forkOriginName,
			RepoLabelList: tags,
			WebHookConfig: webhookobj,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) CountAllVisibleNamespace(username string) (int64, error) {
	return dbif.CountAllVisibleNamespaceSearchResult(username, "")
}

func (dbif *SqliteGitusDatabaseInterface) CountAllVisibleRepositories(username string) (int64, error) {
	return dbif.CountAllVisibleRepositoriesSearchResult(username, "")
}

func (dbif *SqliteGitusDatabaseInterface) SearchAllVisibleNamespacePaginated(username string, query string, pageNum int64, pageSize int64) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	var rs *sql.Rows
	var stmt *sql.Stmt
	var err error
	if len(query) > 0 {
		qpattern := db.ToSqlSearchPattern(query)
		if len(username) > 0 {
			stmt, err = dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %s_namespace
WHERE (ns_name LIKE ? ESCAPE ? OR ns_title LIKE ? ESCAPE ?)
AND (ns_status = 1 OR ns_status = 3 OR ns_owner = ? OR ns_acl LIKE ? ESCAPE ?)
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx))
			if err != nil { return nil, err }
			rs, err = stmt.Query(qpattern, "\\", qpattern, "\\", username, db.ToSqlSearchPattern(username), "\\", pageSize, pageNum*pageSize)
			if err != nil { return nil, err }
		} else {
			stmt, err = dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %s_namespace
WHERE (ns_name LIKE ? ESCAPE ? OR ns_title LIKE ? ESCAPE ?)
AND (ns_status = 1 OR ns_status = 3)
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx))
			if err != nil { return nil, err }
			rs, err = stmt.Query(qpattern, "\\", qpattern, "\\", pageSize, pageNum*pageSize)
			if err != nil { return nil, err }
		}
	} else {
		if len(username) > 0 {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %s_namespace
WHERE (ns_status = 1 OR ns_status = 3 OR ns_owner = ? OR ns_acl LIKE ? ESCAPE ?)
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx))
			if err != nil { return nil, err }
			rs, err = stmt.Query(username, db.ToSqlSearchPattern(username), "\\", pageSize, pageNum*pageSize)
			if err != nil { return nil, err }
		} else {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %s_namespace
WHERE (ns_status = 1 OR ns_status = 3)
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx))
			if err != nil { return nil, err }
			rs, err = stmt.Query(pageSize, pageNum*pageSize)
			if err != nil { return nil, err }
		}
	}
	if err != nil { return nil, err }
	defer rs.Close()
	res := make(map[string]*model.Namespace, 0)
	var name, title, desc, email, owner, acl string
	var reg_date int64
	var status int
	for rs.Next() {
		err = rs.Scan(&name, &title, &desc, &email, &owner, &reg_date, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: reg_date,
			ACL: a,
			Status: model.GitusNamespaceStatus(status),
		}
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) SearchAllVisibleRepositoryPaginated(username string, query string, pageNum int64, pageSize int64) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	var r *sql.Rows
	var err error
	if len(query) > 0 {
		qpattern := db.ToSqlSearchPattern(query)
		if len(username) > 0 {
			upattern := db.ToSqlSearchPattern(username)
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_webhook, rowid
FROM %s_repository repo
FULL JOIN (
    SELECT ns_name, ns_status FROM %s_namespace WHERE ns_status = 1 OR ns_status = 3 OR ns_owner = ? OR ns_acl LIKE ? ESCAPE ?
) ns ON repo.repo_namespace = ns.ns_name
WHERE (
    ((ns.ns_status = 1 OR ns.ns_status = 3) AND ns.ns_name IS NOT NULL)
    OR repo.repo_status = 1 OR repo_status = 4 OR repo_status = 5
    OR repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
AND (repo_name LIKE ? ESCAPE ? OR repo_namespace LIKE ? ESCAPE ?)
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx, pfx))
			if err != nil { return nil, err }
			r, err = stmt.Query(username, upattern, "\\", username, upattern, "\\", qpattern, "\\", qpattern, "\\", pageSize, pageNum*pageSize)
			if err != nil { return nil, err }
		} else {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_webhook, rowid
FROM %s_repository repo
FULL JOIN (
    SELECT ns_name, ns_status FROM %s_namespace WHERE ns_status = 1 OR ns_status = 3
) ns ON repo.repo_namespace = ns.ns_name
WHERE (
    ((ns.ns_status = 1 OR ns.ns_status = 3) AND ns.ns_name IS NOT NULL)
    OR repo.repo_status = 1 OR repo_status = 4)
AND (repo_name LIKE ? ESCAPE ? OR repo_namespace LIKE ? ESCAPE ?)
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx, pfx))
			if err != nil { return nil, err }
			r, err = stmt.Query(qpattern, "\\", qpattern, "\\", pageSize, pageNum*pageSize)
			if err != nil { return nil, err }
		}
	} else {
		if len(username) > 0 {
			upattern := db.ToSqlSearchPattern(username)
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_webhook, rowid
FROM %s_repository repo
FULL JOIN (
    SELECT ns_name, ns_status FROM %s_namespace WHERE ns_status = 1 OR ns_status = 3 OR ns_owner = ? OR ns_acl LIKE ? ESCAPE ?
) ns ON repo.repo_namespace = ns.ns_name
WHERE (
    ((ns.ns_status = 1 OR ns.ns_status = 3) AND ns.ns_name IS NOT NULL)
    OR repo.repo_status = 1 OR repo_status = 4
    OR repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx, pfx))
			if err != nil { return nil, err }
			r, err = stmt.Query(username, upattern, "\\", username, upattern, "\\", pageSize, pageNum*pageSize)
			if err != nil { return nil, err }
		} else {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_webhook, rowid
FROM %s_repository repo
FULL JOIN (
    SELECT ns_name, ns_status FROM %s_namespace WHERE ns_status = 1 OR ns_status = 3
) ns ON repo.repo_namespace = ns.ns_name
WHERE (
    ((ns.ns_status = 1 OR ns.ns_status = 3) AND ns.ns_name IS NOT NULL)
    OR repo.repo_status = 1 OR repo_status = 4)
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx, pfx))
			if err != nil { return nil, err }
			r, err = stmt.Query(pageSize, pageNum*pageSize)
			if err != nil { return nil, err }
		}
	}
	defer r.Close()
	res := make([]*model.Repository, 0)
	for r.Next() {
		var ns, name, desc, acl, forkOriginNamespace, forkOriginName, webhookstr string
		var status, rowid int64
		var repoType uint8
		err = r.Scan(&repoType, &ns, &name, &desc, &acl, &status, &forkOriginNamespace, &forkOriginName, &webhookstr, &rowid)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, name)
		lr, err := model.CreateLocalRepository(repoType, ns, name, p)
		if err != nil { return nil, err }
		webhookobj, err := model.ParseWebHookConfig(webhookstr)
		if err != nil { return nil, err }
		res = append(res, &model.Repository{
			AbsId: rowid,
			Type: repoType,
			Namespace: ns,
			Name: name,
			Description: desc,
			AccessControlList: a,
			Status: model.GitusRepositoryStatus(status),
			Repository: lr,
			ForkOriginNamespace: forkOriginNamespace,
			ForkOriginName: forkOriginName,
			WebHookConfig: webhookobj,
		})
	}
	return res, nil
}


func (dbif *SqliteGitusDatabaseInterface) CountAllVisibleNamespaceSearchResult(username string, pattern string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	var r *sql.Row
	var err error
	if len(pattern) > 0 {
		qpattern := db.ToSqlSearchPattern(pattern)
		if len(username) > 0 {
			upattern := db.ToSqlSearchPattern(username)
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_namespace
WHERE (ns_status = 1 OR ns_status = 3 OR ns_owner = ? OR ns_acl LIKE ? ESCAPE ?) AND (ns_name LIKE ? ESCAPE ? OR ns_title LIKE ? ESCAPE ?)
`, pfx))
			if err != nil { return 0, err }
			r = stmt.QueryRow(username, upattern, "\\", qpattern, "\\", qpattern, "\\")
		} else {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_namespace
WHERE (ns_status = 1 OR ns_status = 3)
AND (ns_name LIKE ? ESCAPE ? OR ns_title LIKE ? ESCAPE ?)
`, pfx))
			if err != nil { return 0, err }
			r = stmt.QueryRow(qpattern, "\\", qpattern, "\\")
		}
	} else {
		if len(username) > 0 {
			upattern := db.ToSqlSearchPattern(username)
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_namespace
WHERE (ns_status = 1 OR ns_status = 3 OR ns_owner = ? OR ns_acl LIKE ? ESCAPE ?)
`, pfx))
			if err != nil { return 0, err }
			r = stmt.QueryRow(username, upattern, "\\")
		} else {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_namespace WHERE (ns_status = 1 OR ns_status = 3)
`, pfx))
			if err != nil { return 0, err }
			r = stmt.QueryRow()
		}
	}
	if r.Err() != nil { return 0, r.Err() }
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, r.Err() }
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) CountAllVisibleRepositoriesSearchResult(username string, pattern string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	var r *sql.Row
	var err error
	if len(pattern) > 0 {
		qpattern := db.ToSqlSearchPattern(pattern)
		if len(username) > 0 {
			upattern := db.ToSqlSearchPattern(username)
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository repo
FULL JOIN (
    SELECT ns_name, ns_status FROM %s_namespace WHERE ns_status = 1 OR ns_status = 3 OR ns_owner = ? OR ns_acl LIKE ? ESCAPE ?
) ns ON repo.repo_namespace = ns.ns_name
WHERE (
    ((ns.ns_status = 1 OR ns.ns_status = 3) AND ns.ns_name IS NOT NULL)
    OR repo.repo_status = 1 OR repo_status = 4
    OR repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
AND (repo_name LIKE ? ESCAPE ? OR repo_namespace LIKE ? ESCAPE ?)
`, pfx, pfx))
			if err != nil { return 0, err }
			r = stmt.QueryRow(username, upattern, "\\", username, upattern, "\\", qpattern, "\\", qpattern, "\\")
		} else {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository repo
FULL JOIN (
    SELECT ns_name, ns_status FROM %s_namespace WHERE ns_status = 1 OR ns_status = 3
) ns ON repo.repo_namespace = ns.ns_name
WHERE (
    ((ns.ns_status = 1 OR ns.ns_status = 3) AND ns.ns_name IS NOT NULL)
    OR repo.repo_status = 1 OR repo_status = 4)
AND (repo_name LIKE ? ESCAPE ? OR repo_namespace LIKE ? ESCAPE ?)
`, pfx, pfx))
			if err != nil { return 0, err }
			r = stmt.QueryRow(qpattern, "\\", qpattern, "\\")
		}
	} else {
		if len(username) > 0 {
			upattern := db.ToSqlSearchPattern(username)
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository repo
FULL JOIN (
    SELECT ns_name, ns_status FROM %s_namespace WHERE ns_status = 1 OR ns_status = 3 OR ns_owner = ? OR ns_acl LIKE ? ESCAPE ?
) ns ON repo.repo_namespace = ns.ns_name
WHERE (
    ((ns.ns_status = 1 OR ns.ns_status = 3) AND ns.ns_name IS NOT NULL)
    OR repo.repo_status = 1 OR repo_status = 4
    OR repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
`, pfx, pfx))
			if err != nil { return 0, err }
			r = stmt.QueryRow(username, upattern, "\\", username, upattern, "\\")
		} else {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository repo
FULL JOIN (
    SELECT ns_name, ns_status FROM %s_namespace WHERE ns_status = 1 OR ns_status = 3
) ns ON repo.repo_namespace = ns.ns_name
WHERE (
    ((ns.ns_status = 1 OR ns.ns_status = 3) AND ns.ns_name IS NOT NULL)
    OR repo.repo_status = 1 OR repo_status = 4
)
`, pfx, pfx))
			if err != nil { return 0, err }
			r = stmt.QueryRow()
		}
	}
	if r.Err() != nil { return 0, r.Err() }
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, r.Err() }
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllRepositoryIssue(ns string, name string) ([]*model.Issue, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, issue_id, issue_author, issue_status, issue_title, issue_content, issue_timestamp
FROM %s_issue
WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r, err := stmt.Query(ns, name)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]*model.Issue, 0)
	for r.Next() {
		var issueAbsId, issueTimestamp int64
		var issueId, issueStatus int
		var issueAuthor, issueTitle, issueContent string
		err = r.Scan(&issueAbsId, &issueId, &issueAuthor, &issueStatus, &issueTitle, &issueContent, &issueTimestamp)
		if err != nil { return nil, err }
		res = append(res, &model.Issue{
			IssueAbsId: issueAbsId,
			RepoNamespace: ns,
			RepoName: name,
			IssueStatus: issueStatus,
			IssueId: issueId,
			IssueTime: issueTimestamp,
			IssueTitle: issueTitle,
			IssueAuthor: issueAuthor,
			IssueContent: issueContent,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetRepositoryIssue(ns string, name string, iid int) (*model.Issue, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, issue_timestamp, issue_author, issue_title, issue_content, issue_status, issue_priority
FROM %s_issue
WHERE repo_namespace = ? AND repo_name = ? AND issue_id = ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r := stmt.QueryRow(ns, name, iid)
	if r.Err() != nil { return nil, r.Err() }
	var absid, timestamp int64
	var status, priority int
	var author, title, content string
	err = r.Scan(&absid, &timestamp, &author, &title, &content, &status, &priority)
	if err != nil { return nil, err }
	return &model.Issue{
		IssueAbsId: absid,
		RepoNamespace: ns,
		RepoName: name,
		IssueId: iid,
		IssueAuthor: author,
		IssueTitle: title,
		IssueTime: timestamp,
		IssueContent: content,
		IssueStatus: status,
		IssuePriority: priority,
	}, nil
}

func (dbif *SqliteGitusDatabaseInterface) CountAllRepositoryIssue(ns string, name string) (int, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*)
FROM %s_issue
WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return 0, err }
	defer stmt.Close()
	r := stmt.QueryRow(ns, name)
	if r.Err() != nil { return 0, r.Err() }
	var res int
	err = r.Scan(&res)
	if err != nil { return 0, err }
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) NewRepositoryIssue(ns string, name string, author string, title string, content string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_issue WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return 0, err }
	defer stmt.Close()
	r := stmt.QueryRow(ns, name)
	if r.Err() != nil { return 0, err }
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, err }
	res += 1
	tx, err := dbif.connection.Begin()
	if err != nil { return 0, err }
	defer tx.Rollback()
	stmt2, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %s_issue(repo_namespace, repo_name, issue_id, issue_timestamp, issue_author, issue_title, issue_content, issue_status, issue_priority)
VALUES (?,?,?,?,?,?,?,?,?)
`, pfx))
	if err != nil { return 0, err }
	_, err = stmt2.Exec(ns, name, res, time.Now().Unix(), author, title, content, model.ISSUE_OPENED, 0)
	if err != nil { return 0, err }
	err = tx.Commit()
	if err != nil { return 0, err }
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) HardDeleteRepositoryIssue(ns string, name string, issueId int) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %s_issue WHERE repo_namespace = ? AND repo_name = ? AND issue_id = ?
`, pfx))
	if err != nil { return err }
	defer stmt.Close()
	_, err = stmt.Exec(ns, name, issueId)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) SetIssuePriority(namespace string, name string, id int64, priority int) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_issue SET issue_priority = ? WHERE repo_namespace = ? AND repo_name = ? AND issue_id = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(priority, namespace, name, id)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllIssueEvent(ns string, name string, issueId int) ([]*model.IssueEvent, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid FROM %s_issue WHERE repo_namespace = ? AND repo_name = ? AND issue_id = ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt1.Close()
	r := stmt1.QueryRow(ns, name, issueId)
	if r.Err() != nil { return nil, r.Err() }
	var absId int
	err = r.Scan(&absId)
	if err != nil { return nil, err }
	stmt2, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, issue_event_type, issue_event_time, issue_event_author, issue_event_content
FROM %s_issue_event
WHERE issue_abs_id = ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt2.Close()
	rs, err := stmt2.Query(absId)
	if err != nil { return nil, err }
	defer rs.Close()
	res := make([]*model.IssueEvent, 0)
	for rs.Next() {
		var author, content string
		var eventType int
		var eventAbsId, timestamp int64
		err = rs.Scan(&eventAbsId, &eventType, &timestamp, &author, &content)
		if err != nil { return nil, err }
		res = append(res, &model.IssueEvent{
			EventAbsId: eventAbsId,
			EventType: eventType,
			EventTimestamp: timestamp,
			EventAuthor: author,
			EventContent: content,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) NewRepositoryIssueEvent(ns string, name string, issueId int64, eType int, author string, content string) error {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, issue_status FROM %s_issue WHERE repo_namespace = ? AND repo_name = ? AND issue_id = ?
`, pfx))
	if err != nil { return err }
	defer stmt.Close()
	r := stmt.QueryRow(ns, name, issueId)
	if r.Err() != nil { return r.Err() }
	var issueAbsId int64
	var issueStatus int
	err = r.Scan(&issueAbsId, &issueStatus)
	if err != nil { return err }
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt2, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %s_issue_event(issue_abs_id, issue_event_type, issue_event_time, issue_event_author, issue_event_content) VALUES (?,?,?,?,?)
`, pfx))
	if err != nil { return err }
	defer stmt2.Close()
	_, err = stmt2.Exec(issueAbsId, eType, time.Now().Unix(), author, content)
	if err != nil { return err }
	newIssueStatus := issueStatus
	switch eType {
	case model.EVENT_CLOSED_AS_SOLVED:
		newIssueStatus = model.ISSUE_CLOSED_AS_SOLVED
	case model.EVENT_CLOSED_AS_DISCARDED:
		newIssueStatus = model.ISSUE_CLOSED_AS_DISCARDED
	case model.EVENT_REOPENED:
		newIssueStatus = model.ISSUE_OPENED
	}
	if newIssueStatus != issueStatus {
		stmt3, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_issue SET issue_status = ? WHERE rowid = ?
`, pfx))
		if err != nil { return err }
		defer stmt3.Close()
		_, err = stmt3.Exec(newIssueStatus, issueAbsId)
		if err != nil { return err }
	}
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) HardDeleteRepositoryIssueEvent(eventAbsId int64) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %s_issue_event WHERE rowid = ?
`, pfx))
	if err != nil { return err }
	defer stmt.Close()
	_, err = stmt.Exec(eventAbsId)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllBelongingNamespace(viewingUser string, userName string) ([]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	nsStatusClause := "ns_status = 1"
	if len(viewingUser) > 0 {
		if viewingUser == userName {
			nsStatusClause = "1"
		} else {
			nsStatusClause = "ns_acl LIKE ? ESCAPE ?"
		}
	}
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %s_namespace
WHERE (%s) AND (ns_owner = ? OR ns_acl LIKE ? ESCAPE ?)
`, pfx, nsStatusClause))
	if err != nil { return nil, err }
	defer stmt.Close()
	userSearchPattern := db.ToSqlSearchPattern(userName)
	var rs *sql.Rows
	if len(viewingUser) > 0 {
		if viewingUser == userName {
			rs, err = stmt.Query(userName, userSearchPattern, "\\")
		} else {
			viewingPattern := db.ToSqlSearchPattern(viewingUser)
			rs, err = stmt.Query(viewingPattern, "\\", userName, userSearchPattern, "\\")
		}
	} else {
		rs, err = stmt.Query(userName, userSearchPattern, "\\")
	}
	if err != nil { return nil, err }
	res := make([]*model.Namespace, 0)
	for rs.Next() {
		var name, title, desc, email, owner, acl string
		var regtime int64
		var status int64
		err = rs.Scan(&name, &title, &desc, &email, &owner, &regtime, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res = append(res, &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: regtime,
			ACL: a,
			Status: model.GitusNamespaceStatus(status),
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllBelongingRepository(viewingUser string, user string, query string, pageNum int64, pageSize int64) ([]*model.Repository, error) {
	// the fact that go does not have if-expr is killing me.
	// NOTE:
	// + if viewingUser is empty, it means the viewing user is a guest,
	//   which means we should only select public repositories of user.
	// + if viewingUser is non-empty and the same as user, we select
	//   all belonging repositories regardless of status.
	// + if viewingUser is non-empty but not the same as user, we select
	//   all belonging repositories of user but filter with viewingUser.
	pfx := dbif.config.Database.TablePrefix
	var stmt *sql.Stmt
	var err error
	var r *sql.Rows
	if len(viewingUser) <= 0 {
		if len(query) <= 0 {
			stmt, err = dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, rowid
FROM %s_repository
WHERE (repo_status = 1 OR repo_status = 4) AND (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
ORDER BY repo_name ASC, repo_namespace ASC
`, pfx))
		} else {
			stmt, err = dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, rowid
FROM %s_repository
WHERE (repo_status = 1 OR repo_status = 4) AND (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
AND (repo_name LIKE ? ESCAPE ? OR repo_namespace LIKE ? ESCAPE ?)
ORDER BY repo_name ASC, repo_namespace ASC
`, pfx))
		}
		if err != nil { return nil, err }
		if len(query) <= 0 {
			r, err = stmt.Query(user, db.ToSqlSearchPattern(user), "\\")
		} else {
			qpat := db.ToSqlSearchPattern(query)
			r, err = stmt.Query(user, db.ToSqlSearchPattern(user), "\\", qpat, "\\", qpat, "\\")
		}
	} else if viewingUser == user {
		if len(query) <= 0 {
			stmt, err = dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, rowid
FROM %s_repository
WHERE (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
ORDER BY repo_name ASC, repo_namespace ASC
`, pfx))
		} else {
			stmt, err = dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, rowid
FROM %s_repository
WHERE (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
AND (repo_name LIKE ? ESCAPE ? OR repo_namespace LIKE ? ESCAPE ?)
ORDER BY repo_name ASC, repo_namespace ASC
`, pfx))
		}
		if err != nil { return nil, err }
		if len(query) <= 0 {
			r, err = stmt.Query(user, db.ToSqlSearchPattern(user), "\\")
		} else {
			qpat := db.ToSqlSearchPattern(query)
			r, err = stmt.Query(user, db.ToSqlSearchPattern(user), "\\", qpat, "\\", qpat, "\\")
		}
	} else {
		if len(query) <= 0 {
			stmt, err = dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, rowid
FROM %s_repository
WHERE (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
AND (repo_status = 1 OR repo_status = 4 OR repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
ORDER BY repo_name ASC, repo_namespace ASC
`, pfx))
		} else {
			stmt, err = dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, rowid
FROM %s_repository
WHERE (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
AND (repo_status = 1 OR repo_status = 4 OR repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
AND (repo_name LIKE ? ESCAPE ? OR repo_namespace LIKE ? ESCAPE ?)
ORDER BY repo_name ASC, repo_namespace ASC
`, pfx))
		}
		if err != nil { return nil, err }
		if len(query) <= 0 {
			r, err = stmt.Query(user, db.ToSqlSearchPattern(user), "\\", user, db.ToSqlSearchPattern(user), "\\")
		} else {
			qpat := db.ToSqlSearchPattern(query)
			r, err = stmt.Query(user, db.ToSqlSearchPattern(user), "\\", user, db.ToSqlSearchPattern(user), "\\", qpat, "\\", qpat, "\\")
		}
	}
	defer r.Close()
	res := make([]*model.Repository, 0)
	for r.Next() {
		var ns, name, desc, acl, owner, forkOriginNamespace, forkOriginName, labelList string
		var status, rowid int64
		var repoType uint8
		err := r.Scan(&repoType, &ns, &name, &desc, &owner, &acl, &status, &forkOriginNamespace, &forkOriginName, &labelList, &rowid)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, name)
		lr, err := model.CreateLocalRepository(repoType, ns, name, p)
		if err != nil { return nil, err }
		var tags []string = nil
		if len(labelList) > 0 {
			tags = strings.Split(labelList[1:len(labelList)-1], "}{")
		}
		res = append(res, &model.Repository{
			AbsId: rowid,
			Type: repoType,
			Namespace: ns,
			Name: name,
			Description: desc,
			Owner: owner,
			AccessControlList: a,
			Status: model.GitusRepositoryStatus(status),
			Repository: lr,
			ForkOriginNamespace: forkOriginNamespace,
			ForkOriginName: forkOriginName,
			RepoLabelList: tags,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) CountAllBelongingRepository(viewingUser string, user string, query string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	var stmt *sql.Stmt
	var r *sql.Row
	var err error
	if len(viewingUser) <= 0 {
		if len(query) <= 0 {
			stmt, err = dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
WHERE (repo_status = 1 OR repo_status = 4) AND (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
`, pfx))
		} else {
			stmt, err = dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
WHERE (repo_status = 1 OR repo_status = 4) AND (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?) AND (repo_name LIKE ? ESCAPE ? OR repo_namespace LIKE ? ESCAPE ?)
`, pfx))
		}
		if err != nil { return 0, err }
		if len(query) <= 0 {
			r = stmt.QueryRow(user, db.ToSqlSearchPattern(user), "\\")
		} else {
			qpat := db.ToSqlSearchPattern(query)
			r = stmt.QueryRow(user, db.ToSqlSearchPattern(user), "\\", qpat, "\\", qpat, "\\")
		}
	} else if viewingUser == user {
		if len(query) <= 0 {
			stmt, err = dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
WHERE (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
`, pfx))
		} else {
			stmt, err = dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
WHERE (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
AND (repo_name LIKE ? ESCAPE ? OR repo_namespace LIKE ? ESCAPE ?)
`, pfx))
		}
		if err != nil { return 0, err }
		if len(query) <= 0 {
			r = stmt.QueryRow(user, db.ToSqlSearchPattern(user), "\\")
		} else {
			qpat := db.ToSqlSearchPattern(query)
			r = stmt.QueryRow(user, db.ToSqlSearchPattern(user), "\\", qpat, "\\", qpat, "\\")
		}
	} else {
		if len(query) <= 0 {
			stmt, err = dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
WHERE (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
AND (repo_status = 1 OR repo_status = 4 OR repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
`, pfx))
		} else {
			stmt, err = dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
WHERE (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
AND (repo_status = 1 OR repo_status = 4 OR repo_owner = ? OR repo_acl LIKE ? ESCAPE ?)
AND (repo_name LIKE ? ESCAPE ? OR repo_namespace LIKE ? ESCAPE ?)
`, pfx))
		}
		if err != nil { return 0, err }
		if len(query) <= 0 {
			r = stmt.QueryRow(user, db.ToSqlSearchPattern(user), "\\", user, db.ToSqlSearchPattern(user), "\\")
		} else {
			qpat := db.ToSqlSearchPattern(query)
			r = stmt.QueryRow(user, db.ToSqlSearchPattern(user), "\\", user, db.ToSqlSearchPattern(user), "\\", qpat, "\\", qpat, "\\")
		}
	}
	if r.Err() != nil { return 0, r.Err() }
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, err }
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetForkRepositoryOfUser(username string, originNamespace string, originName string) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_acl, repo_status, repo_label_list, rowid
FROM %s_repository
WHERE repo_owner = ? AND repo_fork_origin_namespace = ? AND repo_fork_origin_name = ?
`, pfx))
	if err != nil { return nil, err }
	r, err := stmt.Query(username, originNamespace, originName)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]*model.Repository, 0)
	for r.Next() {
		var ns, name, desc, acl, labelList string
		var status int
		var rowid int64
		var repoType uint8
		err = r.Scan(&repoType, &ns, &name, &desc, &acl, &status, &labelList, &rowid)
		if err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, err
		}
		p := path.Join(dbif.config.GitRoot, ns, name)
		lr, err := model.CreateLocalRepository(repoType, ns, name, p)
		if err != nil { return nil, err }
		var tags []string = nil
		if len(labelList) > 0 {
			tags = strings.Split(labelList[1:len(labelList)-1], "}{")
		}
		mr, err := model.NewRepository(ns, name, lr)
		mr.AbsId = rowid
		mr.Owner = username
		mr.Type = repoType
		mr.Status = model.GitusRepositoryStatus(status)
		mr.ForkOriginNamespace = originNamespace
		mr.ForkOriginName = originName
		mr.RepoLabelList = tags
		aclobj, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		mr.AccessControlList = aclobj
		res = append(res, mr)
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllPullRequestPaginated(namespace string, name string, pageNum int64, pageSize int64) ([]*model.PullRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, pull_request_id, username, title, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp
FROM %spull_request
WHERE receiver_namespace = ? AND receiver_name = ?
ORDER BY pull_request_id ASC LIMIT ? OFFSET ?
`, pfx))
	if err != nil { return nil, err }
	r, err := stmt.Query(namespace, name, pageSize, pageNum*pageSize)
	if err != nil { return nil, err }
	res := make([]*model.PullRequest, 0)
	for r.Next() {
		var prid, absid, prtime int64
		var status int
		var username, title, receiverBranch string
		var providerNamespace, providerName, provideBranch string
		var mergeCheckResultString string
		var mergeCheckTimestamp int64
		err = r.Scan(&absid, &prid, &username, &title, &receiverBranch, &providerNamespace, &providerName, &provideBranch, &mergeCheckResultString, &mergeCheckTimestamp, &status, &prtime)
		if err != nil { return nil, err }
		var mergeCheckResult *gitlib.MergeCheckResult = nil
		if len(mergeCheckResultString) > 0 {		
			err = json.Unmarshal([]byte(mergeCheckResultString), &mergeCheckResult)
			if err != nil { return nil, err }
		}
		res = append(res, &model.PullRequest{
			PRId: prid,
			PRAbsId: absid,
			Title: title,
			Author: username,
			Timestamp: prtime,
			ReceiverNamespace: namespace,
			ReceiverName: name,
			ReceiverBranch: receiverBranch,
			ProviderNamespace: providerNamespace,
			ProviderName: providerName,
			ProviderBranch: provideBranch,
			Status: status,
			MergeCheckResult: mergeCheckResult,
			MergeCheckTimestamp: mergeCheckTimestamp,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) CountIssue(query string, namespace string, name string, filterType int) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	statusClause := ""
	switch filterType {
    case 1: statusClause = "issue_status = 1"
	case 2: statusClause = "NOT (issue_status = 1)"
	case 3: statusClause = "issue_status = 2"
	case 4: statusClause = "issue_status = 3"
	}
	queryClause := ""
	if query == "" {
		queryClause = "issue_title LIKE ? ESCAPE ?"
	}
	condition := ""
	if statusClause == "" {
		if queryClause == "" {
		} else {
			condition = fmt.Sprintf("WHERE %s", queryClause)
		}
	} else {
		if queryClause == "" {
			condition = fmt.Sprintf("WHERE %s", statusClause)
		} else {
			condition = fmt.Sprintf("WHERE %s AND %s", queryClause, statusClause)
		}
	}
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_issue %s
`, pfx, condition))
	if err != nil { return 0, err }
	var cnt int64
	var r *sql.Row
	if queryClause == "" {
		r = stmt1.QueryRow()
	} else {
		queryPattern := db.ToSqlSearchPattern(query)
		r = stmt1.QueryRow(queryPattern, "\\")
	}
	if r.Err() != nil { return 0, r.Err() }
	err = r.Scan(&cnt)
	if err != nil { return 0, err }
	return cnt, nil
}

func (dbif *SqliteGitusDatabaseInterface) SearchIssuePaginated(query string, namespace string, name string, filterType int, pageNum int64, pageSize int64) ([]*model.Issue, error) {
	pfx := dbif.config.Database.TablePrefix
	statusClause := ""
	switch filterType {
    case 1: statusClause = "AND issue_status = 1"
	case 2: statusClause = "AND NOT (issue_status = 1)"
	case 3: statusClause = "AND issue_status = 2"
	case 4: statusClause = "AND issue_status = 3"
	}
	queryClause := ""
	if query != "" {
		queryClause = "AND issue_title LIKE ? ESCAPE ?"
	}
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, issue_id, issue_author, issue_status, issue_title, issue_content, issue_timestamp, issue_priority
FROM %s_issue
WHERE repo_namespace = ? AND repo_name = ? %s %s
ORDER BY issue_priority DESC, issue_timestamp DESC LIMIT ? OFFSET ?
`, pfx, statusClause, queryClause))
	if err != nil { return nil, err }
	var r *sql.Rows
	if queryClause == "" {
		r, err = stmt1.Query(namespace, name, pageSize, pageNum*pageSize)
	} else {
		queryPattern := db.ToSqlSearchPattern(query)
		r, err = stmt1.Query(namespace, name, queryPattern, "\\", pageSize, pageNum*pageSize)
	}
	if err != nil { return nil, err }
	res := make([]*model.Issue, 0)
	for r.Next() {
		var issueAbsId, issueTimestamp int64
		var issueId, issueStatus, issuePriority int
		var issueAuthor, issueTitle, issueContent string
		err = r.Scan(&issueAbsId, &issueId, &issueAuthor, &issueStatus, &issueTitle, &issueContent, &issueTimestamp, &issuePriority)
		if err != nil { return nil, err }
		res = append(res, &model.Issue{
			IssueAbsId: issueAbsId,
			RepoNamespace: namespace,
			RepoName: name,
			IssueStatus: issueStatus,
			IssueId: issueId,
			IssueTime: issueTimestamp,
			IssueTitle: issueTitle,
			IssueAuthor: issueAuthor,
			IssueContent: issueContent,
			IssuePriority: issuePriority,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) NewPullRequest(username string, title string, receiverNamespace string, receiverName string, receiverBranch string, providerNamespace string, providerName string, providerBranch string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %spull_request
WHERE receiver_namespace = ? AND receiver_name = ?
`, pfx))
	if err != nil { return 0, err }
	var newid int64
	err = stmt1.QueryRow(receiverNamespace, receiverName).Scan(&newid)
	newid += 1
	if err != nil { return 0, err }
	tx, err := dbif.connection.Begin()
	if err != nil { return 0, err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %spull_request(
    username, pull_request_id, title,
    receiver_namespace, receiver_name, receiver_branch,
    provider_namespace, provider_name, provider_branch,
    merge_conflict_check_result, merge_conflict_check_timestamp,
    pull_request_status, pull_request_timestamp
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)
`, pfx))
	if err != nil { return 0, err }
	_, err = stmt.Exec(
		username, newid, title,
		receiverNamespace, receiverName, receiverBranch,
		providerNamespace, providerName, providerBranch,
		new(string), 0,
		model.PULL_REQUEST_OPEN, time.Now().Unix(),
	)
	if err != nil { return 0, err }
	err = tx.Commit()
	if err != nil { return 0, err }
	return newid, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetPullRequest(namespace string, name string, id int64) (*model.PullRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, username, title, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp
FROM %spull_request
WHERE receiver_namespace = ? AND receiver_name = ? AND pull_request_id = ?
`, pfx))
	if err != nil { return nil, err }
	r := stmt.QueryRow(namespace, name, id)
	if r.Err() != nil { return nil, r.Err() }
	var rowid, prtime, mchtime int64
	var username, title, receiverBranch string
	var providerNamespace, providerName, providerBranch string
	var mchResult string
	var prstatus int
	err = r.Scan(&rowid, &username, &title, &receiverBranch, &providerNamespace, &providerName, &providerBranch, &mchResult, &mchtime, &prstatus, &prtime)
	if err != nil { return nil, err }
	var mergeCheckResult *gitlib.MergeCheckResult = nil
	if len(mchResult) > 0 {
		err = json.Unmarshal([]byte(mchResult), &mergeCheckResult)
		if err != nil { return nil, err }
	}
	return &model.PullRequest{
		PRId: id,
		PRAbsId: rowid,
		Title: title,
		Author: username,
		ReceiverNamespace: namespace,
		ReceiverName: name,
		ReceiverBranch: receiverBranch,
		ProviderNamespace: providerNamespace,
		ProviderName: providerName,
		ProviderBranch: providerBranch,
		MergeCheckResult: mergeCheckResult,
		MergeCheckTimestamp: mchtime,
		Status: prstatus,
		Timestamp: prtime,
	}, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetPullRequestByAbsId(absId int64) (*model.PullRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT username, pull_request_id, title, receiver_namespace, receiver_name, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp
FROM %spull_request
WHERE rowid = ?
`, pfx))
	if err != nil { return nil, err }
	r := stmt.QueryRow(absId)
	if r.Err() != nil { return nil, r.Err() }
	var prid, prtime, mchtime int64
	var username, title, receiverNamespace, receiverName, receiverBranch string
	var providerNamespace, providerName, providerBranch string
	var mchResult string
	var prstatus int
	err = r.Scan(&username, &prid, &title, &receiverNamespace, &receiverName, &receiverBranch, &providerNamespace, &providerName, &providerBranch, &mchResult, &mchtime, &prstatus, &prtime)
	if err != nil { return nil, err }
	var mergeCheckResult gitlib.MergeCheckResult
	err = json.Unmarshal([]byte(mchResult), &mergeCheckResult)
	if err != nil { return nil, err }
	return &model.PullRequest{
		PRId: prid,
		PRAbsId: absId,
		Title: title,
		ReceiverNamespace: receiverNamespace,
		ReceiverName: receiverName,
		ReceiverBranch: receiverBranch,
		ProviderNamespace: providerNamespace,
		ProviderName: providerName,
		ProviderBranch: providerBranch,
		MergeCheckResult: &mergeCheckResult,
		MergeCheckTimestamp: mchtime,
		Status: prstatus,
		Timestamp: prtime,
	}, nil
}

func (dbif *SqliteGitusDatabaseInterface) CheckPullRequestMergeConflict(absId int64) (*gitlib.MergeCheckResult, error) {
	// WARNING: currently only works when when the source &
	// the target is git repo. currently (2025.7.28) this check
	// is performed at the controller side, i.e. users cannot
	// create pull request if the repo is not git repo, but the
	// code can still be called. DO NOT CALL UNLESS YOU KNOW
	// WHAT YOU'RE DOING.
	// TODO: fix this after figuring things out.
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT receiver_namespace, receiver_name, receiver_branch, provider_namespace, provider_name, provider_branch
FROM %spull_request
WHERE rowid = ?
`, pfx))
	if err != nil { return nil, err }
	r := stmt.QueryRow(absId)
	if r.Err() != nil { return nil, r.Err() }
	var receiverNamespace, receiverName, receiverBranch string
	var providerNamespace, providerName, providerBranch string
	err = r.Scan(&receiverNamespace, &receiverName, &receiverBranch, &providerNamespace, &providerName, &providerBranch)
	if err != nil { return nil, err }
	tx, err := dbif.connection.Begin()
	if err != nil { return nil, err }
	defer tx.Rollback()
	p := path.Join(dbif.config.GitRoot, receiverNamespace, receiverName)
	lgr := gitlib.NewLocalGitRepository(p)
	remoteName := fmt.Sprintf("%s/%s", providerNamespace, providerName)
	mr, err := lgr.CheckBranchMergeConflict(receiverBranch, remoteName, providerBranch)
	if err != nil { return nil, err }
	stmt2, err := tx.Prepare(fmt.Sprintf(`
UPDATE %spull_request
SET merge_conflict_check_result = ?, merge_conflict_check_timestamp = ?
WHERE rowid = ?
`, pfx))
	if err != nil { return nil, err }
	mrstr, err := json.Marshal(mr)
	if err != nil { return nil, err }
	_, err = stmt2.Exec(string(mrstr), time.Now().Unix(), absId)
	if err != nil { return nil, err }
	err = tx.Commit()
	if err != nil { return nil, err }
	return mr, nil
}

func (dbif *SqliteGitusDatabaseInterface) DeletePullRequest(absId int64) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %spull_request WHERE rowid = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(absId)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllPullRequestEventPaginated(absId int64, pageNum int64, pageSize int64) ([]*model.PullRequestEvent, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT event_type, event_timestamp, event_author, event_content
FROM %spull_request_event
WHERE pull_request_abs_id = ?
ORDER BY event_timestamp ASC LIMIT ? OFFSET ?
`, pfx))
	if err != nil { return nil, err }
	res := make([]*model.PullRequestEvent, 0)
	r, err := stmt.Query(absId, pageSize, pageNum*pageSize)
	if err != nil { return nil, err }
	for r.Next() {
		var eventType int
		var eventTime int64
		var eventAuthor, eventContent string
		err = r.Scan(&eventType, &eventTime, &eventAuthor, &eventContent)
		if err != nil { return nil, err }
		res = append(res, &model.PullRequestEvent{
			PRAbsId: absId,
			EventType: eventType,
			EventTimestamp: eventTime,
			EventAuthor: eventAuthor,
			EventContent: eventContent,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) CheckAndMergePullRequest(absId int64, username string) error {
	// WARNING: currently only works when when the source &
	// the target is git repo. currently (2025.7.28) this check
	// is performed at the controller side, i.e. users cannot
	// create pull request if the repo is not git repo, but the
	// code can still be called. DO NOT CALL UNLESS YOU KNOW
	// WHAT YOU'RE DOING.
	// TODO: fix this after figuring things out.
	r, err := dbif.CheckPullRequestMergeConflict(absId)
	if err != nil { return err }
	// TODO: this would need to be fixed in the future...
	if !r.Successful { return nil }
	pfx := dbif.config.Database.TablePrefix
	stmt0, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT user_email, user_title FROM %s_user WHERE user_name = ?
`, pfx))
	if err != nil { return err }
	rr := stmt0.QueryRow(username)
	if rr.Err() != nil { return rr.Err() }
	var email, userTitle string
	err = rr.Scan(&email, &userTitle)
	if err != nil { return err }
	// fetch
	buf := new(bytes.Buffer)
	cmd1 := exec.Command("git", "fetch", r.ProviderRemoteName, r.ProviderBranch)
	cmd1.Dir = r.ReceiverLocation
	cmd1.Stderr = buf
	err = cmd1.Run()
	if err != nil { return errors.New(err.Error() + ": " + buf.String()) }
	buf.Reset()
	providerFullName := fmt.Sprintf("%s/%s", r.ProviderRemoteName, r.ProviderBranch)
	cmd2 := exec.Command("git", "merge-tree", "--write-tree", r.ReceiverBranch, providerFullName)
	cmd2.Dir = r.ReceiverLocation
	cmd2.Stdout = buf
	err = cmd2.Run()
	if err != nil { return fmt.Errorf("Failed while merge-tree: %s", err.Error()) }
	treeId := strings.TrimSpace(buf.String())
	mergeMessage := fmt.Sprintf("merge: from %s/%s to %s", r.ProviderRemoteName, r.ProviderBranch, r.ReceiverBranch)
	buf.Reset()
	cmd3 := exec.Command("git", "commit-tree", treeId, "-m", mergeMessage, "-p", r.ReceiverBranch, "-p", providerFullName)
	cmd3.Dir = r.ReceiverLocation
	cmd3.Stdout = buf
	cmd3.Env = os.Environ()
	cmd3.Env = append(cmd3.Env, fmt.Sprintf("GIT_AUTHOR_NAME=%s", userTitle))
	cmd3.Env = append(cmd3.Env, fmt.Sprintf("GIT_AUTHOR_EMAIL=%s", email))
	cmd3.Env = append(cmd3.Env, fmt.Sprintf("GIT_COMMITTER_NAME=%s", userTitle))
	cmd3.Env = append(cmd3.Env, fmt.Sprintf("GIT_COMMITTER_EMAIL=%s", email))
	err = cmd3.Run()
	if err != nil { return fmt.Errorf("Failed while commit-tree: %s", err.Error()) }
	commitId := strings.TrimSpace(buf.String())
	buf.Reset()
	receiverBranchFullName := fmt.Sprintf("refs/heads/%s", r.ReceiverBranch)
	cmd4 := exec.Command("git", "update-ref", receiverBranchFullName, commitId)
	cmd4.Dir = r.ReceiverLocation
	cmd4.Stderr = buf
	err = cmd4.Run()
	if err != nil { return fmt.Errorf("Failed while update-ref: %s; %s", err.Error(), buf.String()) }
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	t := time.Now().Unix()
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %spull_request SET pull_request_status = ?, pull_request_timestamp = ? WHERE rowid = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(model.PULL_REQUEST_CLOSED_AS_MERGED, t, absId)
	if err != nil { return err }
	stmt2, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %spull_request_event(pull_request_abs_id, event_type, event_timestamp, event_author, event_content)
VALUES (?,?,?,?,?)
`, pfx))
	if err != nil { return err }
	_, err = stmt2.Exec(absId, model.PULL_REQUEST_EVENT_CLOSE_AS_MERGED, t, username, "")
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) CommentOnPullRequest(absId int64, author string, content string) (*model.PullRequestEvent, error) {
	pfx := dbif.config.Database.TablePrefix
	t := time.Now().Unix()
	tx, err := dbif.connection.Begin()
	if err != nil { return nil, err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %spull_request_event(pull_request_abs_id, event_type, event_timestamp, event_author, event_content) VALUES (?,?,?,?,?)
`, pfx))
	if err != nil { return nil, err }
	eventContentString := content
	_, err = stmt.Exec(absId, model.PULL_REQUEST_EVENT_COMMENT, t, author, eventContentString)
	if err != nil { return nil, err }
	err = tx.Commit()
	if err != nil { return nil, err }
	return &model.PullRequestEvent{
		PRAbsId: absId,
		EventType: model.PULL_REQUEST_EVENT_COMMENT,
		EventTimestamp: t,
		EventAuthor: author,
		EventContent: eventContentString,
	}, nil
}

func (dbif *SqliteGitusDatabaseInterface) CommentOnPullRequestCode(absId int64, comment *model.PullRequestCommentOnCode) (*model.PullRequestEvent, error) {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return nil, err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %spull_request_event(pull_request_abs_id, event_type, event_timestamp, event_author, event_content)
VALUES (?,?,?,?,?)
`, pfx))
	if err != nil { return nil, err }
	t := time.Now().Unix()
	contentBytes, _ := json.Marshal(comment)
	contentString := string(contentBytes)
	_, err = stmt.Exec(absId, model.PULL_REQUEST_EVENT_COMMENT_ON_CODE, t, comment.Username, contentString)
	if err != nil { return nil, err }
	err = tx.Commit()
	if err != nil { return nil, err }
	return &model.PullRequestEvent{
		PRAbsId: absId,
		EventType: model.PULL_REQUEST_EVENT_COMMENT_ON_CODE,
		EventTimestamp: t,
		EventAuthor: comment.Username,
		EventContent: contentString,
	}, nil
}

func (dbif *SqliteGitusDatabaseInterface) ClosePullRequestAsNotMerged(absid int64, author string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %spull_request_event(pull_request_abs_id, event_type, event_timestamp, event_author, event_content)
VALUES (?,?,?,?,?)
`, pfx))
	if err != nil { return err }
	t := time.Now().Unix()
	_, err = stmt.Exec(absid, model.PULL_REQUEST_EVENT_CLOSE_AS_NOT_MERGED, t, author, new(string))
	if err != nil { return err }
	stmt2, err := tx.Prepare(fmt.Sprintf(`
UPDATE %spull_request
SET pull_request_status = ?
WHERE rowid = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt2.Exec(model.PULL_REQUEST_CLOSED_AS_NOT_MERGED, absid)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) ReopenPullRequest(absid int64, author string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %spull_request_event(pull_request_abs_id, event_type, event_timestamp, event_author, event_content)
VALUES (?,?,?,?,?)
`, pfx))
	if err != nil { return err }
	t := time.Now().Unix()
	_, err = stmt.Exec(absid, model.PULL_REQUEST_EVENT_REOPEN, t, author, new(string))
	if err != nil { return err }
	stmt2, err := tx.Prepare(fmt.Sprintf(`
UPDATE %spull_request
SET pull_request_status = ?
WHERE rowid = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt2.Exec(model.PULL_REQUEST_OPEN, absid)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) CountPullRequest(query string, namespace string, name string, filterType int) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	statusClause := ""
	switch filterType {
	case 0: statusClause = ""
	case 1: statusClause = "AND pull_request_status = 1"
	case 2: statusClause = "AND NOT (pull_request_status = 1)"
	}
	queryClause := ""
	if query != "" { queryClause = "AND title LIKE ? ESCAPE ?" }
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %spull_request
WHERE receiver_namespace = ? AND receiver_name = ? %s %s
`, pfx, statusClause, queryClause))
	if err != nil { return 0, err }
	var r *sql.Row
	if query == "" {
		r = stmt.QueryRow(namespace, name)
	} else {
		pat := db.ToSqlSearchPattern(query)
		r = stmt.QueryRow(namespace, name, pat, "\\")
	}
	if r.Err() != nil { return 0, r.Err() }
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, err }
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) SearchPullRequestPaginated(query string, namespace string, name string, filterType int, pageNum int64, pageSize int64) ([]*model.PullRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	statusClause := ""
	switch filterType {
	case 0: statusClause = ""
	case 1: statusClause = "AND pull_request_status = 1"
	case 2: statusClause = "AND NOT (pull_request_status = 1)"
	case 3: statusClause = "AND pull_request_status = 2"
	case 4: statusClause = "AND pull_request_status = 3"
	}
	queryClause := ""
	if query != "" { queryClause = "AND title LIKE ? ESCAPE ?" }
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, username, pull_request_id, title, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp
FROM %spull_request
WHERE receiver_namespace = ? AND receiver_name = ? %s %s
ORDER BY pull_request_timestamp DESC LIMIT ? OFFSET ?
`, pfx, statusClause, queryClause))
	if err != nil { return nil, err }
	var r *sql.Rows
	if query == "" {
		r, err = stmt.Query(namespace, name, pageSize, pageNum*pageSize)
	} else {
		pat := db.ToSqlSearchPattern(query)
		r, err = stmt.Query(namespace, name, pat, "\\", pageSize, pageNum*pageSize)
	}
	if r.Err() != nil { return nil, r.Err() }
	res := make([]*model.PullRequest, 0)
	for r.Next() {
		var prid, absid, prtime int64
		var status int
		var username, title, receiverBranch string
		var providerNamespace, providerName, provideBranch string
		var mergeCheckResultString string
		var mergeCheckTimestamp int64
		err = r.Scan(&absid, &username, &prid, &title, &receiverBranch, &providerNamespace, &providerName, &provideBranch, &mergeCheckResultString, &mergeCheckTimestamp, &status, &prtime)
		if err != nil { return nil, err }
		var mergeCheckResult *gitlib.MergeCheckResult = nil
		if len(mergeCheckResultString) > 0 {		
			err = json.Unmarshal([]byte(mergeCheckResultString), &mergeCheckResult)
			if err != nil { return nil, err }
		}
		res = append(res, &model.PullRequest{
			PRId: prid,
			PRAbsId: absid,
			Title: title,
			Author: username,
			Timestamp: prtime,
			ReceiverNamespace: namespace,
			ReceiverName: name,
			ReceiverBranch: receiverBranch,
			ProviderNamespace: providerNamespace,
			ProviderName: providerName,
			ProviderBranch: provideBranch,
			Status: status,
			MergeCheckResult: mergeCheckResult,
			MergeCheckTimestamp: mergeCheckTimestamp,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllRegisteredEmailOfUser(username string) ([]struct{Email string;Verified bool}, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT email, verified FROM %s_user_email WHERE username = ?
`, pfx))
	if err != nil { return nil, err }
	r, err := stmt.Query(username)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]struct{Email string; Verified bool}, 0)
	var email string
	var verified int
	for r.Next() {
		err = r.Scan(&email, &verified)
		if err != nil { return nil, err }
		res = append(res, struct{Email string; Verified bool}{
			Email: email,
			Verified: verified == 1,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) AddEmail(username string, email string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %s_user_email(username, email, verified) VALUES (?, ?, 0)
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(username, email)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) VerifyRegisteredEmail(username string, email string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_user_email SET verified = 1 WHERE username = ? AND email = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(username, email)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) DeleteRegisteredEmail(username string, email string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %s_user_email WHERE username = ? AND email = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(username, email)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) CheckIfEmailVerified(username string, email string) (bool, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT verified FROM %s_user_email WHERE username = ? AND email = ?
`, pfx))
	if err != nil { return false, err }
	s := stmt.QueryRow(username, email)
	if s.Err() != nil { return false, s.Err() }
	var r int
	err = s.Scan(&r)
	if err != nil { return false, err }
	return r == 1, nil
}

func (dbif *SqliteGitusDatabaseInterface) ResolveEmailToUsername(email string) (string, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT username FROM %s_user_email WHERE email = ? AND verified = 1
`, pfx))
	if err != nil { return "", err }
	defer stmt.Close()
	s := stmt.QueryRow(email)
	if s.Err() != nil { return "", s.Err() }
	var r string
	err = s.Scan(&r)
	if err != nil { return "", err }
	return r, nil
}

func (dbif *SqliteGitusDatabaseInterface) ResolveMultipleEmailToUsername(emailList map[string]string) (map[string]string, error) {
	pfx := dbif.config.Database.TablePrefix
	l := make([]any, 0)
	q := make([]string, 0)
	i := 1
	for k := range emailList {
		l = append(l, k)
		q = append(q, "?")
		i += 1
	}
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT email, username FROM %s_user_email WHERE verified = 1 AND email IN (%s)
`, pfx, strings.Join(q, ",")))
	if err != nil { return nil, err }
	defer stmt.Close()
	s, err := stmt.Query(l...)
	if err != nil { return nil, err }
	defer s.Close()
	var email, username string
	for s.Next() {
		err = s.Scan(&email, &username)
		if err != nil { return nil, err }
		emailList[email] = username
	}
	return emailList, nil
}

func (dbif *SqliteGitusDatabaseInterface) InsertRegistrationRequest(username string, email string, passwordHash string, reason string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %s_user_reg_request(username, email, password_hash, reason, timestamp) VALUES (?,?,?,?,?)
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(username, email, passwordHash, reason, time.Now().Unix())
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) ApproveRegistrationRequest(absid int64) error {
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT username, email, password_hash FROM %s_user_reg_request
WHERE rowid = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt1.Exec(absid)
	if err != nil { return err }
	var username, email, passwordHash string
	r := stmt1.QueryRow(absid)
	if r.Err() != nil { return r.Err() }
	err = r.Scan(&username, &email, &passwordHash)
	if err != nil { return err }
	if dbif.config.EmailConfirmationRequired {
		err = dbif.UpdateUserStatus(username, model.NORMAL_USER_CONFIRM_NEEDED)
		if err != nil { return err }
	} else {
		err = dbif.UpdateUserStatus(username, model.NORMAL_USER)
		if err != nil { return err }
	}
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %s_user_reg_request WHERE username = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(username)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) DisapproveRegistrationRequest(absid int64) error {
	pfx := dbif.config.Database.TablePrefix
	stmt1, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT username FROM %s_user_reg_request
WHERE rowid = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt1.Exec(absid)
	if err != nil { return err }
	var username string
	r := stmt1.QueryRow(absid)
	if r.Err() != nil { return r.Err() }
	err = r.Scan(&username)
	if err != nil { return err }
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %s_user_reg_request WHERE username = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(username)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) GetRegistrationRequestPaginated(pageNum int64, pageSize int64) ([]*model.RegistrationRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT username, email, password_hash, reason, timestamp
FROM %s_user_reg_request
ORDER BY timestamp DESC LIMIT ? OFFSET ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r, err := stmt.Query(pageSize, pageNum*pageSize)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]*model.RegistrationRequest, 0)
	var username, email, passwordHash, reason string
	var timestamp int64
	for r.Next() {
		err = r.Scan(&username, &email, &passwordHash, &reason, &timestamp)
		if err != nil { return nil, err }
		res = append(res, &model.RegistrationRequest{
			Username: username,
			Email: email,
			PasswordHash: passwordHash,
			Reason: reason,
			Timestamp: time.Unix(timestamp, 0),
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetRequestOfUsernamePaginated(username string, pageNum int64, pageSize int64) ([]*model.RegistrationRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, email, password_hash, reason, timestamp
FROM %s_user_reg_request
WHERE username = ?
ORDER BY timestamp DESC LIMIT ? OFFSET ?
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	r, err := stmt.Query(pageSize, pageNum*pageSize)
	if err != nil { return nil, err }
	defer r.Close()
	res := make([]*model.RegistrationRequest, 0)
	var email, passwordHash, reason string
	var rowid, timestamp int64
	for r.Next() {
		err = r.Scan(&rowid, &username, &email, &passwordHash, &reason, &timestamp)
		if err != nil { return nil, err }
		res = append(res, &model.RegistrationRequest{
			AbsId: rowid,
			Username: username,
			Email: email,
			PasswordHash: passwordHash,
			Reason: reason,
			Timestamp: time.Unix(timestamp, 0),
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) CountRegistrationRequest(query string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	var r *sql.Row
	query = strings.TrimSpace(query)
	if len(query) <= 0 {
		stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_user_reg_request
`, pfx))
		if err != nil { return 0, err }
		r = stmt.QueryRow()
		if r.Err() != nil { return 0, r.Err() }
	} else {
		stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_user_reg_request WHERE username LIKE ? ESCAPE ?
`, pfx))
		if err != nil { return 0, err }
		r = stmt.QueryRow(db.ToSqlSearchPattern(query), "\\")
		if r.Err() != nil { return 0, r.Err() }
	}
	var cnt int64
	err := r.Scan(&cnt)
	if err != nil { return 0, err }
	return cnt, nil
}

func (dbif *SqliteGitusDatabaseInterface) SearchRegistrationRequestPaginated(query string, pageNum int64, pageSize int64) ([]*model.RegistrationRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	var r *sql.Rows
	query = strings.TrimSpace(query)
	if len(query) <= 0 {
		stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, username, email, password_hash, timestamp FROM %s_user_reg_request ORDER BY timestamp DESC LIMIT ? OFFSET ?
`, pfx))
		if err != nil { return nil, err }
		r, err = stmt.Query(pageSize, pageNum*pageSize)
		if err != nil { return nil, err }
	} else {
		stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, username, email, password_hash, timestamp FROM %s_user_reg_request WHERE  username LIKE ? ESCAPE ? ORDER BY timestamp LIMIT ? OFFSET ?
`, pfx))
		if err != nil { return nil, err }
		r, err = stmt.Query(db.ToSqlSearchPattern(query), "\\",  pageSize, pageNum*pageSize)
		if err != nil { return nil, err }
	}
	defer r.Close()
	var absid int64
	var username, email, passwordHash string
	var timestamp int64
	res := make([]*model.RegistrationRequest, 0)
	for r.Next() {
		err := r.Scan(&absid, &username, &email, &passwordHash, &timestamp)
		if err != nil { return nil, err }
		res = append(res, &model.RegistrationRequest{
			AbsId: absid,
			Username: username,
			Email: email,
			PasswordHash: passwordHash,
			Timestamp: time.Unix(timestamp, 0),
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetRegistrationRequestByAbsId(absid int64) (*model.RegistrationRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT rowid, username, email, password_hash, timestamp FROM %s_user_reg_request WHERE rowid = ?
`, pfx))
	if err != nil { return nil, err }
	r, err := stmt.Query(absid)
	if err != nil { return nil, err }
	var rowid, timestamp int64
	var username, email, passwordHash string
	err = r.Scan(&rowid, &username, &email, &passwordHash, &timestamp)
	if err != nil { return nil, err }
	return &model.RegistrationRequest{
		AbsId: rowid,
		Username: username,
		Email: email,
		PasswordHash: passwordHash,
		Timestamp: time.Unix(timestamp, 0),
	}, nil
}

func (dbif *SqliteGitusDatabaseInterface) AddRepositoryLabel(ns string, name string, lbl string) error {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_label_list FROM %s_repository WHERE repo_namespace = ? and repo_name = ?
`, pfx))
	if err != nil { return err }
	r := stmt.QueryRow(ns, name)
	if r.Err() != nil { return r.Err() }
	var rll string
	err = r.Scan(&rll)
	if err != nil { return err }
	var tags []string
	if len(rll) <= 0 {
		tags = make([]string, 0)
	} else {
		tags = strings.Split(rll[1:len(rll)-1], "}{")
		if slices.Contains(tags, lbl) { return nil }
	}
	tags = append(tags, lbl)
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt2, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_repository SET repo_label_list = ? WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt2.Exec(fmt.Sprintf("{%s}", strings.Join(tags, "}{")), ns, name)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) RemoveRepositoryLabel(ns string, name string, lbl string) error {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_label_list FROM %s_repository WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return err }
	r := stmt.QueryRow(ns, name)
	if r.Err() != nil { return r.Err() }
	var rll string
	err = r.Scan(&rll)
	if err != nil { return err }
	tags := strings.Split(rll[1:len(rll)-1], "}{")
	idx := slices.Index(tags, lbl)
	if idx == -1 { return nil }
	tags = slices.Delete(tags, idx, idx+1)
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt2, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_repository SET repo_label_list = ? WHERE repo_namespace = ? AND repo_name = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt2.Exec(fmt.Sprintf("{%s}", strings.Join(tags, "}{")), ns, name)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) GetRepositoryLabel(ns string, name string) ([]string, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT repo_label_list FROM %s_repository WHERE repo_namespace = ? and repo_name = ?
`, pfx))
	if err != nil { return nil, err }
	r := stmt.QueryRow(ns, name)
	if r.Err() != nil { return nil, r.Err() }
	var rll string
	err = r.Scan(&rll)
	if err != nil { return nil, err }
	tags := strings.Split(rll[1:len(rll)-1], "}{")
	return tags, nil
}

func (dbif *SqliteGitusDatabaseInterface) CountRepositoryWithLabel(username string, label string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	var r *sql.Row
	var err error
	if len(username) <= 0 {
		stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
WHERE repo_label_list LIKE ? ESCAPE ?
AND repo_status = 1 OR repo_status = 4
`, pfx))
		if err != nil { return 0, err }
		r = stmt.QueryRow(db.ToSqlSearchPattern(fmt.Sprintf("{%s}", label)), "\\")
		if r.Err() != nil { return 0, r.Err() }
	} else {
		stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
WHERE repo_label_list LIKE ? ESCAPE ?
AND (
    repo_status = 1 OR repo_status = 4 OR repo_status = 5
    OR (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?))
`, pfx))
		if err != nil { return 0, err }
		r = stmt.QueryRow(db.ToSqlSearchPattern(fmt.Sprintf("{%s}", label)), "\\", username, db.ToSqlSearchPattern(username), "\\")
		if r.Err() != nil { return 0, r.Err() }
	}
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, err }
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetRepositoryWithLabelPaginated(username string, label string, pageNum int64, pageSize int64) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	var r *sql.Rows
	var err error
	if len(username) <= 0 {
		stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT  repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, rowid
FROM %s_repository
WHERE repo_label_list LIKE ? ESCAPE ?
AND repo_status = 1 OR repo_status = 4
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx))
		if err != nil { return nil, err }
		r, err = stmt.Query(db.ToSqlSearchPattern(fmt.Sprintf("{%s}", label)), "\\", pageSize, pageNum*pageSize)
		if err != nil { return nil, err }
	} else {
		stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT  repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, rowid
FROM %s_repository
WHERE repo_label_list LIKE ? ESCAPE ?
AND (
    repo_status = 1 OR repo_status = 4 OR repo_status = 5
    OR (repo_owner = ? OR repo_acl LIKE ? ESCAPE ?))
ORDER BY rowid ASC LIMIT ? OFFSET ?
`, pfx))
		if err != nil { return nil, err }
		r, err = stmt.Query(db.ToSqlSearchPattern(fmt.Sprintf("{%s}", label)), "\\", username, db.ToSqlSearchPattern(username), "\\", pageSize, pageNum*pageSize)
		if err != nil { return nil, err }
	}
	var rtype uint8
	var ns, name, desc, owner, acl string
	var status int
	var rowid int64
	var forkOriginNs, forkOriginName, labelList string
	res := make([]*model.Repository, 0)
	for r.Next() {
		err = r.Scan(&rtype, &ns, &name, &desc, &owner, &acl, &status, &forkOriginNs, &forkOriginName, &labelList, &rowid)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, name)
		var tags []string = nil
		if len(labelList) > 0 {
			tags = strings.Split(labelList[1:len(labelList)-1], "}{")
		}
		res = append(res, &model.Repository{
			AbsId: rowid,
			Type: rtype,
			Namespace: ns,
			Name: name,
			Owner: owner,
			Description: desc,
			AccessControlList: a,
			Status: model.GitusRepositoryStatus(status),
			Repository: gitlib.NewLocalGitRepository(p),
			ForkOriginNamespace: forkOriginNs,
			ForkOriginName: forkOriginName,
			RepoLabelList: tags,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) NewSnippet(username string, name string, status uint8) (*model.Snippet, error) {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return nil, err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %s_snippet(snippet_full_name, name, username, description, timestamp, status, shared_user)
VALUES (?,?,?,?,?,?,?)
`, pfx))
	if err != nil { return nil, err }
	t := time.Now()
	_, err = stmt.Exec(fmt.Sprintf("%s:%s", username, name), name, username, new(string), t.Unix(), status, "{}")
	if err != nil { return nil, err }
	p := path.Join(dbif.config.SnippetRoot, username, name)
	if !db.IsSubDir(dbif.config.SnippetRoot, p) {
		return nil, db.ErrInvalidLocation
	}
	os.RemoveAll(p)
	err = os.MkdirAll(p, fs.ModeDir|0755)
	if err != nil { return nil, err }
	err = tx.Commit()
	if err != nil { return nil, err }
	return &model.Snippet{
		Name: name,
		BelongingUser: username,
		Description: "",
		Time: t.Unix(),
		FileList: nil,
		Status: status,
		SharedUser: nil,
	}, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllSnippet(username string) ([]*model.Snippet, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user
FROM %s_snippet
WHERE username = ?
`, pfx))
	if err != nil { return nil, err }
	r, err := stmt.Query(username)
	if err != nil { return nil, err }
	defer r.Close()
	var name, desc string
	var time int64
	var status uint8
	var shareduser string
	res := make([]*model.Snippet, 0)
	for r.Next() {
		err = r.Scan(&name, &desc, &time, &status, &shareduser)
		if err != nil { return nil, err }
		var su map[string]bool
		json.Unmarshal([]byte(shareduser), &su)
		res = append(res, &model.Snippet{
			Name: name,
			BelongingUser: username,
			Description: desc,
			Time: time,
			FileList: nil,
			Status: status,
			SharedUser: su,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) CountAllVisibleSnippet(username string, viewingUser string, query string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	var r *sql.Row
	var err error
	if len(query) > 0 {
		q := db.ToSqlSearchPattern(query)
		if len(viewingUser) <= 0 {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_snippet
WHERE username = ? AND (status = 1) AND name LIKE ? ESCAPE ?
`, pfx))
			if err != nil { return 0, err }
			r = stmt.QueryRow(username, q, "\\")
		} else if viewingUser == username {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_snippet
WHERE username = ? AND name LIKE ? ESCAPE ?
`, pfx))
			if err != nil { return 0, err }
			r = stmt.QueryRow(username, q, "\\")
		} else {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_snippet
WHERE username = ? AND (status = 1 OR status = 2 OR (status = 4 AND shared_user LIKE ? ESCAPE ?)) AND name LIKE ? ESCAPE ?
`, pfx))
			if err != nil { return 0, err }
			u := db.ToSqlSearchPattern("\"" + viewingUser + "\"")
			r = stmt.QueryRow(username, u, "\\", q, "\\")
		}
	} else {
		if len(viewingUser) <= 0 {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_snippet
WHERE username = ? AND (status = 1)
`, pfx))
			if err != nil { return 0, err }
			r = stmt.QueryRow(username)
		} else if viewingUser == username {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_snippet WHERE username = ?
`, pfx))
			if err != nil { return 0, err }
			r = stmt.QueryRow(username)
		} else {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT COUNT(*) FROM %s_snippet
WHERE username = ? AND (status = 1 OR status = 2 OR (status = 4 AND shared_user LIKE ? ESCAPE ?))
`, pfx))
			if err != nil { return 0, err }
			u := db.ToSqlSearchPattern("\"" + viewingUser + "\"")
			r = stmt.QueryRow(username, u, "\\")
		}
	}
	if r.Err() != nil { return 0, r.Err() }
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, err }
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) GetAllVisibleSnippetPaginated(username string, viewingUser string, query string, pageNum int64, pageSize int64) ([]*model.Snippet, error) {
	pfx := dbif.config.Database.TablePrefix
	var r *sql.Rows
	var err error
	if len(query) > 0 {
		q := db.ToSqlSearchPattern(query)
		if len(viewingUser) <= 0 {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = ? AND (status = 1) AND name LIKE ? ESCAPE ?
`, pfx))
			if err != nil { return nil, err }
			r, err = stmt.Query(username, q, "\\")
			if err != nil { return nil, err }
		} else if viewingUser == username {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = ? AND name LIKE ? ESCAPE ?
`, pfx))
			if err != nil { return nil, err }
			r, err = stmt.Query(username, q, "\\")
			if err != nil { return nil, err }
		} else {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = ? AND (status = 1 OR status = 2 OR (status = 4 AND shared_user LIKE ? ESCAPE ?)) AND name LIKE ? ESCAPE ?
`, pfx))
			if err != nil { return nil, err }
			u := db.ToSqlSearchPattern("\"" + viewingUser + "\"")
			r, err = stmt.Query(username, u, "\\", q, "\\")
			if err != nil { return nil, err }
		}
	} else {
		if len(viewingUser) <= 0 {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = ? AND (status = 1)
`, pfx))
			if err != nil { return nil, err }
			r, err = stmt.Query(username)
			if err != nil { return nil, err }
		} else if viewingUser == username {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = ?
`, pfx))
			if err != nil { return nil, err }
			r, err = stmt.Query(username)
			if err != nil { return nil, err }
		} else {
			stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = ? AND (status = 1 OR status = 2 OR (status = 4 AND shared_user LIKE ? ESCAPE ?))
`, pfx))
			if err != nil { return nil, err }
			u := db.ToSqlSearchPattern("\"" + viewingUser + "\"")
			r, err = stmt.Query(username, u, "\\")
			if err != nil { return nil, err }
		}
	}
	defer r.Close()
	var name, desc string
	var status uint8
	var timestamp int64
	var sharedUser string
	var sharedUserMap map[string]bool
	res := make([]*model.Snippet, 0)
	for r.Next() {
		err = r.Scan(&name, &desc, &timestamp, &status, &sharedUser)
		if err != nil { return nil, err }
		err = json.Unmarshal([]byte(sharedUser), &sharedUserMap)
		if err != nil { return nil, err }
		res = append(res, &model.Snippet{
			Name: name,
			BelongingUser: username,
			Description: desc,
			Status: status,
			Time: timestamp,
			FileList: nil,
			SharedUser: sharedUserMap,
		})
	}
	return res, nil
}

func (dbif *SqliteGitusDatabaseInterface) DeleteSnippet(username string, name string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
DELETE FROM %s_snippet WHERE username = ? AND name = ?
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(username, name)
	if err != nil { return err }
	p := path.Join(dbif.config.SnippetRoot, username, name)
	err = os.RemoveAll(p)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) SaveSnippetInfo(m *model.Snippet) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_snippet SET description = ?, status = ?, shared_user = ? WHERE username = ? AND name = ?
`, pfx))
	if err != nil { return err }
	s, err := json.Marshal(m.SharedUser)
	if err != nil { return err }
	_, err = stmt.Exec(m.Description, m.Status, string(s), m.BelongingUser, m.Name)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) GetSnippet(username string, name string) (*model.Snippet, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT description, timestamp, status, shared_user FROM %s_snippet
WHERE username = ? AND name = ?
`, pfx))
	if err != nil { return nil, err }
	r := stmt.QueryRow(username, name)
	if r.Err() != nil { return nil, r.Err() }
	var desc, shareduser string
	var timestamp int64
	var status uint8
	err = r.Scan(&desc, &timestamp, &status, &shareduser)
	if err != nil { return nil, err }
	var su map[string]bool
	err = json.Unmarshal([]byte(shareduser), &su)
	if err != nil { return nil, err }
	return &model.Snippet{
		Name: name,
		BelongingUser: username,
		Description: desc,
		Time: timestamp,
		Status: status,
		FileList: nil,
		SharedUser: su,
	}, nil
}

func (dbif *SqliteGitusDatabaseInterface) RegisterWebhookRequest(uuid string, reportUuid string, repoNs string, repoName string, commitId string) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	d := new(model.WebhookResult)
	d.Status = model.WEBHOOK_RESULT_UNDEFINED
	d.ReportUUID = reportUuid
	d.UUID = uuid
	d.RepoNamespace = repoNs
	d.RepoName = repoName
	s, err := json.Marshal(d)
	if err != nil { return err }
	stmt, err := tx.Prepare(fmt.Sprintf(`
INSERT INTO %s_webhook_log(uuid, repo_namespace, repo_name, commit_id, webhook_result)
VALUES (?,?,?,?,?)
`, pfx))
	if err != nil { return err }
	_, err = stmt.Exec(uuid, repoNs, repoName, commitId, string(s))
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) UpdateWebhookResult(uuid string, result *model.WebhookResult) error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	stmt, err := tx.Prepare(fmt.Sprintf(`
UPDATE %s_webhook_log SET webhook_result = ? WHERE uuid = ?
`, pfx))
	if err != nil { return err }
	s, err := json.Marshal(result)
	if err != nil { return err }
	_, err = stmt.Exec(string(s), uuid)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *SqliteGitusDatabaseInterface) GetWebhookResultByUUID(uuid string) (*model.WebhookResult, error) {
	pfx := dbif.config.Database.TablePrefix
	stmt, err := dbif.connection.Prepare(fmt.Sprintf(`
SELECT webhook_result FROM %s_webhook_log
WHERE uuid = ?
`, pfx))
	if err != nil { return nil, err }
	r := stmt.QueryRow(uuid)
	if r.Err() != nil { return nil, r.Err() }
	var webhookResultStr string
	err = r.Scan(&webhookResultStr)
	if err != nil { return nil, err }
	webhookResult := new(model.WebhookResult)
	err = json.Unmarshal([]byte(webhookResultStr), &webhookResult)
	if err != nil { return nil, err }
	return webhookResult, nil
}

