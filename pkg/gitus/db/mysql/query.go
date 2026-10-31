package mysql

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/GitusCodeForge/Gitus/pkg/gitlib"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/db"
	"github.com/GitusCodeForge/Gitus/pkg/gitus/model"
)

func (dbif *MysqlGitusDatabaseInterface) GetUserByName(name string) (*model.GitusUser, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT user_title, user_email, user_bio, user_website, user_reg_datetime, user_password_hash, user_status, user_2fa_config, user_website_preference
FROM %s_user
WHERE user_name = $1
`, pfx), name)
	var title, email, bio, website, password string
	var tfaConfig, websitePreference string
	var datetime time.Time
	var status int
	err := stmt.Scan(&title, &email, &bio, &website, &datetime, &password, &status, &tfaConfig, &websitePreference)
	if errors.Is(err, sql.ErrNoRows) { return nil, db.ErrEntityNotFound }
	if err != nil { return nil, err }
	var tfa model.GitusUser2FAConfig
	if err = json.Unmarshal([]byte(tfaConfig), &tfa); err != nil { return nil, err }
	var wPref model.GitusUserWebsitePreference
	if err = json.Unmarshal([]byte(websitePreference), &wPref); err != nil { return nil, err }
	return &model.GitusUser{
		Name: name,
		Title: title,
		Email: email,
		Bio: bio,
		Website: website,
		PasswordHash: password,
		RegisterTime: datetime.Unix(),
		Status: model.GitusUserStatus(status),
		TFAConfig: tfa,
		WebsitePreference: wPref,
	}, nil
}

func (dbif *MysqlGitusDatabaseInterface) GetAllAuthKeyByUsername(name string) ([]model.GitusAuthKey, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.query(ctx, fmt.Sprintf(`
SELECT key_name, key_text
FROM %s_user_authkey
WHERE user_name = $1
`, pfx), name)
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make([]model.GitusAuthKey, 0)
	for stmt.Next() {
		var kname, ktext string
		err := stmt.Scan(&kname, &ktext)
		if err != nil { return nil, err }
		res = append(res, model.GitusAuthKey{
			UserName: name,
			KeyName: kname,
			KeyText: ktext,
		})
	}
	return res, nil
}

// GetUserLoginInfo returns only what UseLoginInfo needs, without decoding the
// 2FA config and the website preference JSON that GetUserByName has to read.
func (dbif *MysqlGitusDatabaseInterface) GetUserLoginInfo(name string) (*model.GitusUserLoginInfo, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT user_name, user_title, user_email, user_status
FROM %s_user
WHERE user_name = $1
`, pfx), name)
	var username, title, email string
	var status int
	err := stmt.Scan(&username, &title, &email, &status)
	if errors.Is(err, sql.ErrNoRows) { return nil, db.ErrEntityNotFound }
	if err != nil { return nil, err }
	return &model.GitusUserLoginInfo{
		Name: username,
		Title: title,
		Email: email,
		Status: model.GitusUserStatus(status),
	}, nil
}

func (dbif *MysqlGitusDatabaseInterface) GetAuthKeyByName(userName string, keyName string) (*model.GitusAuthKey, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT key_text FROM %s_user_authkey WHERE user_name = $1 AND key_name = $2
`, pfx), userName, keyName)
	var ktext string
	err := stmt.Scan(&ktext)
	if errors.Is(err, sql.ErrNoRows) { return nil, db.ErrEntityNotFound }
	if err != nil { return nil, err }
	return &model.GitusAuthKey{
		UserName: userName,
		KeyName: keyName,
		KeyText: ktext,
	}, nil
}

func (dbif *MysqlGitusDatabaseInterface) RegisterAuthKey(username string, keyname string, keytext string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
INSERT INTO %s_user_authkey(user_name, key_name, key_text)
VALUES ($1, $2, $3)
`, pfx), username, keyname, keytext)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) UpdateAuthKey(username string, keyname string, keytext string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_user_authkey
SET key_text = $1
WHERE user_name = $2 AND key_name = $3
`, pfx), keytext, username, keyname)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) RemoveAuthKey(username string, keyname string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
DELETE FROM %s_user_authkey
WHERE user_name = $1 AND key_name = $2
`, pfx), username, keyname)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) GetAllSignKeyByUsername(name string) ([]model.GitusSigningKey, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.query(ctx, fmt.Sprintf(`
SELECT key_name, key_text FROM %s_user_signkey WHERE user_name = $1
`, pfx), name)
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make([]model.GitusSigningKey, 0)
	for stmt.Next() {
		var kname, ktext string
		err := stmt.Scan(&kname, &ktext)
		if err != nil { return nil, err }
		res = append(res, model.GitusSigningKey{
			UserName: name,
			KeyName: kname,
			KeyText: ktext,
		})
	}
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) GetSignKeyByName(userName string, keyName string) (*model.GitusSigningKey, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT key_text FROM %s_user_signkey WHERE user_name = $1 AND key_name = $2
`, pfx), userName, keyName)
	var ktext string
	err := stmt.Scan(&ktext)
	if errors.Is(err, sql.ErrNoRows) { return nil, db.ErrEntityNotFound }
	if err != nil { return nil, err }
	return &model.GitusSigningKey{
		UserName: userName,
		KeyName: keyName,
		KeyText: ktext,
	}, nil
}

func (dbif *MysqlGitusDatabaseInterface) UpdateSignKey(username string, keyname string, keytext string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_user_signkey
SET key_text = $1
WHERE user_name = $2 AND key_name = $3
`, pfx), keytext, username, keyname)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) RegisterSignKey(username string, keyname string, keytext string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
INSERT INTO %s_user_signkey(user_name, key_name, key_text)
VALUES ($1, $2, $3)
`, pfx), username, keyname, keytext)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) RemoveSignKey(username string, keyname string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
DELETE FROM %s_user_signkey
WHERE user_name = $1 AND key_name = $2
`, pfx), username, keyname)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

// GetRepositoryWithNamespace returns the repository and the namespace it lives
// in with a single query. Resolving a /repo/... URL used to cost two round
// trips (GetNamespaceByName then GetRepositoryByName) on every request.
func (dbif *MysqlGitusDatabaseInterface) GetRepositoryWithNamespace(nsName string, repoName string) (*model.Namespace, *model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT repo.repo_type, repo.repo_namespace, repo.repo_name, repo.repo_description, repo.repo_owner, repo.repo_acl, repo.repo_status, repo.repo_fork_origin_namespace, repo.repo_fork_origin_name, repo.repo_label_list, repo.repo_webhook, repo.repo_absid,
ns.ns_name, ns.ns_title, ns.ns_description, ns.ns_email, ns.ns_owner, ns.ns_reg_datetime, ns.ns_status, ns.ns_acl
FROM %s_repository repo
INNER JOIN %s_namespace ns ON ns.ns_name = repo.repo_namespace
WHERE repo.repo_namespace = $1 AND repo.repo_name = $2
`, pfx, pfx), nsName, repoName)
	var repoType, status int
	var repoNs, name, desc, owner, acl, forkOriginNs, forkOriginName, labelList, webhookStr string
	var absId int64
	var nsTitle, nsDesc, nsEmail, nsOwner, nsACL string
	var nsRegTime time.Time
	var nsStatus int
	err := stmt.Scan(
		&repoType, &repoNs, &name, &desc, &owner, &acl, &status, &forkOriginNs, &forkOriginName, &labelList, &webhookStr, &absId,
		&nsName, &nsTitle, &nsDesc, &nsEmail, &nsOwner, &nsRegTime, &nsStatus, &nsACL)
	if err == sql.ErrNoRows { return nil, nil, db.ErrEntityNotFound }
	if err != nil { return nil, nil, err }
	repo, err := dbif.buildRepository(repoType, repoNs, name, desc, owner, acl, status, forkOriginNs, forkOriginName, labelList, webhookStr, absId)
	if err != nil { return nil, nil, err }
	ns, err := dbif.buildNamespace(nsName, nsTitle, nsDesc, nsEmail, nsOwner, nsRegTime, nsACL, nsStatus)
	if err != nil { return nil, nil, err }
	return ns, repo, nil
}

func (dbif *MysqlGitusDatabaseInterface) GetNamespaceByName(name string) (*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_acl, ns_status
FROM %s_namespace
WHERE ns_name = $1
`, pfx), name)
	var title, description, email, owner, acl string
	var datetime time.Time
	var status int
	err := stmt.Scan(&title, &description, &email, &owner, &datetime, &acl, &status)
	if errors.Is(err, sql.ErrNoRows) { return nil, db.ErrEntityNotFound }
	if err != nil { return nil, err }
	a, err := model.ParseACL(acl)
	if err != nil { return nil, err }
	return &model.Namespace{
		Name: name,
		Title: title,
		Description: description,
		Email: email,
		Owner: owner,
		RegisterTime: datetime.Unix(),
		Status: model.GitusNamespaceStatus(status),
		ACL: a,
	}, nil
}

func (dbif *MysqlGitusDatabaseInterface) GetRepositoryByName(nsName string, repoName string) (*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT repo_type, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook, repo_absid
FROM %s_repository
WHERE repo_namespace = $1 AND repo_name = $2
`, pfx), nsName, repoName)
	var description, owner, acl, forkOriginNamespace, forkOriginName, webhook string
	var repoType, repoStatus int
	var rowid int64
	var labelList string
	err := stmt.Scan(&repoType, &description, &owner, &acl, &repoStatus, &forkOriginNamespace, &forkOriginName, &labelList, &webhook, &rowid)
	if errors.Is(err, sql.ErrNoRows) { return nil, db.ErrEntityNotFound }
	if err != nil { return nil, err }
	p := path.Join(dbif.config.GitRoot, nsName, repoName)
	localRepo, err := model.CreateLocalRepository(uint8(repoType), nsName, repoName, p)
	if err != nil { return nil, err }
	res, err := model.NewRepository(nsName, repoName, localRepo)
	if err != nil { return nil, err }
	res.AbsId = rowid
	res.Type = uint8(repoType)
	res.Description = description
	res.Owner = owner
	res.Status = model.GitusRepositoryStatus(repoStatus)
	res.ForkOriginNamespace = forkOriginNamespace
	res.ForkOriginName = forkOriginName
	var tags []string = nil
	if len(labelList) > 0 {
		tags = strings.Split(labelList[1:len(labelList)-1], "}{")
	}
	res.RepoLabelList = tags
	aclobj, err := model.ParseACL(acl)
	if err != nil { return nil, err }
	res.AccessControlList = aclobj
	webhookobj, err := model.ParseWebHookConfig(webhook)
	if err != nil { return nil, err }
	res.WebHookConfig = webhookobj
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) GetAllNamespace() (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.query(ctx, fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_acl, ns_status
FROM %s_namespace
`, pfx))
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make(map[string]*model.Namespace, 0)
	var name, title, description, email, owner, acl string
	var datetime time.Time
	var status int
	for stmt.Next() {
		err = stmt.Scan(&name, &title, &description, &email, &owner, &datetime, &acl, &status)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: description,
			Email: email,
			Owner: owner,
			RegisterTime: datetime.Unix(),
			ACL: a,
			Status: model.GitusNamespaceStatus(status),
		}
	}
	return  res, nil
}

func (dbif *MysqlGitusDatabaseInterface) GetAllVisibleNamespacePaginated(username string, pageNum int64, pageSize int64) (map[string]*model.Namespace, error) {
	return dbif.listVisibleNamespaces(username, "", pageNum, pageSize)
}

// listVisibleNamespaces is the single implementation behind both the namespace
// browse page and the namespace search page; they only differ in whether a
// keyword is given.
func (dbif *MysqlGitusDatabaseInterface) listVisibleNamespaces(username string, query string, pageNum int64, pageSize int64) (map[string]*model.Namespace, error) {
	ctx := context.Background()
	b := db.NewNumberedQueryBuilder()
	b.And(dbif.nsVisibilityCond(b, username))
	b.And(nsKeywordCond(b, query))
	q := fmt.Sprintf("SELECT %s FROM %s_namespace %s ORDER BY ns_absid ASC %s",
		namespaceColumns, dbif.config.Database.TablePrefix, b.Where(), b.Limit(pageSize, pageNum))
	rows, err := dbif.query(ctx, q, b.Args()...)
	if err != nil { return nil, err }
	defer rows.Close()
	return dbif.scanNamespaceRows(rows)
}

func (dbif *MysqlGitusDatabaseInterface) SearchAllVisibleNamespacePaginated(username string, query string, pageNum int64, pageSize int64) (map[string]*model.Namespace, error) {
	return dbif.listVisibleNamespaces(username, query, pageNum, pageSize)
}

func (dbif *MysqlGitusDatabaseInterface) GetAllVisibleRepositoryPaginated(username string, pageNum int64, pageSize int64) ([]*model.Repository, error) {
	return dbif.listRepositories(username, "", pageNum, pageSize)
}

// listRepositories is the single implementation behind both the repository
// browse page and the repository search page.
func (dbif *MysqlGitusDatabaseInterface) listRepositories(username string, query string, pageNum int64, pageSize int64) ([]*model.Repository, error) {
	ctx := context.Background()
	b := db.NewNumberedQueryBuilder()
	b.And(dbif.repoVisibilityCond(b, username))
	b.And(repoKeywordCond(b, query))
	q := fmt.Sprintf("SELECT %s FROM %s_repository repo %s ORDER BY repo.repo_absid ASC %s",
		repositoryColumns, dbif.config.Database.TablePrefix, b.Where(), b.Limit(pageSize, pageNum))
	rows, err := dbif.query(ctx, q, b.Args()...)
	if err != nil { return nil, err }
	defer rows.Close()
	return dbif.scanRepositoryRows(rows)
}

func (dbif *MysqlGitusDatabaseInterface) SearchAllVisibleRepositoryPaginated(username string, query string, pageNum int64, pageSize int64) ([]*model.Repository, error) {
	return dbif.listRepositories(username, query, pageNum, pageSize)
}

func (dbif *MysqlGitusDatabaseInterface) GetAllNamespaceByOwner(name string) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.query(ctx, fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %s_namespace
WHERE ns_owner = $1
`, pfx), name)
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make(map[string]*model.Namespace, 0)
	for stmt.Next() {
		var name, title, desc, email, owner, acl string
		var regtime time.Time
		var status int64
		err = stmt.Scan(&name, &title, &desc, &email, &owner, &regtime, &status, &acl)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: regtime.Unix(),
			ACL: a,
			Status: model.GitusNamespaceStatus(status),
		}
	}
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) GetAllRepositoryFromNamespace(name string) (map[string]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook, repo_absid
FROM %s_repository
WHERE repo_namespace = $1
`, pfx), name)
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make(map[string]*model.Repository, 0)
	var rType, rStatus int
	var repoName, desc, owner, acl, forkOriginNs, forkOriginName, labelList, webhookstr string
	var rowid int64
	for stmt.Next() {
		err := stmt.Scan(&rType, &repoName, &desc, &owner, &acl, &rStatus, &forkOriginNs, &forkOriginName, &labelList, &webhookstr, &rowid)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, name, repoName)
		m, err := model.CreateLocalRepository(uint8(rType), name, repoName, p)
		if err != nil { return nil, err }
		var tags []string = nil
		if len(labelList) > 0 {
			tags = strings.Split(labelList[1:len(labelList)-1], "}{")
		}
		webhookobj, err := model.ParseWebHookConfig(webhookstr)
		res[repoName] = &model.Repository{
			AbsId: rowid,
			Namespace: name,
			Name: repoName,
			Description: desc,
			Owner: owner,
			AccessControlList: a,
			Status: model.GitusRepositoryStatus(rStatus),
			Type: uint8(rType),
			ForkOriginNamespace: forkOriginNs,
			ForkOriginName: forkOriginName,
			Repository: m,
			RepoLabelList: tags,
			WebHookConfig: webhookobj,
		}
	}
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) GetAllVisibleRepositoryFromNamespace(username string, ns string) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var stmt *sql.Rows
	var err error
	if len(username) > 0 {
		stmt, err = dbif.query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook, repo_absid
FROM %s_repository
WHERE repo_namespace = $1 AND (repo_status = 1 OR repo_owner = $2 OR JSON_CONTAINS_PATH(repo_acl, 'one', CONCAT('$.acl."', $2, '"')))
`, pfx), ns, username)
	} else {
		stmt, err = dbif.query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook, repo_absid
FROM %s_repository
WHERE repo_namespace = $1 AND repo_status = 1
`, pfx), ns)
	}
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make([]*model.Repository, 0)
	var rType, rStatus int
	var repoName, desc, owner, acl, forkOriginNs, forkOriginName, webhookstr string
	var labelList string
	var rowid int64
	for stmt.Next() {
		err := stmt.Scan(&rType, &repoName, &desc, &owner, &acl, &rStatus, &forkOriginNs, &forkOriginName, &labelList, &webhookstr, &rowid)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, repoName)
		m, err := model.CreateLocalRepository(uint8(rType), ns, repoName, p)
		if err != nil { return nil, err }
		var tags []string = nil
		if len(labelList) > 0 {
			tags = strings.Split(labelList[1:len(labelList)-1], "}{")
		}
		webhookobj, err := model.ParseWebHookConfig(webhookstr)
		res = append(res, &model.Repository{
			AbsId: rowid,
			Namespace: ns,
			Name: repoName,
			Description: desc,
			Owner: owner,
			AccessControlList: a,
			Status: model.GitusRepositoryStatus(rStatus),
			Type: uint8(rType),
			ForkOriginNamespace: forkOriginNs,
			ForkOriginName: forkOriginName,
			Repository: m,
			RepoLabelList: tags,
			WebHookConfig: webhookobj,
		})
	}
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) RegisterUser(name string, email string, passwordHash string, status model.GitusUserStatus) (*model.GitusUser, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback()
	t := time.Now()
	webp := new(model.GitusUserWebsitePreference)
	webp.UseSiteWideThemeConfig = true
	_, err = execTx(ctx, tx, fmt.Sprintf(`
INSERT INTO %s_user(user_name, user_title, user_email, user_bio, user_website, user_reg_datetime, user_password_hash, user_status, user_2fa_config, user_website_preference)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
`, pfx), name, name, email, new(string), new(string), t, passwordHash, status, new(model.GitusUser2FAConfig), webp)
	if err != nil { return nil, err }
	err = tx.Commit()
	if err != nil { return nil, err }
	return &model.GitusUser{
		Name: name,
		Title: name,
		Email: email,
		Bio: "",
		Website: "",
		PasswordHash: passwordHash,
		RegisterTime: t.Unix(),
		Status: model.GitusUserStatus(status),
	}, nil
}

func (dbif *MysqlGitusDatabaseInterface) UpdateUserInfo(name string, uobj *model.GitusUser) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_user
SET user_title = $1, user_email = $2, user_bio = $3, user_website = $4, user_status = $5, user_2fa_config = $7, user_website_preference = $8
WHERE user_name = $6
`, pfx), uobj.Title, uobj.Email, uobj.Bio, uobj.Website, uobj.Status, name, uobj.TFAConfig, uobj.WebsitePreference)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) UpdateUserPassword(name string, newPasswordHash string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_user
SET user_password_hash = $1
WHERE user_name = $2
`, pfx), newPasswordHash, name)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) HardDeleteUserByName(name string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	for _, ref := range []struct{ table, column string }{
		{"user_authkey", "user_name"},
		{"user_signkey", "user_name"},
		{"user_email", "username"},
	} {
		_, err = execTx(ctx, tx, fmt.Sprintf(`DELETE FROM %s_%s WHERE %s = $1`, pfx, ref.table, ref.column), name)
		if err != nil { return err }
	}
	_, err = execTx(ctx, tx, fmt.Sprintf(`
DELETE FROM %s_user WHERE user_name = $1
`, pfx), name)
	if err != nil { return err }
	return tx.Commit()
}

func (dbif *MysqlGitusDatabaseInterface) UpdateUserStatus(name string, newStatus model.GitusUserStatus) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_user
SET user_status = $1
WHERE user_name = $2
`, pfx), newStatus, name)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) RegisterNamespace(name string, ownerUsername string) (*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback()
	t := time.Now()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
INSERT INTO %s_namespace(ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_acl, ns_status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`, pfx), name, name, new(string), new(string), ownerUsername, t, model.NewACL(), model.NAMESPACE_NORMAL_PUBLIC)
	if err != nil { return nil, err }
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
		RegisterTime: t.Unix(),
		Status: model.NAMESPACE_NORMAL_PUBLIC,
		ACL: nil,
		RepositoryList: nil,
		LocalPath: nsPath,
	}, nil
}

func (dbif *MysqlGitusDatabaseInterface) UpdateNamespaceInfo(name string, nsobj *model.Namespace) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_namespace
SET ns_title = $1, ns_description = $2, ns_email = $3, ns_owner = $4, ns_status = $5
WHERE ns_name = $6
`, pfx), nsobj.Name, nsobj.Description, nsobj.Email, nsobj.Owner, nsobj.Status, name)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) UpdateNamespaceOwner(name string, newOwner string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_namespace
SET ns_owner = $1
WHERE ns_name = $2
`, pfx), newOwner, name)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) UpdateNamespaceStatus(name string, newStatus model.GitusNamespaceStatus) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_namespace
SET ns_status = $1
WHERE ns_name = $2
`, pfx), newStatus, name)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) HardDeleteNamespaceByName(name string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	rows, err := dbif.query(ctx, fmt.Sprintf(`
SELECT repo_name FROM %s_repository WHERE repo_namespace = $1
`, pfx), name)
	if err != nil { return err }
	repoNames := make([]string, 0)
	for rows.Next() {
		var repoName string
		if err = rows.Scan(&repoName); err != nil { rows.Close(); return err }
		repoNames = append(repoNames, repoName)
	}
	rows.Close()
	for _, repoName := range repoNames {
		if err = dbif.HardDeleteRepository(name, repoName); err != nil { return err }
	}
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
DELETE FROM %s_namespace WHERE ns_name = $1
`, pfx), name)
	if err != nil { return err }
	p := path.Join(dbif.config.GitRoot, name)
	if err = os.RemoveAll(p); err != nil { return err }
	return tx.Commit()
}

func (dbif *MysqlGitusDatabaseInterface) CreateRepository(ns string, name string, repoType uint8, owner string) (*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback()
	webhookobj := new(model.WebHookConfig)
	_, err = execTx(ctx, tx, fmt.Sprintf(`
INSERT INTO %s_repository(repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
`, pfx), repoType, ns, name, new(string), owner, model.NewACL(), model.REPO_NORMAL_PUBLIC, new(string), new(string), new(string), webhookobj)
	if err != nil { return nil, err }
	p := path.Join(dbif.config.GitRoot, ns, name)
	if !db.IsSubDir(dbif.config.GitRoot, p) {
		return nil, errors.New("Invalid repository path")
	}
	if err = os.RemoveAll(p); err != nil { return nil, err }
	if err = os.MkdirAll(p, os.ModeDir|0755); err != nil { return nil, err }
	lr, err := model.CreateLocalRepository(repoType, ns, name, p)
	if err != nil { return nil, err }
	if err = model.InitLocalRepository(lr); err != nil { return nil, err }
	r, err := model.NewRepository(ns, name, lr)
	if err != nil { return nil, err }
	r.Type = repoType
	r.Owner = owner
	r.RepoLabelList = nil
	r.WebHookConfig = webhookobj
	// Install the hook before committing, so a failure cannot leave a
	// committed repository without the hook that drives webhooks.
	err = model.InstallManagedHook(lr, dbif.config.FilePath, r.FullName())
	if err != nil { return nil, err }
	if err = tx.Commit(); err != nil { return nil, err }
	return r, nil
}

func (dbif *MysqlGitusDatabaseInterface) SetUpCloneRepository(originNs string, originName string, targetNs string, targetName string, owner string) (*model.Repository, error) {
	// TODO: fix this for multi vcs support
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback()
	webhookobj := new(model.WebHookConfig)
	_, err = execTx(ctx, tx, fmt.Sprintf(`
INSERT INTO %s_repository(repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
`, pfx), model.REPO_TYPE_GIT, targetNs, targetName, new(string), owner, model.NewACL(), model.REPO_NORMAL_PUBLIC, originNs, originName, new(string), webhookobj)
	if err != nil { return nil, err }
	originP := path.Join(dbif.config.GitRoot, originNs, originName)
	targetP := path.Join(dbif.config.GitRoot, targetNs, targetName)
	if !db.IsSubDir(dbif.config.GitRoot, targetP) {
		return nil, errors.New("Invalid location for fork")
	}
	if err = os.RemoveAll(targetP); err != nil { return nil, err }
	if err = os.MkdirAll(targetP, os.ModeDir|0775); err != nil { return nil, err }
	originLr, err := model.CreateLocalRepository(model.REPO_TYPE_GIT, originNs, originName, originP)
	if err != nil { return nil, err }
	targetLr, err := model.CreateLocalForkOf(originLr, targetNs, targetName, targetP)
	if err != nil { return nil, err }
	r, err := model.NewRepository(targetNs, targetName, targetLr)
	if err != nil { return nil, err }
	r.Type = model.REPO_TYPE_GIT
	r.Owner = owner
	r.RepoLabelList = nil
	r.WebHookConfig = webhookobj
	err = model.InstallManagedHook(targetLr, dbif.config.FilePath, r.FullName())
	if err != nil { return nil, err }
	if err = tx.Commit(); err != nil { return nil, err }
	return r, nil
}

func (dbif *MysqlGitusDatabaseInterface) UpdateRepositoryInfo(ns string, name string, robj *model.Repository) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	tag, err := execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_repository
SET repo_description = $1, repo_owner = $2, repo_status = $3, repo_webhook = $6
WHERE repo_namespace = $4 AND repo_name = $5
`, pfx), robj.Description, robj.Owner, robj.Status, ns, name, robj.WebHookConfig)
	if err != nil { return err }
	affected, err := tag.RowsAffected()
	if err != nil { return err }
	if affected <= 0 { return db.ErrEntityNotFound }
	if err = tx.Commit(); err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) UpdateRepositoryStatus(ns string, name string, status model.GitusRepositoryStatus) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_repository
SET repo_status = $1
WHERE repo_namespace = $2 AND repo_name = $3
`, pfx), status, ns, name)
	if err != nil { return err }
	if err = tx.Commit(); err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) HardDeleteRepository(ns string, name string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	// The foreign keys from issues and pull request events would otherwise
	// block the delete; clean up the rows that belong to this repository
	// first.
	statements := []string{
		fmt.Sprintf(`DELETE FROM %s_issue_event WHERE issue_absid IN (SELECT issue_absid FROM %s_issue WHERE repo_namespace = $1 AND repo_name = $2)`, pfx, pfx),
		fmt.Sprintf(`DELETE FROM %s_issue WHERE repo_namespace = $1 AND repo_name = $2`, pfx),
		fmt.Sprintf(`DELETE FROM %s_pull_request_event WHERE pull_request_absid IN (SELECT pull_request_absid FROM %s_pull_request WHERE receiver_namespace = $1 AND receiver_name = $2)`, pfx, pfx),
		fmt.Sprintf(`DELETE FROM %s_pull_request WHERE receiver_namespace = $1 AND receiver_name = $2`, pfx),
		fmt.Sprintf(`DELETE FROM %s_webhook_log WHERE repo_namespace = $1 AND repo_name = $2`, pfx),
		fmt.Sprintf(`DELETE FROM %s_id_counter WHERE repo_namespace = $1 AND repo_name = $2`, pfx),
		fmt.Sprintf(`DELETE FROM %s_repository WHERE repo_namespace = $1 AND repo_name = $2`, pfx),
	}
	for _, stmt := range statements {
		if _, err = execTx(ctx, tx, stmt, ns, name); err != nil { return err }
	}
	p := path.Join(dbif.config.GitRoot, ns, name)
	if err = os.RemoveAll(p); err != nil { return err }
	return tx.Commit()
}

func (dbif *MysqlGitusDatabaseInterface) GetAllUsers(pageNum int64, pageSize int64) ([]*model.GitusUser, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.query(ctx, fmt.Sprintf(`
SELECT user_name, user_title, user_email, user_bio, user_website, user_reg_datetime, user_password_hash, user_status
FROM %s_user
ORDER BY user_id ASC LIMIT $1 OFFSET $2
`, pfx), pageSize, pageNum*pageSize)
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make([]*model.GitusUser, 0)
	var name, title, email, bio, website, hash string
	var rt time.Time
	var status int
	for stmt.Next() {
		err := stmt.Scan(&name, &title, &email, &bio, &website, &rt, &hash, &status)
		if err != nil { return nil, err }
		res = append(res, &model.GitusUser{
			Name: name,
			Title: title,
			Email: email,
			Bio: bio,
			Website: website,
			PasswordHash: hash,
			RegisterTime: rt.Unix(),
			Status: model.GitusUserStatus(status),
		})
	}
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) GetAllNamespaces(pageNum int64, pageSize int64) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.query(ctx, fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_acl, ns_status
FROM %s_namespace
ORDER BY ns_absid ASC LIMIT $1 OFFSET $2
`, pfx), pageSize, pageNum*pageSize)
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make(map[string]*model.Namespace, 0)
	var name, title, desc, email, owner, acl string
	var rt time.Time
	var status int
	for stmt.Next() {
		err := stmt.Scan(&name, &title, &desc, &email, &owner, &rt, &acl, &status)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, name)
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: desc,
			Email: email,
			Owner: owner,
			RegisterTime: rt.Unix(),
			Status: model.GitusNamespaceStatus(status),
			ACL: a,
			RepositoryList: nil,
			LocalPath: p,
		}
	}
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) GetAllRepositories(pageNum int64, pageSize int64) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook, repo_absid
FROM %s_repository
ORDER BY repo_absid ASC LIMIT $1 OFFSET $2
`, pfx), pageSize, pageNum*pageSize)
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make([]*model.Repository, 0)
	var rType int
	var ns, name, desc, owner, acl, forkOriginNs, forkOriginName, labelList, webhookstr string
	var rStatus int
	var rowid int64
	for stmt.Next() {
		err = stmt.Scan(&rType, &ns, &name, &desc, &owner, &acl, &rStatus, &forkOriginNs, &forkOriginName, &labelList, &webhookstr, &rowid)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, name)
		lr, err := model.CreateLocalRepository(uint8(rType), ns, name, p)
		if err != nil { return nil, err }
		var tags []string = nil
		if len(labelList) > 0 {
			tags = strings.Split(labelList[1:len(labelList)-1], "}{")
		}
		webhookobj, err := model.ParseWebHookConfig(webhookstr)
		if err != nil { return nil, err }
		res = append(res, &model.Repository{
			AbsId: rowid,
			Type: uint8(rType),
			Namespace: ns,
			Name: name,
			Owner: owner,
			Description: desc,
			AccessControlList: a,
			Status: model.GitusRepositoryStatus(rStatus),
			Repository: lr,
			ForkOriginNamespace: forkOriginNs,
			ForkOriginName: forkOriginName,
			RepoLabelList: tags,
			WebHookConfig: webhookobj,
		})
	}
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) CountAllUser() (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_user
`, pfx))
	var r int64
	err := stmt.Scan(&r)
	if err != nil { return 0, err }
	return r, nil
}

func (dbif *MysqlGitusDatabaseInterface) CountAllNamespace() (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_namespace
`, pfx))
	var r int64
	err := stmt.Scan(&r)
	if err != nil { return 0, err }
	return r, nil
}

func (dbif *MysqlGitusDatabaseInterface) CountAllRepositories() (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
`, pfx))
	var r int64
	err := stmt.Scan(&r)
	if err != nil { return 0, err }
	return r, nil
}

func (dbif *MysqlGitusDatabaseInterface) CountAllRepositoriesSearchResult(q string) (int64, error) {
	if len(q) <= 0 { return dbif.CountAllRepositories() }
	b := db.NewNumberedQueryBuilder()
	b.And(repoKeywordCond(b, q))
	sql := fmt.Sprintf("SELECT COUNT(*) FROM %s_repository %s", dbif.config.Database.TablePrefix, b.Where())
	return dbif.countQuery(sql, b.Args()...)
}

func (dbif *MysqlGitusDatabaseInterface) CountAllVisibleNamespace(username string) (int64, error) {
	return dbif.CountAllVisibleNamespaceSearchResult(username, "")
}

func (dbif *MysqlGitusDatabaseInterface) CountAllVisibleRepositories(username string) (int64, error) {
	return dbif.CountAllVisibleRepositoriesSearchResult(username, "")
}

func (dbif *MysqlGitusDatabaseInterface) SearchForUser(k string, pageNum int64, pageSize int64) ([]*model.GitusUser, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	pattern := db.ToSqlSearchPattern(k)
	stmt, err := dbif.query(ctx, fmt.Sprintf(`
SELECT user_name, user_title, user_email, user_bio, user_website, user_reg_datetime, user_password_hash, user_status
FROM %s_user
WHERE user_name LIKE $1 ESCAPE $2 OR user_title LIKE $1 ESCAPE $2
ORDER BY user_id ASC LIMIT $3 OFFSET $4
`, pfx), pattern, "\\", pageSize, pageNum*pageSize)
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make([]*model.GitusUser, 0)
	var name, title, email, bio, website, hash string
	var dt time.Time
	var st int
	for stmt.Next() {
		err = stmt.Scan(&name, &title, &email, &bio, &website, &dt, &hash, &st)
		if err != nil { return nil, err }
		res = append(res, &model.GitusUser{
			Name: name,
			Title: title,
			Email: email,
			Bio: bio,
			Website: website,
			Status: model.GitusUserStatus(st),
			PasswordHash: hash,
			RegisterTime: dt.Unix(),
		})
	}
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) SearchForNamespace(k string, pageNum int64, pageSize int64) (map[string]*model.Namespace, error) {
	ctx := context.Background()
	b := db.NewNumberedQueryBuilder()
	b.And(nsKeywordCond(b, k))
	q := fmt.Sprintf("SELECT %s FROM %s_namespace %s ORDER BY ns_absid ASC %s",
		namespaceColumns, dbif.config.Database.TablePrefix, b.Where(), b.Limit(pageSize, pageNum))
	rows, err := dbif.query(ctx, q, b.Args()...)
	if err != nil { return nil, err }
	defer rows.Close()
	return dbif.scanNamespaceRows(rows)
}

func (dbif *MysqlGitusDatabaseInterface) SearchForRepository(k string, pageNum int64, pageSize int64) ([]*model.Repository, error) {
	ctx := context.Background()
	b := db.NewNumberedQueryBuilder()
	b.And(repoKeywordCond(b, k))
	q := fmt.Sprintf("SELECT %s FROM %s_repository repo %s ORDER BY repo.repo_absid ASC %s",
		repositoryColumns, dbif.config.Database.TablePrefix, b.Where(), b.Limit(pageSize, pageNum))
	rows, err := dbif.query(ctx, q, b.Args()...)
	if err != nil { return nil, err }
	defer rows.Close()
	return dbif.scanRepositoryRows(rows)
}

func (dbif *MysqlGitusDatabaseInterface) SetNamespaceACL(nsName string, targetUserName string, acl *model.ACLTuple) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	// MySQL and MariaDB spell JSON updates differently (and MariaDB does not
	// support CAST(... AS JSON)), so read, edit and write the document in Go.
	var aclStr string
	err = queryRowTx(ctx, tx, fmt.Sprintf(`
SELECT ns_acl FROM %s_namespace WHERE ns_name = $1 FOR UPDATE
`, pfx), nsName).Scan(&aclStr)
	if err != nil { return err }
	parsed, err := model.ParseACL(aclStr)
	if err != nil { return err }
	if parsed == nil { parsed = model.NewACL() }
	if parsed.ACL == nil { parsed.ACL = make(map[string]*model.ACLTuple) }
	if acl == nil {
		delete(parsed.ACL, targetUserName)
	} else {
		parsed.ACL[targetUserName] = acl
	}
	serialized, err := parsed.SerializeACL()
	if err != nil { return err }
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_namespace SET ns_acl = $1 WHERE ns_name = $2
`, pfx), serialized, nsName)
	if err != nil { return err }
	return tx.Commit()
}

func (dbif *MysqlGitusDatabaseInterface) SetRepositoryACL(nsName string, repoName string, targetUserName string, acl *model.ACLTuple) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	var aclStr string
	err = queryRowTx(ctx, tx, fmt.Sprintf(`
SELECT repo_acl FROM %s_repository WHERE repo_namespace = $1 AND repo_name = $2 FOR UPDATE
`, pfx), nsName, repoName).Scan(&aclStr)
	if err != nil { return err }
	parsed, err := model.ParseACL(aclStr)
	if err != nil { return err }
	if parsed == nil { parsed = model.NewACL() }
	if parsed.ACL == nil { parsed.ACL = make(map[string]*model.ACLTuple) }
	if acl == nil {
		delete(parsed.ACL, targetUserName)
	} else {
		parsed.ACL[targetUserName] = acl
	}
	serialized, err := parsed.SerializeACL()
	if err != nil { return err }
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_repository SET repo_acl = $1 WHERE repo_namespace = $2 AND repo_name = $3
`, pfx), serialized, nsName, repoName)
	if err != nil { return err }
	return tx.Commit()
}

func (dbif *MysqlGitusDatabaseInterface) GetAllComprisingNamespace(username string) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.query(ctx, fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_acl, ns_status
FROM %s_namespace
WHERE ns_owner = $1 OR JSON_CONTAINS_PATH(ns_acl, 'one', CONCAT('$.acl."', $1, '"'))
`, pfx), username)
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make(map[string]*model.Namespace, 0)
	var name, title, description, email, owner, acl string
	var datetime time.Time
	var status int
	for stmt.Next() {
		err = stmt.Scan(&name, &title, &description, &email, &owner, &datetime, &acl, &status)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res[name] = &model.Namespace{
			Name: name,
			Title: title,
			Description: description,
			Email: email,
			Owner: owner,
			RegisterTime: datetime.Unix(),
			ACL: a,
			Status: model.GitusNamespaceStatus(status),
		}
	}
	return  res, nil
}

func (dbif *MysqlGitusDatabaseInterface) CountAllVisibleNamespaceSearchResult(username string, pattern string) (int64, error) {
	b := db.NewNumberedQueryBuilder()
	b.And(dbif.nsVisibilityCond(b, username))
	b.And(nsKeywordCond(b, pattern))
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s_namespace %s", dbif.config.Database.TablePrefix, b.Where())
	return dbif.countQuery(q, b.Args()...)
}

func (dbif *MysqlGitusDatabaseInterface) CountAllVisibleRepositoriesSearchResult(username string, pattern string) (int64, error) {
	b := db.NewNumberedQueryBuilder()
	b.And(dbif.repoVisibilityCond(b, username))
	b.And(repoKeywordCond(b, pattern))
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s_repository repo %s", dbif.config.Database.TablePrefix, b.Where())
	return dbif.countQuery(q, b.Args()...)
}

func (dbif *MysqlGitusDatabaseInterface) GetAllRepositoryIssue(ns string, name string) ([]*model.Issue, error) {
	ctx := context.Background()
	b := db.NewNumberedQueryBuilder()
	b.And(fmt.Sprintf("repo_namespace = %s", b.Arg(ns)))
	b.And(fmt.Sprintf("repo_name = %s", b.Arg(name)))
	q := fmt.Sprintf("SELECT %s FROM %s_issue %s ORDER BY issue_id ASC",
		issueColumns, dbif.config.Database.TablePrefix, b.Where())
	rows, err := dbif.query(ctx, q, b.Args()...)
	if err != nil { return nil, err }
	defer rows.Close()
	return dbif.scanIssueRows(rows, ns, name)
}

func (dbif *MysqlGitusDatabaseInterface) GetRepositoryIssue(ns string, name string, iid int) (*model.Issue, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT issue_absid, issue_timestamp, issue_author, issue_title, issue_content, issue_status, issue_priority
FROM %s_issue
WHERE repo_namespace = $1 AND repo_name = $2 AND issue_id = $3
`, pfx), ns, name, iid)
	var absid, id, status int64
	var t time.Time
	var priority int
	var author, title, content string
	err := stmt.Scan(&absid, &t, &author, &title, &content, &status, &priority)
	if err == sql.ErrNoRows { return nil, db.ErrEntityNotFound }
	if err != nil { return nil, err }
	return &model.Issue{
		IssueAbsId: absid,
		RepoNamespace: ns,
		RepoName: name,
		IssueId: int(id),
		IssueAuthor: author,
		IssueTitle: title,
		IssueContent: content,
		IssueTime: t.Unix(),
		IssueStatus: int(status),
		IssuePriority: priority,
	}, nil
}

func (dbif *MysqlGitusDatabaseInterface) CountAllRepositoryIssue(ns string, name string) (int, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*)
FROM %s_issue
WHERE repo_namespace = $1 AND repo_name = $2
`, pfx), ns, name)
	var res int
	err := stmt.Scan(&res)
	if err == sql.ErrNoRows { return 0, db.ErrEntityNotFound }
	if err != nil { return 0, err }
	return res, nil
}

// filterType: 0 - all, 1 - open, 2 - closed, 3 - solved, 4 - discarded
// when query = "" it looks for all issue.
func (dbif *MysqlGitusDatabaseInterface) CountIssue(query string, namespace string, name string, filterType int) (int64, error) {
	b := db.NewNumberedQueryBuilder()
	b.And(fmt.Sprintf("repo_namespace = %s", b.Arg(namespace)))
	b.And(fmt.Sprintf("repo_name = %s", b.Arg(name)))
	b.And(issueStatusCond(filterType))
	b.And(issueKeywordCond(b, query))
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s_issue %s", dbif.config.Database.TablePrefix, b.Where())
	return dbif.countQuery(q, b.Args()...)
}

func (dbif *MysqlGitusDatabaseInterface) SearchIssuePaginated(query string, namespace string, name string, filterType int, pageNum int64, pageSize int64) ([]*model.Issue, error) {
	ctx := context.Background()
	b := db.NewNumberedQueryBuilder()
	b.And(fmt.Sprintf("repo_namespace = %s", b.Arg(namespace)))
	b.And(fmt.Sprintf("repo_name = %s", b.Arg(name)))
	b.And(issueStatusCond(filterType))
	b.And(issueKeywordCond(b, query))
	q := fmt.Sprintf("SELECT %s FROM %s_issue %s ORDER BY issue_priority DESC, issue_timestamp DESC %s",
		issueColumns, dbif.config.Database.TablePrefix, b.Where(), b.Limit(pageSize, pageNum))
	rows, err := dbif.query(ctx, q, b.Args()...)
	if err != nil { return nil, err }
	defer rows.Close()
	return dbif.scanIssueRows(rows, namespace, name)
}

func (dbif *MysqlGitusDatabaseInterface) NewRepositoryIssue(ns string, name string, author string, title string, content string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx2 := context.Background()
	tx, err := dbif.begin(ctx2)
	if err != nil { return 0, err }
	defer tx.Rollback()
	newid, err := dbif.nextSequentialId(tx, ns, name, sequentialIdKindIssue, fmt.Sprintf("%s_issue", pfx), "issue_id", "repo_namespace", "repo_name")
	if err != nil { return 0, err }
	t := time.Now()
	_, err = execTx(ctx2, tx, fmt.Sprintf(`
INSERT INTO %s_issue(repo_namespace, repo_name, issue_id, issue_timestamp, issue_author, issue_title, issue_content, issue_status, issue_priority)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`, pfx), ns, name, newid, t, author, title, content, model.ISSUE_OPENED, 0)
	if err != nil { return 0, err }
	err = tx.Commit()
	if err != nil { return 0, err }
	return newid, nil
}

func (dbif *MysqlGitusDatabaseInterface) HardDeleteRepositoryIssue(ns string, name string, issueId int) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	_, err = execTx(ctx, tx, fmt.Sprintf(`
DELETE FROM %s_issue WHERE repo_namespace = $1 AND repo_name = $2 AND issue_id = $3
`, pfx), ns, name, issueId)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) SetIssuePriority(namespace string, name string, id int64, priority int) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_issue SET issue_priority = $1 WHERE issue_id = $2 AND repo_namespace = $3 AND repo_name = $4
`, pfx), priority, id, namespace, name)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) GetAllIssueEvent(ns string, name string, issueId int) ([]*model.IssueEvent, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt1 := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT issue_absid FROM %s_issue WHERE repo_namespace = $1 AND repo_name = $2 AND issue_id = $3
`, pfx), ns, name, issueId)
	var absId int64
	err := stmt1.Scan(&absId);
	if err != nil { return nil, err }
	stmt2, err := dbif.query(ctx, fmt.Sprintf(`
SELECT issue_event_absid, issue_event_type, issue_event_time, issue_event_author, issue_event_content
FROM %s_issue_event WHERE issue_absid = $1
`, pfx), absId)
	if err != nil { return nil, err }
	defer stmt2.Close()
	var etype int
	var eid int64
	var time time.Time
	var author, content string
	res := make([]*model.IssueEvent, 0)
	for stmt2.Next() {
		err = stmt2.Scan(&eid, &etype, &time, &author, &content)
		if err != nil { return nil, err }
		res = append(res, &model.IssueEvent{
			EventAbsId: eid,
			EventType: etype,
			EventTimestamp: time.Unix(),
			EventAuthor: author,
			EventContent: content,
		})
	}
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) NewRepositoryIssueEvent(ns string, name string, issueId int64, eType int, author string, content string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt1 := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT issue_absid, issue_status FROM %s_issue WHERE repo_namespace = $1 AND repo_name = $2 AND issue_id = $3
`, pfx), ns, name, issueId)
	var absId int64
	var issueStatus int
	err := stmt1.Scan(&absId, &issueStatus);
	if err != nil { return err }
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
INSERT INTO %s_issue_event(issue_absid, issue_event_type, issue_event_time, issue_event_author, issue_event_content)
VALUES ($1, $2, $3, $4, $5)
`, pfx), absId, eType, time.Now(), author, content)
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
		_, err := execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_issue SET issue_status = $1 WHERE issue_absid = $2
`, pfx), newIssueStatus, absId)
		if err != nil { return err }
	}
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) HardDeleteRepositoryIssueEvent(eventAbsId int64) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
DELETE FROM %s_issue_event WHERE issue_event_absid = $1
`, pfx), eventAbsId)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) GetAllBelongingNamespace(viewingUser string, user string) ([]*model.Namespace, error) {
	ctx := context.Background()
	b := db.NewNumberedQueryBuilder()
	b.And(dbif.belongingNamespaceCond(b, viewingUser, user))
	q := fmt.Sprintf("SELECT %s FROM %s_namespace %s ORDER BY ns_absid ASC",
		namespaceColumns, dbif.config.Database.TablePrefix, b.Where())
	rows, err := dbif.query(ctx, q, b.Args()...)
	if err != nil { return nil, err }
	defer rows.Close()
	return dbif.scanNamespaceList(rows)
}

func (dbif *MysqlGitusDatabaseInterface) GetAllBelongingRepository(viewingUser string, user string, query string, pageNum int64, pageSize int64) ([]*model.Repository, error) {
	ctx := context.Background()
	b := db.NewNumberedQueryBuilder()
	b.And(dbif.belongingRepoCond(b, viewingUser, user))
	b.And(repoKeywordCond(b, query))
	q := fmt.Sprintf("SELECT %s FROM %s_repository repo %s ORDER BY repo.repo_name ASC, repo.repo_namespace ASC %s",
		repositoryColumns, dbif.config.Database.TablePrefix, b.Where(), b.Limit(pageSize, pageNum))
	rows, err := dbif.query(ctx, q, b.Args()...)
	if err != nil { return nil, err }
	defer rows.Close()
	return dbif.scanRepositoryRows(rows)
}

func (dbif *MysqlGitusDatabaseInterface) CountAllBelongingRepository(viewingUser string, user string, query string) (int64, error) {
	b := db.NewNumberedQueryBuilder()
	b.And(dbif.belongingRepoCond(b, viewingUser, user))
	b.And(repoKeywordCond(b, query))
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s_repository repo %s", dbif.config.Database.TablePrefix, b.Where())
	return dbif.countQuery(q, b.Args()...)
}

func (dbif *MysqlGitusDatabaseInterface) GetForkRepositoryOfUser(username string, originNamespace string, originName string) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_acl, repo_status, repo_label_list, repo_absid
FROM %s_repository
WHERE repo_owner = $1 AND repo_fork_origin_namespace = $2 AND repo_fork_origin_name = $3
`, pfx), username, originNamespace, originName)
	if err != nil { return nil, err }
	defer stmt.Close()
	var ns, name, desc, acl, labelList string
	var status int
	var rowid int64
	var repoType uint8
	res := make([]*model.Repository, 0)
	for stmt.Next() {
		err = stmt.Scan(&repoType, &ns, &name, &desc, &acl, &status, &labelList, &rowid)
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

func (dbif *MysqlGitusDatabaseInterface) GetAllPullRequestPaginated(namespace string, name string, pageNum int64, pageSize int64) ([]*model.PullRequest, error) {
	ctx := context.Background()
	b := db.NewNumberedQueryBuilder()
	b.And(fmt.Sprintf("receiver_namespace = %s", b.Arg(namespace)))
	b.And(fmt.Sprintf("receiver_name = %s", b.Arg(name)))
	q := fmt.Sprintf("SELECT %s FROM %s_pull_request %s ORDER BY pull_request_id ASC %s",
		pullRequestColumns, dbif.config.Database.TablePrefix, b.Where(), b.Limit(pageSize, pageNum))
	rows, err := dbif.query(ctx, q, b.Args()...)
	if err != nil { return nil, err }
	defer rows.Close()
	return dbif.scanPullRequestRows(rows, namespace, name)
}

func (dbif *MysqlGitusDatabaseInterface) NewPullRequest(username string, title string, receiverNamespace string, receiverName string, receiverBranch string, providerNamespace string, providerName string, providerBranch string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return 0, err }
	defer tx.Rollback()
	newId, err := dbif.nextSequentialId(tx, receiverNamespace, receiverName, sequentialIdKindPullRequest, fmt.Sprintf("%s_pull_request", pfx), "pull_request_id", "receiver_namespace", "receiver_name")
	if err != nil { return 0, err }
	_, err = execTx(ctx, tx, fmt.Sprintf(`
INSERT INTO %s_pull_request(author_username, pull_request_id, title, receiver_namespace, receiver_name, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
`, pfx), username, newId, title, receiverNamespace, receiverName, receiverBranch, providerNamespace, providerName, providerBranch, new(string), nil, model.PULL_REQUEST_OPEN, time.Now())
	if err != nil { return 0, err }
	err = tx.Commit()
	if err != nil { return 0, err }
	return newId, nil
}

func (dbif *MysqlGitusDatabaseInterface) GetPullRequest(namespace string, name string, id int64) (*model.PullRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT pull_request_absid, author_username, title, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp
FROM %s_pull_request
WHERE receiver_namespace = $1 AND receiver_name = $2 AND pull_request_id = $3
`, pfx), namespace, name, id)
	var absid int64
	var author, title, receiverBranch, providerNs, providerName, providerBranch, mergeCheckString string
	var mergeConflictTime sql.NullTime
	var pullRequestTime time.Time
	var status int
	err := stmt.Scan(&absid, &author, &title, &receiverBranch, &providerNs, &providerName, &providerBranch, &mergeCheckString, &mergeConflictTime, &status, &pullRequestTime)
	if err != nil { return nil, err }
	var mergeCheckResult *gitlib.MergeCheckResult = nil
	if len(mergeCheckString) > 0 {		
		err = json.Unmarshal([]byte(mergeCheckString), &mergeCheckResult)
		if err != nil { return nil, err }
	}
	return &model.PullRequest{
		PRId: id,
		PRAbsId: absid,
		Title: title,
		Author: author,
		Timestamp: pullRequestTime.Unix(),
		ReceiverNamespace: namespace,
		ReceiverName: name,
		ReceiverBranch: receiverBranch,
		ProviderNamespace: providerNs,
		ProviderName: providerName,
		ProviderBranch: providerBranch,
		Status: status,
		MergeCheckResult: mergeCheckResult,
		MergeCheckTimestamp: nullTimeUnix(mergeConflictTime),
	}, nil
}

func (dbif *MysqlGitusDatabaseInterface) GetPullRequestByAbsId(absId int64) (*model.PullRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT author_username, pull_request_id, title, receiver_namespace, receiver_name, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp
FROM %s_pull_request
WHERE pull_request_absid = $1
`, pfx), absId)
	var id int64
	var author, title, receiverNs, receiverName, receiverBranch, providerNs, providerName, providerBranch, mergeCheckString string
	var mergeConflictTime sql.NullTime
	var pullRequestTime time.Time
	var status int
	err := stmt.Scan(&author, &id, &title, &receiverNs, &receiverName, &receiverBranch, &providerNs, &providerName, &providerBranch, &mergeCheckString, &mergeConflictTime, &status, &pullRequestTime)
	if err != nil { return nil, err }
	var mergeCheckResult *gitlib.MergeCheckResult = nil
	if len(mergeCheckString) > 0 {		
		err = json.Unmarshal([]byte(mergeCheckString), &mergeCheckResult)
		if err != nil { return nil, err }
	}
	return &model.PullRequest{
		PRId: id,
		PRAbsId: absId,
		Title: title,
		Author: author,
		Timestamp: pullRequestTime.Unix(),
		ReceiverNamespace: receiverNs,
		ReceiverName: receiverName,
		ReceiverBranch: receiverBranch,
		ProviderNamespace: providerNs,
		ProviderName: providerName,
		ProviderBranch: providerBranch,
		Status: status,
		MergeCheckResult: mergeCheckResult,
		MergeCheckTimestamp: nullTimeUnix(mergeConflictTime),
	}, nil
}

func (dbif *MysqlGitusDatabaseInterface) CheckPullRequestMergeConflict(absId int64) (*gitlib.MergeCheckResult, error) {
	// WARNING: currently only works when when the source &
	// the target is git repo. currently (2025.8.27) this check
	// is performed at the controller side, i.e. users cannot
	// create pull request if the repo is not git repo, but the
	// code can still be called. DO NOT CALL UNLESS YOU KNOW
	// WHAT YOU'RE DOING.
	// TODO: fix this after figuring things out.
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT receiver_namespace, receiver_name, receiver_branch, provider_namespace, provider_name, provider_branch
FROM %s_pull_request
WHERE pull_request_absid = $1
`, pfx), absId)
	var receiverNamespace, receiverName, receiverBranch string
	var providerNamespace, providerName, providerBranch string
	err := stmt.Scan(&receiverNamespace, &receiverName, &receiverBranch, &providerNamespace, &providerName, &providerBranch)
	if err != nil { return nil, err }
	tx, err := dbif.begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback()
	p := path.Join(dbif.config.GitRoot, receiverNamespace, receiverName)
	lgr := gitlib.NewLocalGitRepository(p)
	remoteName := fmt.Sprintf("%s/%s", providerNamespace, providerName)
	mr, err := lgr.CheckBranchMergeConflict(receiverBranch, remoteName, providerBranch)
	if err != nil { return nil, err }
	mrstr, err := json.Marshal(mr)
	if err != nil { return nil, err }
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_pull_request
SET merge_conflict_check_result = $2, merge_conflict_check_timestamp = $3
WHERE pull_request_absid = $1
`, pfx), absId, string(mrstr), time.Now())
	if err != nil { return nil, err }
	err = tx.Commit()
	if err != nil { return nil, err }
	return mr, nil
}

func (dbif *MysqlGitusDatabaseInterface) DeletePullRequest(absId int64) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
DELETE FROM %s_pull_request WHERE pull_request_absid = $1
`, pfx), absId)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) GetAllPullRequestEventPaginated(absId int64, pageNum int64, pageSize int64) ([]*model.PullRequestEvent, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.query(ctx, fmt.Sprintf(`
SELECT event_type, event_timestamp, event_author, event_content
FROM %s_pull_request_event
WHERE pull_request_absid = $1
ORDER BY event_timestamp ASC LIMIT $2 OFFSET $3
`, pfx), absId, pageSize, pageNum*pageSize)
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make([]*model.PullRequestEvent, 0)
	var etype int
	var timestamp time.Time
	var author, content string
	for stmt.Next() {
		err = stmt.Scan(&etype, &timestamp, &author, &content)
		if err != nil { return nil, err }
		res = append(res, &model.PullRequestEvent{
			PRAbsId: absId,
			EventType: etype,
			EventTimestamp: timestamp.Unix(),
			EventAuthor: author,
			EventContent: content,
		})
	}
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) CheckAndMergePullRequest(absId int64, username string) error {
	// WARNING: currently only works when when the source &
	// the target is git repo. currently (2025.8.27) this check
	// is performed at the controller side, i.e. users cannot
	// create pull request if the repo is not git repo, but the
	// code can still be called. DO NOT CALL UNLESS YOU KNOW
	// WHAT YOU'RE DOING.
	// TODO: fix this after figuring things out. (doing the
	// following possibly bad for performance?) this would
	// need to be fixed in the future...
	r, err := dbif.CheckPullRequestMergeConflict(absId)
	if err != nil { return err }
	if !r.Successful { return nil }
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt0 := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT user_email, user_title FROM %s_user WHERE user_name = $1
`, pfx), username)
	var email, userTitle string
	err = stmt0.Scan(&email, &userTitle)
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
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	t := time.Now()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_pull_request SET pull_request_status = $1, pull_request_timestamp = $2 WHERE pull_request_absid = $3
`, pfx), model.PULL_REQUEST_CLOSED_AS_MERGED, t, absId)
	if err != nil { return err }
	_, err = execTx(ctx, tx, fmt.Sprintf(`
INSERT INTO %s_pull_request_event(pull_request_abs_id, event_type, event_timestamp, event_author, event_content)
VALUES ($1,$2,$3,$4,$5)
`, pfx), absId, model.PULL_REQUEST_EVENT_CLOSE_AS_MERGED, t, username, "")
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) CommentOnPullRequest(absId int64, author string, content string) (*model.PullRequestEvent, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback()
	t := time.Now()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
INSERT INTO %s_pull_request_event(pull_request_absid, event_type, event_timestamp, event_author, event_content) VALUES ($1,$2,$3,$4,$5)
`, pfx), absId, model.PULL_REQUEST_EVENT_COMMENT, t, author, content)
	if err != nil { return nil, err }
	err = tx.Commit()
	if err != nil { return nil, err }
	return &model.PullRequestEvent{
		PRAbsId: absId,
		EventType: model.PULL_REQUEST_EVENT_COMMENT,
		EventTimestamp: t.Unix(),
		EventAuthor: author,
		EventContent: content,
	}, nil
}

func (dbif *MysqlGitusDatabaseInterface) CommentOnPullRequestCode(absId int64, comment *model.PullRequestCommentOnCode) (*model.PullRequestEvent, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback()
	t := time.Now()
	contentBytes, _ := json.Marshal(comment)
	contentString := string(contentBytes)
	_, err = execTx(ctx, tx, fmt.Sprintf(`
INSERT INTO %s_pull_request_event(pull_request_absid, event_type, event_timestamp, event_author, event_content)
VALUES ($1,$2,$3,$4,$5)
`, pfx), absId, model.PULL_REQUEST_EVENT_COMMENT_ON_CODE, t, comment.Username, contentString)
	if err != nil { return nil, err }
	err = tx.Commit()
	if err != nil { return nil, err }
	return &model.PullRequestEvent{
		PRAbsId: absId,
		EventType: model.PULL_REQUEST_EVENT_COMMENT_ON_CODE,
		EventTimestamp: t.Unix(),
		EventAuthor: comment.Username,
		EventContent: contentString,
	}, nil
}

func (dbif *MysqlGitusDatabaseInterface) ClosePullRequestAsNotMerged(absid int64, author string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	t := time.Now()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
INSERT INTO %s_pull_request_event(pull_request_absid, event_type, event_timestamp, event_author, event_content)
VALUES ($1,$2,$3,$4,$5)
`, pfx), absid, model.PULL_REQUEST_EVENT_CLOSE_AS_NOT_MERGED, t, author, new(string))
	if err != nil { return err }
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_pull_request
SET pull_request_status = $1
WHERE pull_request_absid = $2
`, pfx), model.PULL_REQUEST_CLOSED_AS_NOT_MERGED, absid)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) ReopenPullRequest(absid int64, author string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	t := time.Now()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
INSERT INTO %s_pull_request_event(pull_request_absid, event_type, event_timestamp, event_author, event_content)
VALUES ($1,$2,$3,$4,$5)
`, pfx), absid, model.PULL_REQUEST_EVENT_REOPEN, t, author, new(string))
	if err != nil { return err }
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_pull_request
SET pull_request_status = $1
WHERE pull_request_absid = $2
`, pfx), model.PULL_REQUEST_OPEN, absid)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) CountPullRequest(query string, namespace string, name string, filterType int) (int64, error) {
	b := db.NewNumberedQueryBuilder()
	b.And(fmt.Sprintf("receiver_namespace = %s", b.Arg(namespace)))
	b.And(fmt.Sprintf("receiver_name = %s", b.Arg(name)))
	b.And(pullRequestStatusCond(filterType))
	b.And(pullRequestKeywordCond(b, query))
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s_pull_request %s", dbif.config.Database.TablePrefix, b.Where())
	return dbif.countQuery(q, b.Args()...)
}

func (dbif *MysqlGitusDatabaseInterface) SearchPullRequestPaginated(query string, namespace string, name string, filterType int, pageNum int64, pageSize int64) ([]*model.PullRequest, error) {
	ctx := context.Background()
	b := db.NewNumberedQueryBuilder()
	b.And(fmt.Sprintf("receiver_namespace = %s", b.Arg(namespace)))
	b.And(fmt.Sprintf("receiver_name = %s", b.Arg(name)))
	b.And(pullRequestStatusCond(filterType))
	b.And(pullRequestKeywordCond(b, query))
	q := fmt.Sprintf("SELECT %s FROM %s_pull_request %s ORDER BY pull_request_timestamp DESC %s",
		pullRequestColumns, dbif.config.Database.TablePrefix, b.Where(), b.Limit(pageSize, pageNum))
	rows, err := dbif.query(ctx, q, b.Args()...)
	if err != nil { return nil, err }
	defer rows.Close()
	return dbif.scanPullRequestRows(rows, namespace, name)
}

func (dbif *MysqlGitusDatabaseInterface) GetAllRegisteredEmailOfUser(username string) ([]struct{Email string;Verified bool}, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.query(ctx, fmt.Sprintf(`
SELECT email, email_verified FROM %s_user_email WHERE username = $1
`, pfx), username)
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make([]struct{Email string;Verified bool}, 0)
	var email string
	var verified int
	for stmt.Next() {
		err = stmt.Scan(&email, &verified)
		if err != nil { return nil, err }
		res = append(res, struct{Email string;Verified bool}{
			Email: email,
			Verified: verified == 1,
		})
	}
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) AddEmail(username string, email string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
INSERT INTO %s_user_email(username, email, email_verified) VALUES ($1, $2, 0)
`, pfx), username, email)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) VerifyRegisteredEmail(username string, email string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_user_email SET email_verified = 1 WHERE username = $1 AND email = $2
`, pfx), username, email)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) DeleteRegisteredEmail(username string, email string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
DELETE FROM %s_user_email WHERE username = $1 AND email = $2
`, pfx), username, email)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) CheckIfEmailVerified(username string, email string) (bool, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT email_verified FROM %s_user_email WHERE username = $1 AND email = $2
`, pfx), username, email)
	var r int
	err := stmt.Scan(&r)
	if err != nil { return false, err }
	return r == 1, nil
}

func (dbif *MysqlGitusDatabaseInterface) ResolveEmailToUsername(email string) (string, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT username FROM %s_user_email WHERE email = $1 AND email_verified = 1
`, pfx), email)
	var r string
	err := stmt.Scan(&r)
	if err != nil { return "", err }
	return r, nil
}

func (dbif *MysqlGitusDatabaseInterface) ResolveMultipleEmailToUsername(emailList map[string]string) (map[string]string, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	l := make([]any, 0)
	q := make([]string, 0)
	i := 1
	for k := range emailList {
		l = append(l, k)
		q = append(q, fmt.Sprintf("$%d", i))
		i += 1
	}
	stmt, err := dbif.query(ctx, fmt.Sprintf(`
SELECT email, username FROM %s_user_email WHERE email_verified = 1 AND email IN (%s)
`, pfx, strings.Join(q, ",")), l...)
	if err != nil { return nil, err }
	defer stmt.Close()
	var email, username string
	for stmt.Next() {
		err = stmt.Scan(&email, &username)
		if err != nil { return nil, err }
		emailList[email] = username
	}
	return emailList, nil
}

func (dbif *MysqlGitusDatabaseInterface) InsertRegistrationRequest(username string, email string, passwordHash string, reason string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
INSERT INTO %s_user_reg_request(username, email, password_hash, reason, timestamp) VALUES ($1,$2,$3,$4,$5)
`, pfx), username, email, passwordHash, reason, time.Now())
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) ApproveRegistrationRequest(absid int64) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt1 := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT username, email, password_hash FROM %s_user_reg_request
WHERE request_absid = $1
`, pfx), absid)
	var username, email, passwordHash string
	err := stmt1.Scan(&username, &email, &passwordHash)
	if err != nil { return err }
	if dbif.config.EmailConfirmationRequired {
		_, err = dbif.RegisterUser(username, email, passwordHash, model.NORMAL_USER_CONFIRM_NEEDED)
		if err != nil { return err }
	} else {
		_, err = dbif.RegisterUser(username, email, passwordHash, model.NORMAL_USER)
		if err != nil { return err }
	}
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
DELETE FROM %s_user_reg_request WHERE username = $1
`, pfx), username)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) DisapproveRegistrationRequest(absid int64) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT username FROM %s_user_reg_request
WHERE request_absid = $1
`, pfx), absid)
	var username string
	err := stmt.Scan(&username)
	if err != nil { return err }
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
DELETE FROM %s_user_reg_request WHERE username = $1
`, pfx), username)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) GetRegistrationRequestPaginated(pageNum int64, pageSize int64) ([]*model.RegistrationRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.query(ctx, fmt.Sprintf(`
SELECT request_absid, username, email, password_hash, reason, timestamp
FROM %s_user_reg_request
ORDER BY timestamp DESC LIMIT $1 OFFSET $2
`, pfx), pageSize, pageNum*pageSize)
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make([]*model.RegistrationRequest, 0)
	var username, email, passwordHash, reason string
	var timestamp time.Time
	var absid int64
	for stmt.Next() {
		err = stmt.Scan(&absid, &username, &email, &passwordHash, &reason, &timestamp)
		if err != nil { return nil, err }
		res = append(res, &model.RegistrationRequest{
			AbsId: absid,
			Username: username,
			Email: email,
			PasswordHash: passwordHash,
			Reason: reason,
			Timestamp: timestamp,
		})
	}
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) GetRequestOfUsernamePaginated(username string, pageNum int64, pageSize int64) ([]*model.RegistrationRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.query(ctx, fmt.Sprintf(`
SELECT request_absid, email, password_hash, reason, timestamp
FROM %s_user_reg_request
WHERE username = $1
ORDER BY timestamp DESC LIMIT $2 OFFSET $3
`, pfx), username, pageSize, pageNum*pageSize)
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make([]*model.RegistrationRequest, 0)
	var absid int64
	var email, passwordHash, reason string
	var timestamp time.Time
	for stmt.Next() {
		err = stmt.Scan(&absid, &email, &passwordHash, &reason, &timestamp)
		if err != nil { return nil, err }
		res = append(res, &model.RegistrationRequest{
			AbsId: absid,
			Username: username,
			Email: email,
			PasswordHash: passwordHash,
			Reason: reason,
			Timestamp: timestamp,
		})
	}
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) CountRegistrationRequest(query string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var r *sql.Row
	var err error
	query = strings.TrimSpace(query)
	if len(query) <= 0 {
		r = dbif.queryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_user_reg_request
`, pfx))
	} else {
		r = dbif.queryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_user_reg_request WHERE username LIKE $1 ESCAPE $2
`, pfx), db.ToSqlSearchPattern(query), "\\")
	}
	var cnt int64
	err = r.Scan(&cnt)
	if err != nil { return 0, err }
	return cnt, nil
}

func (dbif *MysqlGitusDatabaseInterface) SearchRegistrationRequestPaginated(query string, pageNum int64, pageSize int64) ([]*model.RegistrationRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var stmt *sql.Rows
	var err error
	query = strings.TrimSpace(query)
	if len(query) <= 0 {
		stmt, err = dbif.query(ctx, fmt.Sprintf(`
SELECT request_absid, username, email, password_hash, timestamp FROM %s_user_reg_request ORDER BY timestamp DESC LIMIT $1 OFFSET $2 
`, pfx), pageSize, pageNum*pageSize)
	} else {
		stmt, err = dbif.query(ctx, fmt.Sprintf(`
SELECT request_absid, username, email, password_hash, timestamp FROM %s_user_reg_request WHERE username  LIKE $1 ESCAPE $2 ORDER BY timestamp DESC LIMIT $3 OFFSET $4
`, pfx), db.ToSqlSearchPattern(query), "\\", pageSize, pageNum*pageSize)
	}
	if err != nil { return nil, err }
	defer stmt.Close()
	var absid int64
	var username, email, passwordHash string
	var timestamp time.Time
	res := make([]*model.RegistrationRequest, 0)
	for stmt.Next() {
		err := stmt.Scan(&absid, &username, &email, &passwordHash, &timestamp)
		if err != nil { return nil, err }
		res = append(res, &model.RegistrationRequest{
			AbsId: absid,
			Username: username,
			Email: email,
			PasswordHash: passwordHash,
			Timestamp: timestamp,
		})
	}
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) GetRegistrationRequestByAbsId(absid int64) (*model.RegistrationRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT request_absid, username, email, password_hash, timestamp FROM %s_user_reg_request WHERE request_absid = $1
`, pfx), absid)
	var rowid int64
	var timestamp time.Time
	var username, email, passwordHash string
	err := stmt.Scan(&rowid, &username, &email, &passwordHash, &timestamp)
	if err != nil { return nil, err }
	return &model.RegistrationRequest{
		AbsId: rowid,
		Username: username,
		Email: email,
		PasswordHash: passwordHash,
		Timestamp: timestamp,
	}, nil
}

func (dbif *MysqlGitusDatabaseInterface) AddRepositoryLabel(ns string, name string, lbl string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT repo_label_list FROM %s_repository WHERE repo_namespace = $1 AND repo_name = $2
`, pfx), ns, name)
	var rll string
	err := stmt.Scan(&rll)
	if err != nil { return err }
	var tags []string
	if len(rll) <= 0 {
		tags = make([]string, 0)
	} else {
		tags = strings.Split(rll[1:len(rll)-1], "}{")
		if slices.Contains(tags, lbl) { return nil }
	}
	tags = append(tags, lbl)
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_repository SET repo_label_list = $1 WHERE repo_namespace = $2 AND repo_name = $3
`, pfx), fmt.Sprintf("{%s}", strings.Join(tags, "}{")), ns, name)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) RemoveRepositoryLabel(ns string, name string, lbl string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT repo_label_list FROM %s_repository WHERE repo_namespace = $1 AND repo_name = $2
`, pfx), ns, name)
	var rll string
	err := stmt.Scan(&rll)
	if err != nil { return err }
	tags := strings.Split(rll[1:len(rll)-1], "}{")
	idx := slices.Index(tags, lbl)
	if idx == -1 { return nil }
	tags = slices.Delete(tags, idx, idx+1)
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_repository SET repo_label_list = $1 WHERE repo_namespace = $2 AND repo_name = $3
`, pfx), fmt.Sprintf("{%s}", strings.Join(tags, "}{")), ns, name)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) GetRepositoryLabel(ns string, name string) ([]string, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT repo_label_list FROM %s_repository WHERE repo_namespace = $1 and repo_name = $2
`, pfx), ns, name)
	var rll string
	err := stmt.Scan(&rll)
	if err != nil { return nil, err }
	tags := strings.Split(rll[1:len(rll)-1], "}{")
	return tags, nil
}

func (dbif *MysqlGitusDatabaseInterface) CountRepositoryWithLabel(username string, label string) (int64, error) {
	b := db.NewNumberedQueryBuilder()
	b.And(repoLabelCond(b, label))
	b.And(dbif.repoVisibilityCond(b, username))
	q := fmt.Sprintf("SELECT COUNT(*) FROM %s_repository repo %s", dbif.config.Database.TablePrefix, b.Where())
	return dbif.countQuery(q, b.Args()...)
}

func (dbif *MysqlGitusDatabaseInterface) GetRepositoryWithLabelPaginated(username string, label string, pageNum int64, pageSize int64) ([]*model.Repository, error) {
	ctx := context.Background()
	b := db.NewNumberedQueryBuilder()
	b.And(repoLabelCond(b, label))
	b.And(dbif.repoVisibilityCond(b, username))
	q := fmt.Sprintf("SELECT %s FROM %s_repository repo %s ORDER BY repo.repo_name ASC, repo.repo_namespace ASC %s",
		repositoryColumns, dbif.config.Database.TablePrefix, b.Where(), b.Limit(pageSize, pageNum))
	rows, err := dbif.query(ctx, q, b.Args()...)
	if err != nil { return nil, err }
	defer rows.Close()
	return dbif.scanRepositoryRows(rows)
}


func (dbif *MysqlGitusDatabaseInterface) NewSnippet(username string, name string, status uint8) (*model.Snippet, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback()
	t := time.Now()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
INSERT INTO %s_snippet(name, username, description, timestamp, status, shared_user)
VALUES ($1, $2, $3, $4, $5, $6)
`, pfx), name, username, new(string), t, status, "{}")
	if err != nil { return nil, err }
	p := path.Join(dbif.config.SnippetRoot, username, name)
	if !db.IsSubDir(dbif.config.SnippetRoot, p) {
		return nil, db.ErrInvalidLocation
	}
	err = os.RemoveAll(p)
	if err != nil { return nil, err }
	err = os.MkdirAll(p, os.ModeDir|0755)
	if err != nil { return nil, err }
	err = tx.Commit()
	if err != nil { return nil, err }
	return &model.Snippet{
		Name: name,
		BelongingUser: username,
		Description: "",
		Time: t.Unix(),
		FileList: make(map[string]string, 0),
		Status: status,
	}, nil
}

func (dbif *MysqlGitusDatabaseInterface) GetAllSnippet(username string) ([]*model.Snippet, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.query(ctx, fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = $1
`, pfx), username)
	if err != nil { return nil, err }
	var name, desc string
	var status uint8
	var timestamp time.Time
	var sharedUserStr string
	res := make([]*model.Snippet, 0)
	for stmt.Next() {
		err = stmt.Scan(&name, &desc, &timestamp, &status, &sharedUserStr)
		if err != nil { return nil, err }
		var sharedUser map[string]bool
		if len(sharedUserStr) > 0 {
			if err = json.Unmarshal([]byte(sharedUserStr), &sharedUser); err != nil { return nil, err }
		}
		res = append(res, &model.Snippet{
			Name: name,
			BelongingUser: username,
			Description: desc,
			Status: status,
			Time: timestamp.Unix(),
			FileList: nil,
			SharedUser: sharedUser,
		})
	}
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) CountAllVisibleSnippet(username string, viewingUser string, query string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var stmt *sql.Row
	var err error
	if len(query) > 0 {
		q := db.ToSqlSearchPattern(query)
		if len(viewingUser) <= 0 {
			stmt = dbif.queryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_snippet
WHERE username = $1 AND (status = 1) AND name LIKE $2 ESCAPE $3
`, pfx), username, q, "\\")
		} else if viewingUser == username {
			stmt = dbif.queryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_snippet
WHERE username = $1 AND name LIKE $2 ESCAPE $3
`, pfx), username, q, "\\")
		} else {
			stmt = dbif.queryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_snippet
WHERE username = $1 AND (status = 1 OR status = 2 OR (status = 4 AND JSON_CONTAINS_PATH(shared_user, 'one', CONCAT('$."', $2, '"')))) AND name LIKE $3 ESCAPE $4
`, pfx), username, viewingUser, q, "\\")
		}
	} else {
		if len(viewingUser) <= 0 {
			stmt = dbif.queryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_snippet
WHERE username = $1 AND (status = 1 OR status = 2)
`, pfx), username)
		} else if viewingUser == username {
			stmt = dbif.queryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_snippet
WHERE username = $1
`, pfx), username)
		} else {
			stmt = dbif.queryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_snippet
WHERE username = $1 AND (status = 1 OR status = 2 OR (status = 4 AND JSON_CONTAINS_PATH(shared_user, 'one', CONCAT('$."', $2, '"'))))
`, pfx), username, viewingUser)
		}
	}
	var res int64
	err = stmt.Scan(&res)
	if err != nil { return 0, err }
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) GetAllVisibleSnippetPaginated(username string, viewingUser string, query string, pageNum int64, pageSize int64) ([]*model.Snippet, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var stmt *sql.Rows
	var err error
	if len(query) > 0 {
		q := db.ToSqlSearchPattern(query)
		if len(viewingUser) <= 0 {
			stmt, err = dbif.query(ctx, fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = $1 AND (status = 1) AND name LIKE $2 ESCAPE $3
`, pfx), username, q, "\\")
		} else if viewingUser == username {
			stmt, err = dbif.query(ctx, fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = $1 AND name LIKE $2 ESCAPE $3
`, pfx), username, q, "\\")
		} else {
			stmt, err = dbif.query(ctx, fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = $1 AND (status = 1 OR status = 2 OR (status = 4 AND JSON_CONTAINS_PATH(shared_user, 'one', CONCAT('$."', $2, '"')))) AND name LIKE $3 ESCAPE $4
`, pfx), username, viewingUser, q, "\\")
		}
	} else {
		if len(viewingUser) <= 0 {
			stmt, err = dbif.query(ctx, fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = $1 AND (status = 1 OR status = 2)
`, pfx), username)
		} else if viewingUser == username {
			stmt, err = dbif.query(ctx, fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = $1
`, pfx), username)
		} else {
			stmt, err = dbif.query(ctx, fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = $1 AND (status = 1 OR status = 2 OR (status = 4 AND JSON_CONTAINS_PATH(shared_user, 'one', CONCAT('$."', $2, '"'))))
`, pfx), username, viewingUser)
		}
	}
	if err != nil { return nil, err }
	var name, desc string
	var status uint8
	var timestamp time.Time
	var sharedUserStr string
	res := make([]*model.Snippet, 0)
	for stmt.Next() {
		err = stmt.Scan(&name, &desc, &timestamp, &status, &sharedUserStr)
		if err != nil { return nil, err }
		var sharedUser map[string]bool
		if len(sharedUserStr) > 0 {
			if err = json.Unmarshal([]byte(sharedUserStr), &sharedUser); err != nil { return nil, err }
		}
		res = append(res, &model.Snippet{
			Name: name,
			BelongingUser: username,
			Description: desc,
			Status: status,
			Time: timestamp.Unix(),
			FileList: nil,
			SharedUser: sharedUser,
		})
	}
	return res, nil
}

func (dbif *MysqlGitusDatabaseInterface) DeleteSnippet(username string, name string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
DELETE FROM %s_snippet WHERE username = $1 AND name = $2
`, pfx), username, name)
	if err != nil { return err }
	p := path.Join(dbif.config.SnippetRoot, username, name)
	err = os.RemoveAll(p)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) SaveSnippetInfo(m *model.Snippet) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_snippet
SET description = $1, status = $2, shared_user = $3
WHERE username = $4 AND name = $5
`, pfx), m.Description, m.Status, m.SharedUser, m.BelongingUser, m.Name)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) GetSnippet(username string, name string) (*model.Snippet, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT description, timestamp, status, shared_user FROM %s_snippet
WHERE username = $1 AND name = $2
`, pfx), username, name)
	var desc, shareduser string
	var timestamp time.Time
	var status uint8
	err := stmt.Scan(&desc, &timestamp, &status, &shareduser)
	if err != nil { return nil, err }
	var su map[string]bool
	err = json.Unmarshal([]byte(shareduser), &su)
	if err != nil { return nil, err }
	return &model.Snippet{
		Name: name,
		BelongingUser: username,
		Description: desc,
		Time: timestamp.Unix(),
		Status: status,
		FileList: nil,
		SharedUser: su,
	}, nil
}


func (dbif *MysqlGitusDatabaseInterface) RegisterWebhookRequest(uuid string, reportUuid string, repoNs string, repoName string, commitId string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	d := new(model.WebhookResult)
	d.Status = model.WEBHOOK_RESULT_UNDEFINED
	d.ReportUUID = reportUuid
	d.UUID = uuid
	d.RepoNamespace = repoNs
	d.RepoName = repoName
	_, err = execTx(ctx, tx, fmt.Sprintf(`
INSERT INTO %s_webhook_log(uuid, repo_namespace, repo_name, commit_id, webhook_result)
VALUES ($1,$2,$3,$4,$5)
`, pfx), uuid, repoNs, repoName, commitId, d)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) UpdateWebhookResult(uuid string, result *model.WebhookResult) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.begin(ctx)
	if err != nil { return err }
	defer tx.Rollback()
	if result.Timestamp == 0 { result.Timestamp = time.Now().Unix() }
	_, err = execTx(ctx, tx, fmt.Sprintf(`
UPDATE %s_webhook_log SET webhook_result = $1 WHERE uuid = $2
`, pfx), result, uuid)
	if err != nil { return err }
	err = tx.Commit()
	if err != nil { return err }
	return nil
}

func (dbif *MysqlGitusDatabaseInterface) GetWebhookResultByUUID(uuid string) (*model.WebhookResult, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.queryRow(ctx, fmt.Sprintf(`
SELECT webhook_result
FROM %s_webhook_log
WHERE uuid = $1
`, pfx), uuid)
	var webhookRes string
	if err := stmt.Scan(&webhookRes); err != nil { return nil, err }
	res := new(model.WebhookResult)
	if err := json.Unmarshal([]byte(webhookRes), res); err != nil { return nil, err }
	return res, nil
}



// kinds used by id_counter (see nextSequentialId).
const (
	sequentialIdKindIssue = 1
	sequentialIdKindPullRequest = 2
)

// nextSequentialId hands out the next id of an issue or pull request of a
// repository. It replaces SELECT COUNT(*) + 1, which handed the same id to two
// concurrent creations and rescanned the table every time.
//
// The counter row is created on first use with MAX(existing)+1, so databases
// that already contain issues continue right after the last one. The upsert is
// a single statement, which is what makes it safe under concurrency.
func (dbif *MysqlGitusDatabaseInterface) nextSequentialId(tx *sql.Tx, ns string, name string, kind int, table string, idColumn string, nsColumn string, nameColumn string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	// MySQL has neither ON CONFLICT ... RETURNING nor INSERT ... RETURNING, so
	// the counter is bumped in place and read back in the same transaction.
	// The upsert locks the counter row, which is what serialises concurrent
	// creations of the same kind.
	_, err := execTx(context.Background(), tx, fmt.Sprintf(`
INSERT INTO %s_id_counter(repo_namespace, repo_name, kind, next_id)
VALUES ($1, $2, $3, (SELECT COALESCE(MAX(%s), 0) + 1 FROM %s WHERE %s = $4 AND %s = $5))
ON DUPLICATE KEY UPDATE next_id = next_id + 1
`, pfx, idColumn, table, nsColumn, nameColumn), ns, name, kind, ns, name)
	if err != nil { return 0, err }
	var next int64
	err = queryRowTx(context.Background(), tx, fmt.Sprintf(`
SELECT next_id FROM %s_id_counter
WHERE repo_namespace = $1 AND repo_name = $2 AND kind = $3
FOR UPDATE
`, pfx), ns, name, kind).Scan(&next)
	if err != nil { return 0, err }
	return next, nil
}
