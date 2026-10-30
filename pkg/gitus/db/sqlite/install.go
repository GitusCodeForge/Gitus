package sqlite

import "fmt"


func (dbif *SqliteGitusDatabaseInterface) InstallTables() error {
	pfx := dbif.config.Database.TablePrefix
	tx, err := dbif.connection.Begin()
	if err != nil { return err }
	defer tx.Rollback()
	_, err = tx.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_user (
    user_name TEXT UNIQUE,
    user_title TEXT,
    user_email TEXT,
    user_bio TEXT,
    user_website TEXT,
    user_reg_datetime TEXT,
    user_password_hash TEXT,
    user_status INTEGER,
    user_2fa_config TEXT,
    user_website_preference TEXT
)`, pfx))
	if err != nil { return err }
	_, err = tx.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_user_authkey (
    user_name TEXT,
    key_name TEXT,
    key_text TEXT,
    FOREIGN KEY (user_name) REFERENCES %s_user(user_name)
)`, pfx, pfx))
	if err != nil { return err }
	_, err = tx.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_user_signkey (
    user_name TEXT,
    key_name TEXT,
    key_text TEXT,
    FOREIGN KEY (user_name) REFERENCES %s_user(user_name)
)`, pfx, pfx))
	if err != nil { return err }
	_, err = tx.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_namespace (
    ns_name TEXT UNIQUE,
  	ns_title TEXT,
  	ns_description TEXT,
  	ns_email TEXT,
  	ns_owner TEXT,
  	ns_reg_datetime TEXT,
    ns_acl TEXT,
  	ns_status INTEGER,
      FOREIGN KEY (ns_owner) REFERENCES %s_user(user_name)
)`, pfx, pfx))
	if err != nil { return err }
	_, err = tx.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_repository (
    repo_type INTEGER,
    repo_fullname TEXT UNIQUE,
    repo_namespace TEXT,
    repo_name TEXT,
    repo_description TEXT,
    repo_owner TEXT,
  	repo_acl TEXT,
  	repo_status INTEGER,
  	repo_fork_origin_namespace TEXT,
  	repo_fork_origin_name TEXT,
    repo_label_list TEXT,
    repo_webhook TEXT,
      FOREIGN KEY (repo_namespace) REFERENCES %s_namespace(ns_name)
)`, pfx, pfx))
	if err != nil { return err }

	_, err = tx.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_repo_redirect (
    old_ns TEXT,
    old_name TEXT,
	new_ns TEXT,
	new_name TEXT,
	redirect_timestamp INTEGER
)`, pfx))
	if err != nil { return err }

	_, err = tx.Exec(fmt.Sprintf(`
CREATE UNIQUE INDEX IF NOT EXISTS idx_%s_user_user_name
ON %s_user (user_name)
`, pfx, pfx))
	if err != nil { return err }

	_, err = tx.Exec(fmt.Sprintf(`
CREATE INDEX IF NOT EXISTS idx_%s_user_authkey_user_name
ON %s_user_authkey (user_name);
`, pfx, pfx))
	if err != nil { return err }

	_, err = tx.Exec(fmt.Sprintf(`
CREATE INDEX IF NOT EXISTS idx_%s_user_signkey_user_name
ON %s_user_signkey (user_name);
`, pfx, pfx))
	if err != nil { return err }

	_, err = tx.Exec(fmt.Sprintf(`
CREATE INDEX IF NOT EXISTS idx_%s_namespace_ns_name
ON %s_namespace (ns_name);
`, pfx, pfx))
	if err != nil { return err }

	_, err = tx.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_issue (
    repo_namespace TEXT,
	repo_name TEXT,
	issue_id INTEGER,
	issue_timestamp INTEGER,
	issue_author TEXT,
	issue_title TEXT,
	issue_content TEXT,
	-- 1 - opened.  2 - close as solved.  3 - close as discarded.
	issue_status INTEGER,
    issue_priority SMALLINT,
	FOREIGN KEY (repo_namespace, repo_name)
      REFERENCES %s_repository(repo_namespace, repo_name),
	FOREIGN KEY (issue_author)
	  REFERENCES %s_user(user_name)
);
`, pfx, pfx, pfx))
	if err != nil { return err }

	_, err = tx.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_issue_event (
    issue_abs_id INTEGER,
	-- 1 - comment.  2 - close as solved.  3 - close as discarded.
	-- 4 - reopened.
	issue_event_type INTEGER,
	issue_event_time INTEGER,
	issue_event_author TEXT,
	issue_event_content TEXT,
    FOREIGN KEY (issue_abs_id) REFERENCES %s_issue(rowid)
);
`, pfx, pfx))
	if err != nil { return err }

	_, err = tx.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_pull_request (
    username TEXT,
    pull_request_id INT,
    title TEXT,
    -- the receiving repo
    receiver_namespace TEXT,
    receiver_name TEXT,
    receiver_branch TEXT,
    -- the repo you're pulling from
    provider_namespace TEXT,
    provider_name TEXT,
    provider_branch TEXT,
    -- in json.
    merge_conflict_check_result TEXT,
    merge_conflict_check_timestamp INT,
    pull_request_status INT,
    pull_request_timestamp INT,
	FOREIGN KEY (username) REFERENCES %s_user(user_name)
  )
`, pfx, pfx))
	if err != nil { return err }
	
	_, err = tx.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_pull_request_event (
    pull_request_abs_id INTEGER,
	-- 1 - normal comment.
	-- 2 - comment on code.
	-- 3 - update on provider branch.
	-- 4 - merge conflict check.
	-- 5 - close as not merged.
	-- 6 - close (merged).
    -- 7 - reopen
	event_type INTEGER,
	event_timestamp INTEGER,
	event_author TEXT,
	event_content TEXT,
	FOREIGN KEY (pull_request_abs_id) REFERENCES %spull_request(rowid)
  )
`, pfx, pfx))
	if err != nil { return err }

	_, err = tx.Exec(fmt.Sprintf(`
CREATE INDEX IF NOT EXISTS idx_%s_pull_request_event_pull_request_abs_id
ON %s_pull_request_event (pull_request_abs_id);
`, pfx, pfx))
	if err != nil { return err }

	_, err = tx.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_user_email (
    username TEXT,
	email TEXT,
    verified INTEGER,
	FOREIGN KEY (username) REFERENCES %s_user(user_name)
)`, pfx, pfx))
	if err != nil { return err }

	_, err = tx.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_user_reg_request (
    username TEXT,
	email TEXT,
    password_hash TEXT,
	reason TEXT
    timestamp INTEGER
)`, pfx))
	if err != nil { return err }
	
	_, err = tx.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_snippet (
    snippet_full_name TEXT UNIQUE,
    name TEXT,
    username TEXT,
	description TEXT,
    timestamp INTEGER,
    status INTEGER,
    shared_user TEXT,
    FOREIGN KEY (username) REFERENCES %s_user(user_name)
)`, pfx, pfx))
	if err != nil { return err }

	_, err = tx.Exec(fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_webhook_log (
    uuid TEXT UNIQUE,
	repo_namespace TEXT,
	repo_name TEXT,
	commit_id TEXT,
    webhook_result TEXT
)`, pfx))
	if err != nil { return err }

	// Indexes for the queries that run on every issue/PR list, every issue
	// view, every email lookup and every ownership check. See P4 of the audit.
	indexStatements := []string{
		`CREATE INDEX IF NOT EXISTS idx_%s_issue_repo_namespace_repo_name ON %s_issue (repo_namespace, repo_name);`,
		`CREATE INDEX IF NOT EXISTS idx_%s_issue_event_issue_abs_id ON %s_issue_event (issue_abs_id);`,
		`CREATE INDEX IF NOT EXISTS idx_%s_pull_request_receiver_namespace_receiver_name ON %s_pull_request (receiver_namespace, receiver_name);`,
		`CREATE INDEX IF NOT EXISTS idx_%s_repository_repo_namespace_repo_name ON %s_repository (repo_namespace, repo_name);`,
		`CREATE INDEX IF NOT EXISTS idx_%s_user_email_email ON %s_user_email (email);`,
		`CREATE INDEX IF NOT EXISTS idx_%s_user_email_username ON %s_user_email (username);`,
		`CREATE INDEX IF NOT EXISTS idx_%s_snippet_username ON %s_snippet (username);`,
		`CREATE INDEX IF NOT EXISTS idx_%s_namespace_ns_owner ON %s_namespace (ns_owner);`,
		`CREATE INDEX IF NOT EXISTS idx_%s_repository_repo_owner ON %s_repository (repo_owner);`,
	}
	for _, stmt := range indexStatements {
		_, err = tx.Exec(fmt.Sprintf(stmt, pfx, pfx))
		if err != nil { return err }
	}

	tx.Commit()
	return nil
}

