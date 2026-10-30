package postgres

import (
	"context"
	"errors"
	"fmt"

	pgx "github.com/jackc/pgx/v5"
)

// unique name: VARCHAR(64)
// title: VARCHAR(96)
// long description: VARCHAR(4096)
// email: VARCHAR(256)
// url: VARCHAR(2048)
// password hash: VARCHAR(256)
// timestamp: TIMESTAMP
// complex (e.g. ACLs): JSONB
// integer enum: SMALLINT

func (dbif *PostgresGitusDatabaseInterface) InstallTables() error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	tx, err := dbif.pool.Begin(ctx)
	if err != nil { return err }
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_user (
    user_id BIGINT GENERATED ALWAYS AS IDENTITY,
    user_name VARCHAR(64) UNIQUE,
    user_title VARCHAR(96),
    user_email VARCHAR(256),
    user_bio VARCHAR(4096),
    user_website VARCHAR(2048),
    user_reg_datetime TIMESTAMP,
    user_password_hash VARCHAR(256),
    user_status SMALLINT,
    user_2fa_config JSONB,
    user_website_preference JSONB
)
`, pfx))
	if err != nil { return err }
	_, err = tx.Exec(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_user_authkey (
    user_name VARCHAR(64),
    key_name VARCHAR(64),
    key_text TEXT,
    FOREIGN KEY (user_name) REFERENCES %s_user(user_name)
)`, pfx, pfx))
	if err != nil { return err }
	_, err = tx.Exec(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_user_signkey (
    user_name VARCHAR(64),
    key_name VARCHAR(64),
    key_text TEXT,
    FOREIGN KEY (user_name) REFERENCES %s_user(user_name)
)`, pfx, pfx))
	if err != nil { return err }
	_, err = tx.Exec(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_namespace (
    ns_absid BIGINT GENERATED ALWAYS AS IDENTITY,
    ns_name VARCHAR(64) UNIQUE,
    ns_title VARCHAR(96),
    ns_description VARCHAR(4096),
    ns_email VARCHAR(256),
    ns_owner VARCHAR(64),
    ns_reg_datetime TIMESTAMP,
    ns_acl JSONB,
    ns_status SMALLINT,
    FOREIGN KEY (ns_owner) REFERENCES %s_user(user_name)
)`, pfx, pfx))
	if err != nil { return err }
	_, err = tx.Exec(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_repository (
    repo_absid BIGINT GENERATED ALWAYS AS IDENTITY,
    repo_type SMALLINT,
    repo_namespace VARCHAR(64) REFERENCES %s_namespace(ns_name),
    repo_name VARCHAR(64),
    repo_description VARCHAR(4096),
    repo_owner VARCHAR(64),
    repo_acl JSONB,
    repo_status SMALLINT,
    repo_fork_origin_namespace VARCHAR(64),
    repo_fork_origin_name VARCHAR(64),
    repo_label_list VARCHAR(512),
    repo_webhook JSONB,
    UNIQUE (repo_namespace, repo_name)
)`, pfx, pfx))
	if err != nil { return err }
	_, err = tx.Exec(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_issue (
    issue_absid BIGINT PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
    repo_namespace VARCHAR(64),
    repo_name VARCHAR(64),
    issue_id INTEGER,
    issue_timestamp TIMESTAMP,
    issue_author VARCHAR(64) REFERENCES %s_user(user_name),
    issue_title VARCHAR(4096),
    issue_content TEXT,
    issue_status SMALLINT,
    issue_priority SMALLINT,
    FOREIGN KEY (repo_namespace, repo_name) REFERENCES %s_repository(repo_namespace, repo_name)
)`, pfx, pfx, pfx))
	if err != nil { return err }
	_, err = tx.Exec(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_issue_event (
    issue_event_absid BIGINT GENERATED ALWAYS AS IDENTITY,
    issue_absid BIGINT REFERENCES %s_issue(issue_absid),
    issue_event_type SMALLINT,
    issue_event_time TIMESTAMP,
    issue_event_author VARCHAR(64),
    issue_event_content TEXT
)`, pfx, pfx))
	if err != nil { return err }
	_, err = tx.Exec(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_pull_request(
    pull_request_absid BIGINT PRIMARY KEY GENERATED ALWAYS AS IDENTITY,
    author_username VARCHAR(64),
    pull_request_id INTEGER,
    title VARCHAR(4096),
    receiver_namespace VARCHAR(64),
    receiver_name VARCHAR(64),
    receiver_branch VARCHAR(96),
    provider_namespace VARCHAR(64),
    provider_name VARCHAR(64),
    provider_branch VARCHAR(96),
    merge_conflict_check_result TEXT,
    merge_conflict_check_timestamp TIMESTAMP,
    pull_request_status SMALLINT,
    pull_request_timestamp TIMESTAMP,
    FOREIGN KEY (author_username) REFERENCES %s_user(user_name)
)`, pfx, pfx))
	if err != nil { return err }
	_, err = tx.Exec(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_pull_request_event (
    pull_request_absid BIGINT,
    event_type SMALLINT,
    event_timestamp TIMESTAMP,
    event_author VARCHAR(64),
    event_content TEXT,
    FOREIGN KEY (pull_request_absid) REFERENCES %s_pull_request(pull_request_absid)
)`, pfx, pfx))
	if err != nil { return err }
	_, err = tx.Exec(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_user_email (
    username VARCHAR(64) REFERENCES %s_user(user_name),
	email VARCHAR(256),
    email_verified SMALLINT
)`, pfx, pfx))
	if err != nil { return err }
	_, err = tx.Exec(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_user_reg_request (
    request_absid BIGINT GENERATED ALWAYS AS IDENTITY,
    username VARCHAR(64),
	email VARCHAR(256),
    password_hash VARCHAR(256),
	reason VARCHAR(4096),
    timestamp TIMESTAMP
)`, pfx))
	// NOTE: the shared_user field in this table is implemented
	// as an json kvtable (e.g. {"user1":true,"user2":true})
	// for query w/ `shared_user ? USERNAME`.
	_, err = tx.Exec(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_snippet (
    absid BIGINT GENERATED ALWAYS AS IDENTITY,
    string_id VARCHAR(256),
    name VARCHAR(64),
    username VARCHAR(64) REFERENCES %s_user(user_name),
	description VARCHAR(4096),
    timestamp TIMESTAMP,
    status SMALLINT,
    shared_user JSONB
)`, pfx, pfx))
	_, err = tx.Exec(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_webhook_log (
    uuid VARCHAR(48) UNIQUE,
	repo_namespace VARCHAR(64),
	repo_name VARCHAR(64),
	commit_id VARCHAR(96),
    webhook_result JSONB
)`, pfx))
	if err != nil { return err }

	_, err = tx.Exec(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_id_counter (
    repo_namespace VARCHAR(64),
    repo_name VARCHAR(64),
    kind SMALLINT,
    next_id BIGINT,
    PRIMARY KEY (repo_namespace, repo_name, kind)
)`, pfx))
	if err != nil { return err }

	for _, def := range indexDefinitions {
		_, err = tx.Exec(ctx, fmt.Sprintf(def.sql, pfx, pfx))
		if err != nil { return err }
	}

	err = tx.Commit(ctx)
	if err != nil { return err }

	return nil
}


// indexDefinition is one of the indexes the listing and lookup queries rely on
// (P4 of the audit). The name keeps the %s placeholder for the table prefix,
// because PostgreSQL index names are global within a schema.
// (repository(repo_namespace, repo_name) already has a UNIQUE constraint.)
type indexDefinition struct {
	name string
	sql string
}

var indexDefinitions = []indexDefinition{
	{"idx_%s_issue_repo_namespace_repo_name", `CREATE INDEX IF NOT EXISTS idx_%s_issue_repo_namespace_repo_name ON %s_issue (repo_namespace, repo_name);`},
	{"idx_%s_issue_event_issue_absid", `CREATE INDEX IF NOT EXISTS idx_%s_issue_event_issue_absid ON %s_issue_event (issue_absid);`},
	{"idx_%s_pull_request_receiver_namespace_receiver_name", `CREATE INDEX IF NOT EXISTS idx_%s_pull_request_receiver_namespace_receiver_name ON %s_pull_request (receiver_namespace, receiver_name);`},
	{"idx_%s_user_email_email", `CREATE INDEX IF NOT EXISTS idx_%s_user_email_email ON %s_user_email (email);`},
	{"idx_%s_user_email_username", `CREATE INDEX IF NOT EXISTS idx_%s_user_email_username ON %s_user_email (username);`},
	{"idx_%s_snippet_username", `CREATE INDEX IF NOT EXISTS idx_%s_snippet_username ON %s_snippet (username);`},
	{"idx_%s_namespace_ns_owner", `CREATE INDEX IF NOT EXISTS idx_%s_namespace_ns_owner ON %s_namespace (ns_owner);`},
	{"idx_%s_repository_repo_owner", `CREATE INDEX IF NOT EXISTS idx_%s_repository_repo_owner ON %s_repository (repo_owner);`},
}

// EnsureSchema creates the tables and indexes that were added after the
// database was installed. It runs once at startup and is idempotent; an index
// that already exists is never rebuilt, so a running instance does not pay for
// DDL (and the table lock it takes) on every restart.
func (dbif *PostgresGitusDatabaseInterface) EnsureSchema() error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	_, err := dbif.pool.Exec(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_id_counter (
    repo_namespace VARCHAR(64),
    repo_name VARCHAR(64),
    kind SMALLINT,
    next_id BIGINT,
    PRIMARY KEY (repo_namespace, repo_name, kind)
)`, pfx))
	if err != nil { return err }
	for _, def := range indexDefinitions {
		var name string
		err = dbif.pool.QueryRow(ctx,
			`SELECT indexname FROM pg_indexes WHERE indexname = $1`,
			fmt.Sprintf(def.name, pfx)).Scan(&name)
		if err == nil { continue }
		if !errors.Is(err, pgx.ErrNoRows) { return err }
		_, err = dbif.pool.Exec(ctx, fmt.Sprintf(def.sql, pfx, pfx))
		if err != nil { return err }
	}
	return nil
}
