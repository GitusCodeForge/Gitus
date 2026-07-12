package postgres

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
	pgx "github.com/jackc/pgx/v5"
)

func (dbif *PostgresGitusDatabaseInterface) GetUserByName(name string) (*model.GitusUser, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT user_title, user_email, user_bio, user_website, user_reg_datetime, user_password_hash, user_status, user_2fa_config, user_website_preference
FROM %s_user
WHERE user_name = $1
`, pfx), name)
	var title, email, bio, website, password string
	var tfa model.GitusUser2FAConfig
	var wPref model.GitusUserWebsitePreference
	var datetime time.Time
	var status int
	err := stmt.Scan(&title, &email, &bio, &website, &datetime, &password, &status, &tfa, &wPref)
	if errors.Is(err, pgx.ErrNoRows) { return nil, db.ErrEntityNotFound }
	if err != nil { return nil, err }
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

func (dbif *PostgresGitusDatabaseInterface) GetAllAuthKeyByUsername(name string) ([]model.GitusAuthKey, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
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

func (dbif *PostgresGitusDatabaseInterface) GetAuthKeyByName(userName string, keyName string) (*model.GitusAuthKey, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT key_text FROM %s_user_authkey WHERE user_name = $1 AND key_name = $2
`, pfx), userName, keyName)
	var ktext string
	err := stmt.Scan(&ktext)
	if errors.Is(err, pgx.ErrNoRows) { return nil, db.ErrEntityNotFound }
	if err != nil { return nil, err }
	return &model.GitusAuthKey{
		UserName: userName,
		KeyName: keyName,
		KeyText: ktext,
	}, nil
}

func (dbif *PostgresGitusDatabaseInterface) RegisterAuthKey(username string, keyname string, keytext string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s_user_authkey(user_name, key_name, key_text)
VALUES ($1, $2, $3)
`, pfx), username, keyname, keytext)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) UpdateAuthKey(username string, keyname string, keytext string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_user_authkey
SET key_text = $1
WHERE user_name = $2 AND key_name = $3
`, pfx), keytext, username, keyname)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) RemoveAuthKey(username string, keyname string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
DELETE FROM %s_user_authkey
WHERE user_name = $1 AND key_name = $2
`, pfx), username, keyname)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) GetAllSignKeyByUsername(name string) ([]model.GitusSigningKey, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
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

func (dbif *PostgresGitusDatabaseInterface) GetSignKeyByName(userName string, keyName string) (*model.GitusSigningKey, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT key_text FROM %s_user_signkey WHERE user_name = $1 AND key_name = $2
`, pfx), userName, keyName)
	var ktext string
	err := stmt.Scan(&ktext)
	if errors.Is(err, pgx.ErrNoRows) { return nil, db.ErrEntityNotFound }
	if err != nil { return nil, err }
	return &model.GitusSigningKey{
		UserName: userName,
		KeyName: keyName,
		KeyText: ktext,
	}, nil
}

func (dbif *PostgresGitusDatabaseInterface) UpdateSignKey(username string, keyname string, keytext string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_user_signkey
SET key_text = $1
WHERE user_name = $2 AND key_name = $3
`, pfx), keytext, username, keyname)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) RegisterSignKey(username string, keyname string, keytext string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s_user_signkey(user_name, key_name, key_text)
VALUES ($1, $2, $3)
`, pfx), username, keyname, keytext)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) RemoveSignKey(username string, keyname string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
DELETE FROM %s_user_signkey
WHERE user_name = $1 AND key_name = $2
`, pfx), username, keyname)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) GetNamespaceByName(name string) (*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_acl, ns_status
FROM %s_namespace
WHERE ns_name = $1
`, pfx), name)
	var title, description, email, owner, acl string
	var datetime time.Time
	var status int
	err := stmt.Scan(&title, &description, &email, &owner, &datetime, &acl, &status)
	if errors.Is(err, pgx.ErrNoRows) { return nil, db.ErrEntityNotFound }
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

func (dbif *PostgresGitusDatabaseInterface) GetRepositoryByName(nsName string, repoName string) (*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT repo_type, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook, repo_absid
FROM %s_repository
WHERE repo_namespace = $1 AND repo_name = $2
`, pfx), nsName, repoName)
	var description, owner, acl, forkOriginNamespace, forkOriginName, webhook string
	var repoType, repoStatus int
	var rowid int64
	var labelList string
	err := stmt.Scan(&repoType, &description, &owner, &acl, &repoStatus, &forkOriginNamespace, &forkOriginName, &labelList, &webhook, &rowid)
	if errors.Is(err, pgx.ErrNoRows) { return nil, db.ErrEntityNotFound }
	if err != nil { return nil, err }
	p := path.Join(dbif.config.GitRoot, nsName, repoName)
	localRepo, err := model.CreateLocalRepository(uint8(repoType), nsName, repoName, p)
	if err != nil { return nil, err }
	res, err := model.NewRepository(nsName, repoName, localRepo)
	if err != nil { return nil, err }
	res.AbsId = rowid
	res.Type = uint8(repoType)
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

func (dbif *PostgresGitusDatabaseInterface) GetAllNamespace() (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
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

func (dbif *PostgresGitusDatabaseInterface) GetAllVisibleNamespace(username string) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_acl, ns_status
FROM %s_namespace
WHERE ns_owner = $1 OR ns_acl->'ACL' ? $2
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

func (dbif *PostgresGitusDatabaseInterface) GetAllVisibleNamespacePaginated(username string, pageNum int64, pageSize int64) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var stmt pgx.Rows
	var err error
	if len(username) > 0 {
		stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_acl, ns_status
FROM %s_namespace
WHERE ns_status = 1 OR ns_owner = $3 OR ns_acl->'ACL' ? $3
ORDER BY ns_absid ASC LIMIT $1 OFFSET $2
`, pfx), pageSize, pageNum*pageSize, username)
	} else {
		stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_acl, ns_status
FROM %s_namespace
WHERE ns_status = 1
ORDER BY ns_absid ASC LIMIT $1 OFFSET $2
`, pfx), pageSize, pageNum*pageSize)
	}
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

func (dbif *PostgresGitusDatabaseInterface) SearchAllVisibleNamespacePaginated(username string, query string, pageNum int64, pageSize int64) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var stmt pgx.Rows
	var err error
	if len(username) > 0 {
		if len(query) > 0 {
			stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_acl, ns_status
FROM %s_namespace
WHERE (ns_name LIKE $1 ESCAPE $2) AND (ns_status = 1 OR ns_owner = $3 OR ns_acl->'ACL' ? $3)
ORDER BY ns_absid ASC LIMIT $4 OFFSET $5
`, pfx), db.ToSqlSearchPattern(query), "%", username, pageSize, pageNum*pageSize)
		} else {
			stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_acl, ns_status
FROM %s_namespace
WHERE (ns_status = 1 OR ns_owner = $1 OR ns_acl->'ACL' ? $1)
ORDER BY ns_absid ASC LIMIT $2 OFFSET $3
`, pfx), username, pageSize, pageNum*pageSize)
		}
	} else {
		if len(query) > 0 {
			stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_acl, ns_status
FROM %s_namespace
WHERE (ns_name LIKE $1 ESCAPE $2) AND (ns_status = 1)
ORDER BY ns_absid ASC LIMIT $3 OFFSET $4
`, pfx), db.ToSqlSearchPattern(query), "%", pageSize, pageNum*pageSize)
		} else {
			stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_acl, ns_status
FROM %s_namespace
WHERE (ns_status = 1)
ORDER BY ns_absid ASC LIMIT $1 OFFSET $2
`, pfx), pageSize, pageNum*pageSize)
		}
	}
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

func (dbif *PostgresGitusDatabaseInterface) GetAllVisibleRepositoryPaginated(username string, pageNum int64, pageSize int64) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var stmt pgx.Rows
	var err error
	if len(username) > 0 {
		stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_webhook, repo_absid
FROM %s_repository
WHERE repo_status = 1 OR repo_owner = $3 OR repo_acl->'acl' ? $3
ORDER BY repo_absid ASC LIMIT $1 OFFSET $2
`, pfx), pageSize, pageNum*pageSize, username)
		fmt.Println("tmt", err)
	} else {
		stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_webhook, repo_absid
FROM %s_repository
WHERE repo_status = 1
ORDER BY repo_absid ASC LIMIT $1 OFFSET $2
`, pfx), pageSize, pageNum*pageSize)
	}
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make([]*model.Repository, 0)
	var ns, name, description, owner, acl, forkOriginNs, forkOriginName, webhookstr string
	var rType, rStatus int
	var rowid int64
	for stmt.Next() {
		err = stmt.Scan(&rType, &ns, &name, &description, &owner, &acl, &rStatus, &forkOriginNs, &forkOriginName, &webhookstr, &rowid)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, name)
		m, err := model.CreateLocalRepository(uint8(rType), ns, name, p)
		if err != nil { return nil, err }
		webhookobj, err := model.ParseWebHookConfig(webhookstr)
		if err != nil { return nil, err }
		res = append(res, &model.Repository{
			AbsId: rowid,
			Namespace: ns,
			Name: name,
			Description: description,
			Owner: owner,
			AccessControlList: a,
			Status: model.GitusRepositoryStatus(rStatus),
			Type: uint8(rType),
			ForkOriginNamespace: forkOriginNs,
			ForkOriginName: forkOriginName,
			Repository: m,
			WebHookConfig: webhookobj,
		})
	}
	return  res, nil
}

func (dbif *PostgresGitusDatabaseInterface) SearchAllVisibleRepositoryPaginated(username string, query string, pageNum int64, pageSize int64) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var stmt pgx.Rows
	var err error
	if len(query) > 0 {
		queryPattern := db.ToSqlSearchPattern(query)
		if len(username) > 0 {
			stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_webhook, repo_absid
FROM %s_repository
WHERE (repo_status = 1 OR repo_owner = $3 OR repo_acl->'acl' ? $3) AND ((repo_namespace LIKE $4 ESCAPE $5) OR (repo_name LIKE $6 ESCAPE $7))
ORDER BY repo_absid ASC LIMIT $1 OFFSET $2
`, pfx), pageSize, pageNum*pageSize, username, queryPattern, "%", queryPattern, "%")
		} else {
			stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_webhook, repo_absid
FROM %s_repository
WHERE repo_status = 1
ORDER BY repo_absid ASC LIMIT $1 OFFSET $2
`, pfx), pageSize, pageNum*pageSize)
		}
	} else {
		if len(username) > 0 {
			stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_webhook, repo_absid
FROM %s_repository
WHERE repo_status = 1 OR repo_owner = $3 OR repo_acl->'acl' ? $3
ORDER BY repo_absid ASC LIMIT $1 OFFSET $2
`, pfx), pageSize, pageNum*pageSize, username)
		} else {
			stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_webhook, repo_absid
FROM %s_repository
WHERE repo_status = 1
ORDER BY repo_absid ASC LIMIT $1 OFFSET $2
`, pfx), pageSize, pageNum*pageSize)
		} 
	}
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make([]*model.Repository, 0)
	var ns, name, description, owner, acl, forkOriginNs, forkOriginName, webhookstr string
	var rType, rStatus int
	var rowid int64
	for stmt.Next() {
		err = stmt.Scan(&rType, &ns, &name, &description, &owner, &acl, &rStatus, &forkOriginNs, &forkOriginName, &webhookstr, &rowid)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, ns, name)
		m, err := model.CreateLocalRepository(uint8(rType), ns, name, p)
		if err != nil { return nil, err }
		webhookobj, err := model.ParseWebHookConfig(webhookstr)
		if err != nil { return nil, err }
		res = append(res, &model.Repository{
			AbsId: rowid,
			Namespace: ns,
			Name: name,
			Description: description,
			Owner: owner,
			AccessControlList: a,
			Status: model.GitusRepositoryStatus(rStatus),
			Type: uint8(rType),
			ForkOriginNamespace: forkOriginNs,
			ForkOriginName: forkOriginName,
			Repository: m,
			WebHookConfig: webhookobj,
		})
	}
	return  res, nil
}

func (dbif *PostgresGitusDatabaseInterface) GetAllNamespaceByOwner(name string) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
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

func (dbif *PostgresGitusDatabaseInterface) GetAllRepositoryFromNamespace(name string) (map[string]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
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
		res[name] = &model.Repository{
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

func (dbif *PostgresGitusDatabaseInterface) GetAllVisibleRepositoryFromNamespace(username string, ns string) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var stmt pgx.Rows
	var err error
	if len(username) > 0 {
		stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook, repo_absid
FROM %s_repository
WHERE repo_namespace = $1 AND repo_status = 1
`, pfx), ns)
	} else {
		stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook, repo_absid
FROM %s_repository
WHERE repo_namespace = $1 AND (repo_status = 1 OR repo_owner = $2 OR repo_acl->'acl' ? $2)
`, pfx), ns, username)
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

func (dbif *PostgresGitusDatabaseInterface) RegisterUser(name string, email string, passwordHash string, status model.GitusUserStatus) (*model.GitusUser, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)
	t := time.Now()
	webp := new(model.GitusUserWebsitePreference)
	webp.UseSiteWideThemeConfig = true
	_, err = tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s_user(user_name, user_title, user_email, user_bio, user_website, user_reg_datetime, user_password_hash, user_status, user_2fa_config, user_website_preference)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
`, pfx), name, name, email, new(string), new(string), t, passwordHash, status, new(model.GitusUser2FAConfig), webp)
	if err != nil { return nil, err }
	err = tx.Commit(ctx)
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

func (dbif *PostgresGitusDatabaseInterface) UpdateUserInfo(name string, uobj *model.GitusUser) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_user
SET user_title = $1, user_email = $2, user_bio = $3, user_website = $4, user_status = $5, user_2fa_config = $7, user_website_preference = $8
WHERE user_name = $6
`, pfx), uobj.Title, uobj.Email, uobj.Bio, uobj.Website, uobj.Status, name, uobj.TFAConfig, uobj.WebsitePreference)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) UpdateUserPassword(name string, newPasswordHash string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_user
SET user_password_hash = $1
WHERE user_name = $2
`, pfx), newPasswordHash, name)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) HardDeleteUserByName(name string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
DELETE FROM %s_user WHERE user_name = $1
`, pfx), name)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) UpdateUserStatus(name string, newStatus model.GitusUserStatus) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_user
SET user_status = $1
WHERE user_name = $2
`, pfx), newStatus, name)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) RegisterNamespace(name string, ownerUsername string) (*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)
	t := time.Now()
	_, err = tx.Exec(ctx, fmt.Sprintf(`
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
	err = tx.Commit(ctx)
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

func (dbif *PostgresGitusDatabaseInterface) UpdateNamespaceInfo(name string, nsobj *model.Namespace) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_namespace
SET ns_title = $1, ns_description = $2, ns_email = $3, ns_owner = $4, ns_status = $5
WHERE ns_name = $6
`, pfx), nsobj.Name, nsobj.Description, nsobj.Email, nsobj.Owner, nsobj.Status, name)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) UpdateNamespaceOwner(name string, newOwner string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_namespace
SET ns_owner = $1
WHERE ns_name = $2
`, pfx), newOwner, name)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) UpdateNamespaceStatus(name string, newStatus model.GitusNamespaceStatus) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_namespace
SET ns_status = $1
WHERE ns_name = $2
`, pfx), newStatus, name)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) HardDeleteNamespaceByName(name string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
DELETE FROM %s_namespace WHERE ns_name = $1
`, pfx), name)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) CreateRepository(ns string, name string, repoType uint8, owner string) (*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)
	webhookobj := new(model.WebHookConfig)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
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
	if err = tx.Commit(ctx); err != nil { return nil, err }
	r, err := model.NewRepository(ns, name, lr)
	if err != nil { return nil, err }
	r.Type = repoType
	r.Owner = owner
	r.RepoLabelList = nil
	r.WebHookConfig = webhookobj
	return r, nil
}

func (dbif *PostgresGitusDatabaseInterface) SetUpCloneRepository(originNs string, originName string, targetNs string, targetName string, owner string) (*model.Repository, error) {
	// TODO: fix this for multi vcs support
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)
	webhookobj := new(model.WebHookConfig)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
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
	if err = tx.Commit(ctx); err != nil { return nil, err }
	r, err := model.NewRepository(targetNs, targetName, targetLr)
	if err != nil { return nil, err }
	r.Type = model.REPO_TYPE_GIT
	r.Owner = owner
	r.RepoLabelList = nil
	r.WebHookConfig = webhookobj
	fmt.Println("done!")
	return r, nil
}

func (dbif *PostgresGitusDatabaseInterface) UpdateRepositoryInfo(ns string, name string, robj *model.Repository) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_repository
SET repo_description = $1, repo_owner = $2, repo_status = $3, repo_webhook = $6
WHERE repo_namespace = $4 AND repo_name = $5
`, pfx), robj.Description, robj.Owner, robj.Status, ns, name, robj.WebHookConfig)
	if err != nil { return err }
	if err = tx.Commit(ctx); err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) UpdateRepositoryStatus(ns string, name string, status model.GitusRepositoryStatus) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_repository
SET repo_status = $1
WHERE repo_namespace = $2 AND repo_name = $3
`, pfx), status, ns, name)
	if err != nil { return err }
	if err = tx.Commit(ctx); err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) HardDeleteRepository(ns string, name string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
DELETE FROM %s_repository
WHERE repo_namespace = $1 AND repo_name = $2
`, pfx), ns, name)
	if err != nil { return err }
	if err = tx.Commit(ctx); err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) GetAllUsers(pageNum int64, pageSize int64) ([]*model.GitusUser, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
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

func (dbif *PostgresGitusDatabaseInterface) GetAllNamespaces(pageNum int64, pageSize int64) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
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

func (dbif *PostgresGitusDatabaseInterface) GetAllRepositories(pageNum int64, pageSize int64) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
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

func (dbif *PostgresGitusDatabaseInterface) CountAllUser() (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_user
`, pfx))
	var r int64
	err := stmt.Scan(&r)
	if err != nil { return 0, err }
	return r, nil
}

func (dbif *PostgresGitusDatabaseInterface) CountAllNamespace() (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_namespace
`, pfx))
	var r int64
	err := stmt.Scan(&r)
	if err != nil { return 0, err }
	return r, nil
}

func (dbif *PostgresGitusDatabaseInterface) CountAllRepositories() (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
`, pfx))
	var r int64
	err := stmt.Scan(&r)
	if err != nil { return 0, err }
	return r, nil
}

func (dbif *PostgresGitusDatabaseInterface) CountAllRepositoriesSearchResult(q string) (int64, error) {
	if len(q) <= 0 { return dbif.CountAllRepositories() }
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository WHERE repo_ns LIKE $1 ESCAPE $2 OR repo_name LIKE $1 ESCAPE $2
`, pfx), db.ToSqlSearchPattern(q), "%")
	var r int64
	err := stmt.Scan(&r)
	if err != nil { return 0, err }
	return r, nil
}

func (dbif *PostgresGitusDatabaseInterface) CountAllVisibleNamespace(username string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var stmt pgx.Row
	if len(username) > 0 {
		stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_namespace WHERE ns_status = 1 OR ns_owner = $1 OR ns_acl->'acl' ? $1
`, pfx), username)
	} else {
		stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_namespace WHERE ns_status = 1
`, pfx))
	}
	var r int64
	err := stmt.Scan(&r)
	if err != nil { return 0, err }
	return r, nil
}

func (dbif *PostgresGitusDatabaseInterface) CountAllVisibleRepositories(username string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var stmt pgx.Row
	if len(username) > 0 {
		stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
INNER JOIN (SELECT ns_name FROM %s_namespace WHERE ns_status = 1 OR ns_owner = $1 OR ns_acl->'acl' ? $1) a
ON %s_repository.repo_namespace = a.ns_name
WHERE repo_status = 1 OR repo_status = 4 OR repo_owner = $1 OR repo_acl->'acl' ? $1
`, pfx, pfx, pfx), username)
	} else {
		stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
INNER JOIN (SELECT ns_name FROM %s_namespace WHERE ns_status = 1) a
ON %s_repository.repo_namespace = a.ns_name
WHERE repo_status = 1 or repo_status = 4
`, pfx, pfx, pfx))
	}
	var r int64
	err := stmt.Scan(&r)
	if err != nil { return 0, err }
	return r, nil
}

func (dbif *PostgresGitusDatabaseInterface) SearchForUser(k string, pageNum int64, pageSize int64) ([]*model.GitusUser, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	pattern := db.ToSqlSearchPattern(k)
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT user_name, user_title, user_email, user_bio, user_website, user_reg_datetime, user_password_hash, user_status
FROM %s_user
WHERE user_name LIKE $1 ESCAPE $2 OR user_title LIKE $1 ESCAPE $2
ORDER BY user_id ASC LIMIT $3 OFFSET $4
`, pfx), pattern, "%", pageSize, pageNum*pageSize)
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

func (dbif *PostgresGitusDatabaseInterface) SearchForNamespace(k string, pageNum int64, pageSize int64) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	pattern := db.ToSqlSearchPattern(k)
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_status, ns_acl
FROM %s_namespace
WHERE ns_name LIKE $1 ESCAPE $2 OR ns_title LIKE $1 ESCAPE $2
ORDER BY ns_absid ASC LIMIT $3 OFFSET $4
`, pfx), pattern, "%", pageSize, pageNum*pageSize)
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

func (dbif *PostgresGitusDatabaseInterface) SearchForRepository(k string, pageNum int64, pageSize int64) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	pattern := db.ToSqlSearchPattern(k)
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_webhook, repo_rowid
FROM %s_repository
WHERE repo_namespace LIKE $1 ESCAPE $2 OR repo_name LIKE $1 ESCAPE $2
ORDER BY repo_absid ASC LIMIT $3 OFFSET $4
`, pfx), pattern, "%", pageSize, pageNum*pageSize)
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make([]*model.Repository, 0)
	for stmt.Next() {
		var rType, rStatus int
		var repoNs, repoName, desc, owner, acl, forkOriginNs, forkOriginName, labelList, webhookstr string
		var rowid int64
		err := stmt.Scan(&rType, &repoNs, &repoName, &desc, &owner, &acl, &rStatus, &forkOriginNs, &forkOriginName, &labelList, &webhookstr, &rowid)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		p := path.Join(dbif.config.GitRoot, repoNs, repoName)
		m, err := model.CreateLocalRepository(uint8(rType), repoNs, repoName, p)
		if err != nil { return nil, err }
		var tags []string = nil
		if len(labelList) > 0 {
			tags = strings.Split(labelList[1:len(labelList)-1], "}{")
		}
		webhookobj, err := model.ParseWebHookConfig(webhookstr)
		res = append(res, &model.Repository{
			AbsId: rowid,
			Namespace: repoNs,
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

func (dbif *PostgresGitusDatabaseInterface) SetNamespaceACL(nsName string, targetUserName string, acl *model.ACLTuple) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	if acl == nil {
		_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_namespace SET ns_acl = ns_acl #- $1 WHERE ns_name = $2
`, pfx), []string{"acl", targetUserName}, nsName)
	} else {
		var r string
		r, err = acl.SerializeACLTuple()
		if err != nil { return err }
		_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_namespace SET ns_acl = jsonb_set(ns_acl, $1, $2) WHERE ns_name = $3
`, pfx), []string{"acl", targetUserName}, r, nsName)
	}
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) SetRepositoryACL(nsName string, repoName string, targetUserName string, acl *model.ACLTuple) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	if acl == nil {
		_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_repository SET repo_acl = repo_acl #- $1 WHERE repo_namespace = $2 AND repo_name = $3
`, pfx), []string{"acl", targetUserName}, nsName, repoName)
	} else {
		r, err := acl.SerializeACLTuple()
		if err != nil { return err }
		_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_repository SET repo_acl = jsonb_set(repo_acl, $1, $2) WHERE repo_namespace = $3 AND repo_name = $4
`, pfx), []string{"acl", targetUserName}, r, nsName, repoName)
	}
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) GetAllComprisingNamespace(username string) (map[string]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_acl, ns_status
FROM %s_namespace
WHERE ns_owner = $1 OR ns_acl->'acl' ? $1
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

func (dbif *PostgresGitusDatabaseInterface) CountAllVisibleNamespaceSearchResult(username string, pattern string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var err error
	var stmt pgx.Row
	if len(pattern) > 0 {
		pat := db.ToSqlSearchPattern(pattern)
		if len(username) > 0 {
			stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_namespace
WHERE (ns_owner = $1 OR ns_acl->'acl' ? $1 OR ns_status = 1)
AND (ns_name LIKE $2 ESCAPE $3 OR ns_title LIKE $2 ESCAPE $3)
`, pfx), username, pat, "%")
		} else {
			stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_namespace
WHERE (ns_status = 1) AND (ns_name LIKE $1 ESCAPE $2 OR ns_title LIKE $1 ESCAPE $2)
`, pfx), pat, "%")
		}
	} else {
		if len(username) > 0 {
			stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_namespace
WHERE (ns_owner = $1 OR ns_acl->'acl' ? $1) OR ns_status = 1
`, pfx), username)
		} else {
			stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_namespace
WHERE ns_status = 1
`, pfx))
		}
	}
	var res int64
	err = stmt.Scan(&res)
	if err != nil { return 0, err }
	return res, nil
}

func (dbif *PostgresGitusDatabaseInterface) CountAllVisibleRepositoriesSearchResult(username string, pattern string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var err error
	var stmt pgx.Row
	if len(pattern) > 0 {
		pat := db.ToSqlSearchPattern(pattern)
		if len(username) > 0 {
			stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*)
FROM %s_namespace
INNER JOIN (
    SELECT ns_name FROM %s_namespace
    WHERE (ns_status = 1) OR (ns_owner = $1 OR ns_acl->'acl' ? $1 OR ns_status = 1)
) a ON %s_repository.repo_namespace = a.ns_name
WHERE (repo_owner = $1 OR repo_acl->'acl' ? $1 OR repo_status = 1 OR repo_status = 4)
AND (repo_name LIKE $2 ESCAPE $3)
`, pfx, pfx, pfx), username, pat, "%")
		} else {
			stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*)
FROM %s_namespace
INNER JOIN (
    SELECT ns_name FROM %s_namespace
    WHERE (ns_status = 1) OR (ns_owner = $1 OR ns_acl->'acl' ? $1 OR ns_status = 1)
) a ON %s_repository.repo_namespace = a.ns_name
WHERE (repo_status = 1 OR repo_status = 4)
AND (repo_name LIKE $1 ESCAPE $2)
`, pfx, pfx, pfx), pat, "%")
		}
	} else {
		if len(username) > 0 {
			stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*)
FROM %s_namespace
INNER JOIN (
    SELECT ns_name FROM %s_namespace
    WHERE (ns_status = 1) OR (ns_owner = $1 OR ns_acl->'acl' ? $1 OR ns_status = 1)
) a ON %s_repository.repo_namespace = a.ns_name
WHERE (repo_owner = $1 OR repo_acl->'acl' ? $1 OR repo_status = 1 OR repo_status = 4)
`, pfx, pfx, pfx), username)
		} else {
			stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*)
FROM %s_namespace
INNER JOIN (
    SELECT ns_name FROM %s_namespace
    WHERE (ns_status = 1)
) a ON %s_repository.repo_namespace = a.ns_name
WHERE (repo_status = 1 OR repo_status = 4)
`, pfx, pfx, pfx))
		}
	}
	var res int64
	err = stmt.Scan(&res)
	if err != nil { return 0, err }
	return res, nil
}

func (dbif *PostgresGitusDatabaseInterface) GetAllRepositoryIssue(ns string, name string) ([]*model.Issue, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT issue_absid, issue_id, issue_timestamp, issue_author, issue_title, issue_content, issue_status
FROM %s_issue
WHERE repo_namespace = $1 AND repo_name = $2
`, pfx), ns, name)
	if err != nil { return nil, err }
	res := make([]*model.Issue, 0)
	var absid, id, status int64
	var t time.Time
	var author, title, content string
	for stmt.Next() {
		err = stmt.Scan(&absid, &id, &t, &author, &title, &content, &status)
		if err != nil { return nil, err }
		res = append(res, &model.Issue{
			IssueAbsId: absid,
			RepoNamespace: ns,
			RepoName: name,
			IssueId: int(id),
			IssueAuthor: author,
			IssueTitle: title,
			IssueContent: content,
			IssueTime: t.Unix(),
			IssueStatus: int(status),
		})
	}
	return res, nil
}

func (dbif *PostgresGitusDatabaseInterface) GetRepositoryIssue(ns string, name string, iid int) (*model.Issue, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT issue_absid, issue_timestamp, issue_author, issue_title, issue_content, issue_status, issue_priority
FROM %s_issue
WHERE repo_namespace = $1 AND repo_name = $2 AND issue_id = $3
`, pfx), ns, name, iid)
	var absid, id, status int64
	var t time.Time
	var priority int
	var author, title, content string
	err := stmt.Scan(&absid, &t, &author, &title, &content, &status, &priority)
	if err == pgx.ErrNoRows { return nil, db.ErrEntityNotFound }
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

func (dbif *PostgresGitusDatabaseInterface) CountAllRepositoryIssue(ns string, name string) (int, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*)
FROM %s_issue
WHERE repo_namespace = $1 AND repo_name = $2
`, pfx), ns, name)
	var res int
	err := stmt.Scan(&res)
	if err == pgx.ErrNoRows { return 0, db.ErrEntityNotFound }
	if err != nil { return 0, err }
	return res, nil
}

// filterType: 0 - all, 1 - open, 2 - closed, 3 - solved, 4 - discarded
// when query = "" it looks for all issue.
func (dbif *PostgresGitusDatabaseInterface) CountIssue(query string, namespace string, name string, filterType int) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	statusClause := ""
	switch filterType {
	case 0: statusClause = "TRUE"
    case 1: statusClause = "issue_status = 1"
	case 2: statusClause = "NOT (issue_status = 1)"
	case 3: statusClause = "issue_status = 2"
	case 4: statusClause = "issue_status = 3"
	}
	var stmt pgx.Row
	if len(query) > 0 {
		stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_issue
WHERE repo_namespace = $1 AND repo_name = $2
AND %s
AND (issue_title LIKE $3 ESCAPE $4)
`, pfx, statusClause), namespace, name, query, "%")
	} else {
		stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_issue
WHERE repo_namespace = $1 AND repo_name = $2
AND %s
`, pfx, statusClause), namespace, name)
	}
	var res int64
	err := stmt.Scan(&res)
	if err == pgx.ErrNoRows { return 0, db.ErrEntityNotFound }
	if err != nil { return 0, err }
	return res, nil
}

func (dbif *PostgresGitusDatabaseInterface) SearchIssuePaginated(query string, namespace string, name string, filterType int, pageNum int64, pageSize int64) ([]*model.Issue, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	statusClause := ""
	switch filterType {
	case 0: statusClause = "TRUE"
    case 1: statusClause = "issue_status = 1"
	case 2: statusClause = "NOT (issue_status = 1)"
	case 3: statusClause = "issue_status = 2"
	case 4: statusClause = "issue_status = 3"
	}
	var err error
	var stmt pgx.Rows
	if len(query) > 0 {
		pat := db.ToSqlSearchPattern(query)
		stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT issue_absid, issue_id, issue_timestamp, issue_author, issue_title, issue_content, issue_status, issue_priority
FROM %s_issue
WHERE repo_namespace = $1 AND repo_name = $2 AND %s AND (issue_title LIKE $3 ESCAPE $4)
ORDER BY issue_priority DESC, issue_timestamp DESC LIMIT $5 OFFSET $6
`, pfx, statusClause), namespace, name, pat, "\\", pageSize, pageNum*pageSize)
	} else {
		stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT issue_absid, issue_id, issue_timestamp, issue_author, issue_title, issue_content, issue_status, issue_priority
FROM %s_issue
WHERE repo_namespace = $1 AND repo_name = $2 AND %s
ORDER BY issue_priority DESC, issue_timestamp DESC LIMIT $3 OFFSET $4
`, pfx, statusClause), namespace, name, pageSize, pageNum*pageSize)
	}
	if err != nil { return nil, err }
	defer stmt.Close()
	res := make([]*model.Issue, 0)
	var absid, id, status int64
	var priority int
	var t time.Time
	var author, title, content string
	for stmt.Next() {
		err = stmt.Scan(&absid, &id, &t, &author, &title, &content, &status, &priority)
		if err != nil { return nil, err }
		res = append(res, &model.Issue{
			IssueAbsId: absid,
			RepoNamespace: namespace,
			RepoName: name,
			IssueId: int(id),
			IssueAuthor: author,
			IssueTitle: title,
			IssueContent: content,
			IssueTime: t.Unix(),
			IssueStatus: int(status),
			IssuePriority: priority,
		})
	}
	return res, nil
}

func (dbif *PostgresGitusDatabaseInterface) NewRepositoryIssue(ns string, name string, author string, title string, content string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt1 := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_issue WHERE repo_namespace = $1 AND repo_name = $2
`, pfx), ns, name)
	var newid int64
	err := stmt1.Scan(&newid)
	if err != nil { return 0, err }
	ctx2 := context.Background()
	tx, err := dbif.pool.Begin(ctx2)
	if err != nil { return 0, err }
	defer tx.Rollback(ctx2)
	t := time.Now()
	_, err = tx.Exec(ctx2, fmt.Sprintf(`
INSERT INTO %s_issue(repo_namespace, repo_name, issue_id, issue_timestamp, issue_author, issue_title, issue_content, issue_status, issue_priority)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
`, pfx), ns, name, newid, t, author, title, content, model.ISSUE_OPENED, 0)
	if err != nil { return 0, err }
	err = tx.Commit(ctx2)
	if err != nil { return 0, err }
	return newid, nil
}

func (dbif *PostgresGitusDatabaseInterface) HardDeleteRepositoryIssue(ns string, name string, issueId int) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	_, err = tx.Exec(ctx, fmt.Sprintf(`
DELETE FROM %s_issue WHERE repo_namespace = $1 AND repo_name = $2 AND issue_id = $3
`, pfx), ns, name, issueId)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) SetIssuePriority(namespace string, name string, id int64, priority int) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_issue SET issue_priority = $1 WHERE issue_id = $2 AND repo_namespace = $3 AND repo_name = $4
`, pfx), priority, id, namespace, name)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) GetAllIssueEvent(ns string, name string, issueId int) ([]*model.IssueEvent, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt1 := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT issue_absid FROM %s_issue WHERE repo_namespace = $1 AND repo_name = $2 AND issue_id = $3
`, pfx), ns, name, issueId)
	var absId int64
	err := stmt1.Scan(&absId);
	if err != nil { return nil, err }
	stmt2, err := dbif.pool.Query(ctx, fmt.Sprintf(`
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

func (dbif *PostgresGitusDatabaseInterface) NewRepositoryIssueEvent(ns string, name string, issueId int64, eType int, author string, content string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt1 := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT issue_absid, issue_status FROM %s_issue WHERE repo_namespace = $1 AND repo_name = $2 AND issue_id = $3
`, pfx), ns, name, issueId)
	var absId int64
	var issueStatus int
	err := stmt1.Scan(&absId, &issueStatus);
	if err != nil { return err }
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
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
		_, err := tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_issue SET issue_status = $1 WHERE issue_absid = $2
`, pfx), newIssueStatus, absId)
		if err != nil { return err }
	}
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) HardDeleteRepositoryIssueEvent(eventAbsId int64) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
DELETE FROM %s_issue_event WHERE issue_event_absid = $1
`, pfx), eventAbsId)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) GetAllBelongingNamespace(viewingUser string, user string) ([]*model.Namespace, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var stmt pgx.Rows
	var err error
	if len(viewingUser) > 0 {
		if viewingUser == user {
			stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_acl, ns_status
FROM %s_namespace WHERE ns_owner = $1 OR ns_acl->'acl' ? $1
`, pfx), viewingUser)
			if err != nil { return nil, err }
		} else {
			stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_acl, ns_status
FROM %s_namespace WHERE (ns_owner = $1 OR ns_acl->'acl' ? $1) AND (ns_owner = $2 OR ns_acl->'acl' ? $2)
`, pfx), viewingUser, user)
			if err != nil { return nil, err }
		}
	} else {
		stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT ns_name, ns_title, ns_description, ns_email, ns_owner, ns_reg_datetime, ns_acl, ns_status
FROM %s_namespace WHERE (ns_status = 1) AND (ns_owner = $1 OR ns_acl->'acl' ? $1)
`, pfx), user)
		if err != nil { return nil, err }
	}
	res := make([]*model.Namespace, 0)
	var name, title, description, email, owner, acl string
	var datetime time.Time
	var status int
	for stmt.Next() {
		err = stmt.Scan(&name, &title, &description, &email, &owner, &datetime, &acl, &status)
		if err != nil { return nil, err }
		a, err := model.ParseACL(acl)
		if err != nil { return nil, err }
		res = append(res, &model.Namespace{
			Name: name,
			Title: title,
			Description: description,
			Email: email,
			Owner: owner,
			RegisterTime: datetime.Unix(),
			ACL: a,
			Status: model.GitusNamespaceStatus(status),
		})
	}
	return  res, nil
}

func (dbif *PostgresGitusDatabaseInterface) GetAllBelongingRepository(viewingUser string, user string, query string, pageNum int64, pageSize int64) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var stmt pgx.Rows
	var err error
	if len(viewingUser) > 0 {
		if viewingUser == user {
			if len(query) <= 0 {
				stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_absid
FROM %s_repository
WHERE repo_owner = $1 OR repo_acl->'acl' ? $1
ORDER BY repo_absid ASC LIMIT $2 OFFSET $3
`, pfx), viewingUser, pageSize, pageNum*pageSize)
			} else {
				stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_absid
FROM %s_repository
WHERE (repo_owner = $1 OR repo_acl->'acl' ? $1)
AND (repo_name LIKE $4 ESCAPE $5 OR repo_namespace LIKE $4 ESCAPE $5)
ORDER BY repo_absid ASC LIMIT $2 OFFSET $3
`, pfx), viewingUser, pageSize, pageNum*pageSize, db.ToSqlSearchPattern(query), "\\")
			}
		} else {
			if len(query) <= 0 {
				stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_absid
FROM %s_repository
WHERE (repo_status = 1 OR repo_status = 4 OR repo_owner = $1 OR repo_acl->'acl' ? $1) AND (repo_owner = $2 OR repo_acl -> 'acl' ? $2)
ORDER BY repo_absid ASC LIMIT $3 OFFSET $4
`, pfx), viewingUser, user, pageSize, pageNum*pageSize)
			} else {
				stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_absid
FROM %s_repository
WHERE (repo_status = 1 OR repo_status = 4 OR repo_owner = $1 OR repo_acl->'acl' ? $1) AND (repo_owner = $2 OR repo_acl -> 'acl' ? $2)
AND (repo_name LIKE $5 ESCAPE $6 OR repo_namespace LIKE $5 ESCAPE $6)
ORDER BY repo_absid ASC LIMIT $3 OFFSET $4
`, pfx), viewingUser, user, pageSize, pageNum*pageSize, db.ToSqlSearchPattern(query), "\\")
			}
		}
	} else {
		if len(query) <= 0 {
			stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_absid
FROM %s_repository
WHERE (repo_status = 1 OR repo_status = 4) AND (repo_owner = $1 OR repo_acl->'acl' ? $1)
ORDER BY repo_absid ASC LIMIT $2 OFFSET $3
`, pfx), user, pageSize, pageNum*pageSize)
		} else {
			stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_absid
FROM %s_repository
WHERE (repo_status = 1 OR repo_status = 4) AND (repo_owner = $1 OR repo_acl->'acl' ? $1)
AND (repo_name LIKE $4 ESCAPE $5 OR repo_namespace LIKE $4 ESCAPE $5)
ORDER BY repo_absid ASC LIMIT $2 OFFSET $3
`, pfx), user, pageSize, pageNum*pageSize, db.ToSqlSearchPattern(query), "\\")
		}
	}
	if err != nil { return nil, err }
	res := make([]*model.Repository, 0)
	for stmt.Next() {
		var ns, name, desc, acl, owner, forkOriginNamespace, forkOriginName, labelList string
		var status, rowid int64
		var repoType uint8
		err := stmt.Scan(&repoType, &ns, &name, &desc, &owner, &acl, &status, &forkOriginNamespace, &forkOriginName, &labelList, &rowid)
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

func (dbif *PostgresGitusDatabaseInterface) CountAllBelongingRepository(viewingUser string, user string, query string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var stmt pgx.Row
	var err error
	if len(viewingUser) > 0 {
		if viewingUser == user {
			if len(query) <= 0 {
				stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
WHERE repo_owner = $1 OR repo_acl->'acl' ? $1
`, pfx), viewingUser)
			} else {
				stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
WHERE (repo_owner = $1 OR repo_acl->'acl' ? $1)
AND (repo_name LIKE $2 ESCAPE $3 OR repo_namespace LIKE $2 ESCAPE $3)
`, pfx), viewingUser, db.ToSqlSearchPattern(query), "\\")
			}
		} else {
			if len(query) <= 0 {
				stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
WHERE (repo_status = 1 OR repo_status = 4 OR repo_owner = $1 OR repo_acl->'acl' ? $1) AND (repo_owner = $2 OR repo_acl -> 'acl' ? $2)
`, pfx), viewingUser, user)
			} else {
				stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
WHERE (repo_status = 1 OR repo_status = 4 OR repo_owner = $1 OR repo_acl->'acl' ? $1) AND (repo_owner = $2 OR repo_acl -> 'acl' ? $2)
AND (repo_name LIKE $3 ESCAPE $4 OR repo_namespace LIKE $3 ESCAPE $4)
`, pfx), viewingUser, user, db.ToSqlSearchPattern(query), "\\")
			}
		}
	} else {
		if len(query) <= 0 {
			stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
WHERE (repo_status = 1 OR repo_status = 4) AND (repo_owner = $1 OR repo_acl->'acl' ? $1)
`, pfx), user)
		} else {
			stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
WHERE (repo_status = 1 OR repo_status = 4) AND (repo_owner = $1 OR repo_acl->'acl' ? $1)
AND (repo_name LIKE $2 ESCAPE $3 OR repo_namespace LIKE $2 ESCAPE $3)
`, pfx), user, db.ToSqlSearchPattern(query), "\\")
		}
	}
	var r int64
	err = stmt.Scan(&r)
	if err != nil { return 0, err }
	return r, nil
}

func (dbif *PostgresGitusDatabaseInterface) GetForkRepositoryOfUser(username string, originNamespace string, originName string) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
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

func (dbif *PostgresGitusDatabaseInterface) GetAllPullRequestPaginated(namespace string, name string, pageNum int64, pageSize int64) ([]*model.PullRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT pull_request_absid, pull_request_id, author_username, title, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp
FROM %s_pull_request
WHERE receiver_namespace = $1 AND receiver_name = $2
ORDER BY pull_request_id ASC LIMIT $3 OFFSET $4
`, pfx), namespace, name, pageSize, pageNum*pageSize)
	if err != nil { return nil, err }
	res := make([]*model.PullRequest, 0)
	var absid, id int64
	var author, title, receiverBranch, providerNs, providerName, providerBranch, mergeCheckResultString string
	var status int
	var mergeCheckTime, pullRequestTime time.Time
	for stmt.Next() {
		err = stmt.Scan(&absid, &id, &author, &title, &receiverBranch, &providerNs, &providerName, &mergeCheckResultString, &mergeCheckTime, &status, &pullRequestTime)
		if err != nil { return nil, err }
		var mergeCheckResult *gitlib.MergeCheckResult = nil
		if len(mergeCheckResultString) > 0 {		
			err = json.Unmarshal([]byte(mergeCheckResultString), &mergeCheckResult)
			if err != nil { return nil, err }
		}
		res = append(res, &model.PullRequest{
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
			MergeCheckTimestamp: mergeCheckTime.Unix(),
		})
	}
	return res, nil
}

func (dbif *PostgresGitusDatabaseInterface) NewPullRequest(username string, title string, receiverNamespace string, receiverName string, receiverBranch string, providerNamespace string, providerName string, providerBranch string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt1 := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_pull_request WHERE receiver_namespace = $1 AND receiver_name = $2
`, pfx), receiverNamespace, receiverName)
	var newId int64
	err := stmt1.Scan(&newId)
	if err != nil { return 0, err }
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return 0, err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s_pull_request(author_username, pull_request_id, title, receiver_namespace, receiver_name, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
`, pfx), username, newId, title, receiverNamespace, receiverName, receiverBranch, providerNamespace, providerName, providerBranch, new(string), new(time.Time), model.PULL_REQUEST_OPEN, time.Now())
	if err != nil { return 0, err }
	err = tx.Commit(ctx)
	if err != nil { return 0, err }
	return newId, nil
}

func (dbif *PostgresGitusDatabaseInterface) GetPullRequest(namespace string, name string, id int64) (*model.PullRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT pull_request_absid, author_username, title, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp
FROM %s_pull_request
WHERE receiver_namespace = $1 AND receiver_name = $2 AND pull_request_id = $3
`, pfx), namespace, name, id)
	var absid int64
	var author, title, receiverBranch, providerNs, providerName, providerBranch, mergeCheckString string
	var mergeConflictTime, pullRequestTime time.Time
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
		MergeCheckTimestamp: mergeConflictTime.Unix(),
	}, nil
}

func (dbif *PostgresGitusDatabaseInterface) GetPullRequestByAbsId(absId int64) (*model.PullRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT author_username, pull_request_id, title, receiver_anmespace, receiver_name, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp
FROM %s_pull_request
WHERE receiver_namespace = $1 AND receiver_name = $2 AND pull_request_id = $3
`, pfx), absId)
	var id int64
	var author, title, receiverNs, receiverName, receiverBranch, providerNs, providerName, providerBranch, mergeCheckString string
	var mergeConflictTime, pullRequestTime time.Time
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
		MergeCheckTimestamp: mergeConflictTime.Unix(),
	}, nil
}

func (dbif *PostgresGitusDatabaseInterface) CheckPullRequestMergeConflict(absId int64) (*gitlib.MergeCheckResult, error) {
	// WARNING: currently only works when when the source &
	// the target is git repo. currently (2025.8.27) this check
	// is performed at the controller side, i.e. users cannot
	// create pull request if the repo is not git repo, but the
	// code can still be called. DO NOT CALL UNLESS YOU KNOW
	// WHAT YOU'RE DOING.
	// TODO: fix this after figuring things out.
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT receiver_namespace, receiver_name, receiver_branch, provider_namespace, provider_name, provider_branch
FROM %s_pull_request
WHERE pull_request_absid = $1
`, pfx), absId)
	var receiverNamespace, receiverName, receiverBranch string
	var providerNamespace, providerName, providerBranch string
	err := stmt.Scan(&receiverNamespace, &receiverName, &receiverBranch, &providerNamespace, &providerName, &providerBranch)
	if err != nil { return nil, err }
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)
	p := path.Join(dbif.config.GitRoot, receiverNamespace, receiverName)
	lgr := gitlib.NewLocalGitRepository(p)
	remoteName := fmt.Sprintf("%s/%s", providerNamespace, providerName)
	mr, err := lgr.CheckBranchMergeConflict(receiverBranch, remoteName, providerBranch)
	if err != nil { return nil, err }
	mrstr, err := json.Marshal(mr)
	if err != nil { return nil, err }
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_pull_request
SET merge_conflict_check_result = $2, merge_conflict_check_timestamp = $3
WHERE pull_request_absid = $1
`, pfx), absId, string(mrstr), time.Now())
	if err != nil { return nil, err }
	err = tx.Commit(ctx)
	if err != nil { return nil, err }
	return mr, nil
}

func (dbif *PostgresGitusDatabaseInterface) DeletePullRequest(absId int64) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
DELETE FROM %s_pull_request WHERE pull_request_absid = $1
`, pfx), absId)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) GetAllPullRequestEventPaginated(absId int64, pageNum int64, pageSize int64) ([]*model.PullRequestEvent, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
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

func (dbif *PostgresGitusDatabaseInterface) CheckAndMergePullRequest(absId int64, username string) error {
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
	stmt0 := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
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
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	t := time.Now()
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_pull_request SET pull_request_status = $1, pull_request_timestamp = $2 WHERE pull_request_absid = $3
`, pfx), model.PULL_REQUEST_CLOSED_AS_MERGED, t, absId)
	if err != nil { return err }
	_, err = tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s_pull_request_event(pull_request_abs_id, event_type, event_timestamp, event_author, event_content)
VALUES ($1,$2,$3,$4,$5)
`, pfx), absId, model.PULL_REQUEST_EVENT_CLOSE_AS_MERGED, t, username, "")
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) CommentOnPullRequest(absId int64, author string, content string) (*model.PullRequestEvent, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)
	t := time.Now().Unix()
	_, err = tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s_pull_request_event(pull_request_absid, event_type, event_timestamp, event_author, event_content) VALUES ($1,$2,$3,$4,$5)
`, pfx), absId, model.PULL_REQUEST_EVENT_COMMENT, t, author, content)
	if err != nil { return nil, err }
	err = tx.Commit(ctx)
	if err != nil { return nil, err }
	return &model.PullRequestEvent{
		PRAbsId: absId,
		EventType: model.PULL_REQUEST_EVENT_COMMENT,
		EventTimestamp: t,
		EventAuthor: author,
		EventContent: content,
	}, nil
}

func (dbif *PostgresGitusDatabaseInterface) CommentOnPullRequestCode(absId int64, comment *model.PullRequestCommentOnCode) (*model.PullRequestEvent, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)
	t := time.Now().Unix()
	contentBytes, _ := json.Marshal(comment)
	contentString := string(contentBytes)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s_pull_request_event(pull_request_absid, event_type, event_timestamp, event_author, event_content)
VALUES ($1,$2,$3,$4,$5)
`, pfx), absId, model.PULL_REQUEST_EVENT_COMMENT_ON_CODE, t, comment.Username, contentString)
	if err != nil { return nil, err }
	err = tx.Commit(ctx)
	if err != nil { return nil, err }
	return &model.PullRequestEvent{
		PRAbsId: absId,
		EventType: model.PULL_REQUEST_EVENT_COMMENT_ON_CODE,
		EventTimestamp: t,
		EventAuthor: comment.Username,
		EventContent: contentString,
	}, nil
}

func (dbif *PostgresGitusDatabaseInterface) ClosePullRequestAsNotMerged(absid int64, author string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	t := time.Now()
	_, err = tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s_pull_request_event(pull_request_absid, event_type, event_timestamp, event_author, event_content)
VALUES ($1,$2,$3,$4,$5)
`, pfx), absid, model.PULL_REQUEST_EVENT_CLOSE_AS_NOT_MERGED, t, author, new(string))
	if err != nil { return err }
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_pull_request
SET pull_request_status = $1
WHERE pull_request_absid = $2
`, pfx), model.PULL_REQUEST_CLOSED_AS_NOT_MERGED, absid)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) ReopenPullRequest(absid int64, author string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	t := time.Now()
	_, err = tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s_pull_request_event(pull_request_absid, event_type, event_timestamp, event_author, event_content)
VALUES ($1,$2,$3,$4,$5)
`, pfx), absid, model.PULL_REQUEST_EVENT_REOPEN, t, author, new(string))
	if err != nil { return err }
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_pull_request
SET pull_request_status = $1
WHERE pull_request_absid = $2
`, pfx), model.PULL_REQUEST_OPEN, absid)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) CountPullRequest(query string, namespace string, name string, filterType int) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	statusClause := ""
	switch filterType {
	case 0: statusClause = ""
	case 1: statusClause = "AND pull_request_status = 1"
	case 2: statusClause = "AND NOT (pull_request_status = 1)"
	}
	var stmt pgx.Row
	if len(query) > 0 {
		pat := db.ToSqlSearchPattern(query)
		stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_pull_request
WHERE receiver_namespace = $1 AND receiver_name = $2 %s AND title LIKE $3 ESCAPE $4
`, pfx, statusClause), namespace, name, pat, "\\")
	} else {
		stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_pull_request
WHERE receiver_namespace = $1 AND receiver_name = $2 %s
`, pfx, statusClause), namespace, name)
	}
	var res int64
	err := stmt.Scan(&res)
	if err != nil { return 0, err }
	return res, nil
}

func (dbif *PostgresGitusDatabaseInterface) SearchPullRequestPaginated(query string, namespace string, name string, filterType int, pageNum int64, pageSize int64) ([]*model.PullRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	statusClause := ""
	switch filterType {
	case 0: statusClause = ""
	case 1: statusClause = "AND pull_request_status = 1"
	case 2: statusClause = "AND NOT (pull_request_status = 1)"
	case 3: statusClause = "AND pull_request_status = 2"
	case 4: statusClause = "AND pull_request_status = 3"
	}
	var stmt pgx.Rows
	var err error
	if len(query) >= 0 {
		pat := db.ToSqlSearchPattern(query)
		stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT pull_request_absid, author_username, pull_request_id, title, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp
FROM %s_pull_request
WHERE receiver_namespace = $1 AND receiver_name = $2 %s AND title LIKE $3 ESCAPE $4
ORDER BY pull_request_timestamp DESC LIMIT $5 OFFSET $6
`, pfx, statusClause), namespace, name, pat, "\\", pageSize, pageNum*pageSize)
		if err != nil { return nil, err }
	} else {
		stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT pull_request_absid, author_username, pull_request_id, title, receiver_branch, provider_namespace, provider_name, provider_branch, merge_conflict_check_result, merge_conflict_check_timestamp, pull_request_status, pull_request_timestamp
FROM %s_pull_request
WHERE receiver_namespace = $1 AND receiver_name = $2 %s
ORDER BY pull_request_timestamp DESC LIMIT $3 OFFSET $4
`, pfx, statusClause), namespace, name, pageSize, pageNum*pageSize)
		if err != nil { return nil, err }
	}
	res := make([]*model.PullRequest, 0)
	var prid, absid int64
	var prtime, mergeCheckTimestamp time.Time
	var status int
	var username, title, receiverBranch string
	var providerNamespace, providerName, provideBranch string
	var mergeCheckResultString string
	for stmt.Next() {
		err = stmt.Scan(&absid, &username, &prid, &title, &receiverBranch, &providerNamespace, &providerName, &provideBranch, &mergeCheckResultString, &mergeCheckTimestamp, &status, &prtime)
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
			Timestamp: prtime.Unix(),
			ReceiverNamespace: namespace,
			ReceiverName: name,
			ReceiverBranch: receiverBranch,
			ProviderNamespace: providerNamespace,
			ProviderName: providerName,
			ProviderBranch: provideBranch,
			Status: status,
			MergeCheckResult: mergeCheckResult,
			MergeCheckTimestamp: mergeCheckTimestamp.Unix(),
		})
	}
	return res, nil
}

func (dbif *PostgresGitusDatabaseInterface) GetAllRegisteredEmailOfUser(username string) ([]struct{Email string;Verified bool}, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
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

func (dbif *PostgresGitusDatabaseInterface) AddEmail(username string, email string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s_user_email(username, email, email_verified) VALUES ($1, $2, 0)
`, pfx), username, email)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) VerifyRegisteredEmail(username string, email string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_user_email SET email_verified = 1 WHERE username = $1 AND email = $2
`, pfx), username, email)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) DeleteRegisteredEmail(username string, email string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
DELETE FROM %s_user_email WHERE username = $1 AND email = $2
`, pfx), username, email)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) CheckIfEmailVerified(username string, email string) (bool, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT email_verified FROM %s_user_email WHERE username = $1 AND email = $2
`, pfx), username, email)
	var r int
	err := stmt.Scan(&r)
	if err != nil { return false, err }
	return r == 1, nil
}

func (dbif *PostgresGitusDatabaseInterface) ResolveEmailToUsername(email string) (string, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT username FROM %s_user_email WHERE email = $1 AND email_verified = 1
`, pfx), email)
	var r string
	err := stmt.Scan(&r)
	if err != nil { return "", err }
	return r, nil
}

func (dbif *PostgresGitusDatabaseInterface) ResolveMultipleEmailToUsername(emailList map[string]string) (map[string]string, error) {
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
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
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

func (dbif *PostgresGitusDatabaseInterface) InsertRegistrationRequest(username string, email string, passwordHash string, reason string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s_user_reg_request(username, email, password_hash, reason, timestamp) VALUES ($1,$2,$3,$4,$5)
`, pfx), username, email, passwordHash, reason, time.Now())
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) ApproveRegistrationRequest(absid int64) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt1 := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
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
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
DELETE FROM %s_user_reg_request WHERE username = $1
`, pfx), username)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) DisapproveRegistrationRequest(absid int64) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT username FROM %s_user_reg_request
WHERE request_absid = $1
`, pfx), absid)
	var username string
	err := stmt.Scan(&username)
	if err != nil { return err }
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
DELETE FROM %s_user_reg_request WHERE username = $1
`, pfx), username)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) GetRegistrationRequestPaginated(pageNum int64, pageSize int64) ([]*model.RegistrationRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
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

func (dbif *PostgresGitusDatabaseInterface) GetRequestOfUsernamePaginated(username string, pageNum int64, pageSize int64) ([]*model.RegistrationRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
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

func (dbif *PostgresGitusDatabaseInterface) CountRegistrationRequest(query string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var r pgx.Row
	var err error
	query = strings.TrimSpace(query)
	if len(query) <= 0 {
		r = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_user_reg_request
`, pfx))
	} else {
		r = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_user_reg_request WHERE username LIKE $1 ESCAPE $2
`, pfx), db.ToSqlSearchPattern(query), "\\")
	}
	var cnt int64
	err = r.Scan(&cnt)
	if err != nil { return 0, err }
	return cnt, nil
}

func (dbif *PostgresGitusDatabaseInterface) SearchRegistrationRequestPaginated(query string, pageNum int64, pageSize int64) ([]*model.RegistrationRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var stmt pgx.Rows
	var err error
	query = strings.TrimSpace(query)
	if len(query) <= 0 {
		stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT request_absid, username, email, password_hash, timestamp FROM %s_user_reg_request ORDER BY timestamp DESC LIMIT $1 OFFSET $2 
`, pfx), pageSize, pageNum*pageSize)
	} else {
		stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
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

func (dbif *PostgresGitusDatabaseInterface) GetRegistrationRequestByAbsId(absid int64) (*model.RegistrationRequest, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
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

func (dbif *PostgresGitusDatabaseInterface) AddRepositoryLabel(ns string, name string, lbl string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
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
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_repository SET repo_label_list = $1 WHERE repo_namespace = $2 AND repo_name = $3
`, pfx), fmt.Sprintf("{%s}", strings.Join(tags, "}{")), ns, name)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) RemoveRepositoryLabel(ns string, name string, lbl string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT repo_label_list FROM %s_repository WHERE repo_namespace = $1 AND repo_name = $2
`, pfx), ns, name)
	var rll string
	err := stmt.Scan(&rll)
	if err != nil { return err }
	tags := strings.Split(rll[1:len(rll)-1], "}{")
	idx := slices.Index(tags, lbl)
	if idx == -1 { return nil }
	tags = slices.Delete(tags, idx, idx+1)
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_repository SET repo_label_list = $1 WHERE repo_namespace = $2 AND repo_name = $3
`, pfx), fmt.Sprintf("{%s}", strings.Join(tags, "}{")), ns, name)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) GetRepositoryLabel(ns string, name string) ([]string, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT repo_label_list FROM %s_repository WHERE repo_namespace = $1 and repo_name = $2
`, pfx), ns, name)
	var rll string
	err := stmt.Scan(&rll)
	if err != nil { return nil, err }
	tags := strings.Split(rll[1:len(rll)-1], "}{")
	return tags, nil
}

func (dbif *PostgresGitusDatabaseInterface) CountRepositoryWithLabel(username string, label string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var r pgx.Row
	var err error
	if len(username) <= 0 {
		r = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
WHERE repo_label_list LIKE $1 ESCAPE $2
AND (repo_status = 1 OR repo_status = 4)
`, pfx), db.ToSqlSearchPattern(fmt.Sprintf("{%s}", label)), "\\")
	} else {
		r = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_repository
WHERE repo_label_list LIKE $1 ESCAPE $2
AND (
    repo_status = 1 OR repo_status = 4 OR repo_status = 5
    OR (repo_owner = $3 OR repo_acl->'acl' ? $4))
`, pfx), db.ToSqlSearchPattern(fmt.Sprintf("{%s}", label)), "\\", username, username)
	}
	var res int64
	err = r.Scan(&res)
	if err != nil { return 0, err }
	return res, nil
}

func (dbif *PostgresGitusDatabaseInterface) GetRepositoryWithLabelPaginated(username string, label string, pageNum int64, pageSize int64) ([]*model.Repository, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var r pgx.Rows
	var err error
	if len(username) <= 0 {
		r, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT  repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_absid
FROM %s_repository
WHERE repo_label_list LIKE $1 ESCAPE $2
AND (repo_status = 1 OR repo_status = 4)
ORDER BY repo_name ASC, repo_namespace ASC LIMIT $3 OFFSET $4
`, pfx), db.ToSqlSearchPattern(fmt.Sprintf("{%s}", label)), "\\", pageSize, pageNum*pageSize)
		if err != nil { return nil, err }
	} else {
		r, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT  repo_type, repo_namespace, repo_name, repo_description, repo_owner, repo_acl, repo_status, repo_fork_origin_namespace, repo_fork_origin_name, repo_label_list, repo_absid
FROM %s_repository
WHERE repo_label_list LIKE $1 ESCAPE $2
AND (
    repo_status = 1 OR repo_status = 4 OR repo_status = 5
    OR (repo_owner = $3 OR repo_acl->'acl' ? $4))
ORDER BY repo_name ASC, repo_namespace ASC LIMIT $5 OFFSET $6
`, pfx), db.ToSqlSearchPattern(fmt.Sprintf("{%s}", label)), "\\", username, username, pageSize, pageNum*pageSize)
		fmt.Println("xx", pageNum)
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


func (dbif *PostgresGitusDatabaseInterface) NewSnippet(username string, name string, status uint8) (*model.Snippet, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return nil, err }
	defer tx.Rollback(ctx)
	t := time.Now()
	_, err = tx.Exec(ctx, fmt.Sprintf(`
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
	err = tx.Commit(ctx)
	if err != nil { return nil, err }
	return &model.Snippet{
		Name: name,
		BelongingUser: username,
		Description: "",
		Time: t.Unix(),
		FileList: make(map[string]string, 0),
	}, nil
}

func (dbif *PostgresGitusDatabaseInterface) GetAllSnippet(username string) ([]*model.Snippet, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt, err := dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = $1
`, pfx), username)
	if err != nil { return nil, err }
	var name, desc string
	var status uint8
	var timestamp time.Time
	var sharedUser map[string]bool
	res := make([]*model.Snippet, 0)
	for stmt.Next() {
		err = stmt.Scan(&name, &desc, &timestamp, &status, &sharedUser)
		if err != nil { return nil, err }
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

func (dbif *PostgresGitusDatabaseInterface) CountAllVisibleSnippet(username string, viewingUser string, query string) (int64, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var stmt pgx.Row
	var err error
	if len(query) > 0 {
		q := db.ToSqlSearchPattern(query)
		if len(viewingUser) <= 0 {
			stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_snippet
WHERE username = $1 AND (status = 1) AND name LIKE $2 ESCAPE $3
`, pfx), username, q, "\\")
		} else if viewingUser == username {
			stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_snippet
WHERE username = $1 AND name LIKE $2 ESCAPE $3
`, pfx), username, q, "\\")
		} else {
			stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_snippet
WHERE username = $1 AND (status = 1 OR status = 2 OR (status = 4 AND shared_user ? $2)) AND name LIKE $3 ESCAPE $4
`, pfx), username, viewingUser, q, "\\")
		}
	} else {
		if len(viewingUser) <= 0 {
			stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_snippet
WHERE username = $1 AND (status = 1 OR status = 2)
`, pfx), username)
		} else if viewingUser == username {
			stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_snippet
WHERE username = $1
`, pfx), username)
		} else {
			stmt = dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT COUNT(*) FROM %s_snippet
WHERE username = $1 AND (status = 1 OR status = 2 OR (status = 4 AND shared_user ? $2))
`, pfx), username, viewingUser)
		}
	}
	var res int64
	err = stmt.Scan(&res)
	if err != nil { return 0, err }
	return res, nil
}

func (dbif *PostgresGitusDatabaseInterface) GetAllVisibleSnippetPaginated(username string, viewingUser string, query string, pageNum int64, pageSize int64) ([]*model.Snippet, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	var stmt pgx.Rows
	var err error
	if len(query) > 0 {
		q := db.ToSqlSearchPattern(query)
		if len(viewingUser) <= 0 {
			stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = $1 AND (status = 1) AND name LIKE $2 ESCAPE $3
`, pfx), username, q, "\\")
		} else if viewingUser == username {
			stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = $1 AND name LIKE $2 ESCAPE $3
`, pfx), username, q, "\\")
		} else {
			stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = $1 AND (status = 1 OR status = 2 OR (status = 4 AND shared_user ? $2)) AND name LIKE $3 ESCAPE $4
`, pfx), username, viewingUser, q, "\\")
		}
	} else {
		if len(viewingUser) <= 0 {
			stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = $1 AND (status = 1 OR status = 2)
`, pfx), username)
		} else if viewingUser == username {
			stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = $1
`, pfx), username)
		} else {
			stmt, err = dbif.pool.Query(ctx, fmt.Sprintf(`
SELECT name, description, timestamp, status, shared_user FROM %s_snippet
WHERE username = $1 AND (status = 1 OR status = 2 OR (status = 4 AND shared_user ? $2))
`, pfx), username, viewingUser)
		}
	}
	if err != nil { return nil, err }
	var name, desc string
	var status uint8
	var timestamp time.Time
	var sharedUser map[string]bool
	res := make([]*model.Snippet, 0)
	for stmt.Next() {
		err = stmt.Scan(&name, &desc, &timestamp, &status, &sharedUser)
		if err != nil { return nil, err }
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

func (dbif *PostgresGitusDatabaseInterface) DeleteSnippet(username string, name string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
DELETE FROM %s_snippet WHERE username = $1 AND name = $2
`, pfx), username, name)
	if err != nil { return err }
	p := path.Join(dbif.config.SnippetRoot, username, name)
	err = os.RemoveAll(p)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) SaveSnippetInfo(m *model.Snippet) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_snippet
SET description = $1, status = $2, shared_user = $3
WHERE username = $4 AND name = $5
`, pfx), m.Description, m.Status, m.SharedUser, m.BelongingUser, m.Name)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) GetSnippet(username string, name string) (*model.Snippet, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
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


func (dbif *PostgresGitusDatabaseInterface) RegisterWebhookRequest(uuid string, reportUuid string, repoNs string, repoName string, commitId string) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	d := new(model.WebhookResult)
	d.Status = model.WEBHOOK_RESULT_UNDEFINED
	d.ReportUUID = reportUuid
	d.UUID = uuid
	d.RepoNamespace = repoNs
	d.RepoName = repoName
	_, err = tx.Exec(ctx, fmt.Sprintf(`
INSERT INTO %s_webhook_log(uuid, repo_namespace, repo_name, commit_id, webhook_result)
VALUES ($1,$2,$3,$4,$5)
`, pfx), uuid, repoNs, repoName, commitId, d)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) UpdateWebhookResult(uuid string, result *model.WebhookResult) error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	if result.Timestamp == 0 { result.Timestamp = time.Now().Unix() }
	_, err = tx.Exec(ctx, fmt.Sprintf(`
UPDATE %s_webhook_log SET webhook_result = $1 WHERE uuid = $2
`, pfx), result, uuid)
	if err != nil { return err }
	err = tx.Commit(ctx)
	if err != nil { return err }
	return nil
}

func (dbif *PostgresGitusDatabaseInterface) GetWebhookResultByUUID(uuid string) (*model.WebhookResult, error) {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	stmt := dbif.pool.QueryRow(ctx, fmt.Sprintf(`
SELECT webhook_result
FROM %s_webhook_log
WHERE uuid = $1
`, pfx), uuid)
	var webhookRes *model.WebhookResult
	err := stmt.Scan(&webhookRes)
	if err != nil { return nil, err }
	return webhookRes, nil
}


