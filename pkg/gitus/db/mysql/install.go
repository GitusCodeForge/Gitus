package mysql

import (
	"context"
	"database/sql"
	"fmt"
)

// unique name: VARCHAR(64)
// title: VARCHAR(96)
// long description: VARCHAR(4096)
// email: VARCHAR(256)
// url: VARCHAR(2048)
// password hash: VARCHAR(256)
// timestamp: DATETIME (TIMESTAMP ends in 2038)
// complex (e.g. ACLs): JSON
// integer enum: SMALLINT

func (dbif *MysqlGitusDatabaseInterface) InstallTables() error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()

	statements := []string{
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_user (
    user_id BIGINT AUTO_INCREMENT PRIMARY KEY,
    user_name VARCHAR(64) UNIQUE,
    user_title VARCHAR(96),
    user_email VARCHAR(256),
    user_bio VARCHAR(4096),
    user_website VARCHAR(2048),
    user_reg_datetime DATETIME,
    user_password_hash VARCHAR(256),
    user_status SMALLINT,
    user_2fa_config JSON,
    user_website_preference JSON
)`, pfx),
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_user_authkey (
    user_name VARCHAR(64),
    key_name VARCHAR(64),
    key_text TEXT,
    FOREIGN KEY (user_name) REFERENCES %s_user(user_name)
)`, pfx, pfx),
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_user_signkey (
    user_name VARCHAR(64),
    key_name VARCHAR(64),
    key_text TEXT,
    FOREIGN KEY (user_name) REFERENCES %s_user(user_name)
)`, pfx, pfx),
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_namespace (
    ns_absid BIGINT AUTO_INCREMENT PRIMARY KEY,
    ns_name VARCHAR(64) UNIQUE,
    ns_title VARCHAR(96),
    ns_description VARCHAR(4096),
    ns_email VARCHAR(256),
    ns_owner VARCHAR(64),
    ns_reg_datetime DATETIME,
    ns_acl JSON,
    ns_status SMALLINT,
    FOREIGN KEY (ns_owner) REFERENCES %s_user(user_name)
)`, pfx, pfx),
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_repository (
    repo_absid BIGINT AUTO_INCREMENT PRIMARY KEY,
    repo_type SMALLINT,
    repo_namespace VARCHAR(64) REFERENCES %s_namespace(ns_name),
    repo_name VARCHAR(64),
    repo_description VARCHAR(4096),
    repo_owner VARCHAR(64),
    repo_acl JSON,
    repo_status SMALLINT,
    repo_fork_origin_namespace VARCHAR(64),
    repo_fork_origin_name VARCHAR(64),
    repo_label_list VARCHAR(512),
    repo_webhook JSON,
    UNIQUE (repo_namespace, repo_name)
)`, pfx, pfx),
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_issue (
    issue_absid BIGINT PRIMARY KEY AUTO_INCREMENT,
    repo_namespace VARCHAR(64),
    repo_name VARCHAR(64),
    issue_id INTEGER,
    issue_timestamp DATETIME,
    issue_author VARCHAR(64) REFERENCES %s_user(user_name),
    issue_title VARCHAR(4096),
    issue_content TEXT,
    issue_status SMALLINT,
    issue_priority SMALLINT,
    FOREIGN KEY (repo_namespace, repo_name) REFERENCES %s_repository(repo_namespace, repo_name)
)`, pfx, pfx, pfx),
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_issue_event (
    issue_event_absid BIGINT AUTO_INCREMENT PRIMARY KEY,
    issue_absid BIGINT REFERENCES %s_issue(issue_absid),
    issue_event_type SMALLINT,
    issue_event_time DATETIME,
    issue_event_author VARCHAR(64),
    issue_event_content TEXT
)`, pfx, pfx),
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_pull_request (
    pull_request_absid BIGINT PRIMARY KEY AUTO_INCREMENT,
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
    merge_conflict_check_timestamp DATETIME,
    pull_request_status SMALLINT,
    pull_request_timestamp DATETIME,
    FOREIGN KEY (author_username) REFERENCES %s_user(user_name)
)`, pfx, pfx),
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_pull_request_event (
    pull_request_absid BIGINT,
    event_type SMALLINT,
    event_timestamp DATETIME,
    event_author VARCHAR(64),
    event_content TEXT,
    FOREIGN KEY (pull_request_absid) REFERENCES %s_pull_request(pull_request_absid)
)`, pfx, pfx),
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_user_email (
    username VARCHAR(64) REFERENCES %s_user(user_name),
    email VARCHAR(256),
    email_verified SMALLINT
)`, pfx, pfx),
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_user_reg_request (
    request_absid BIGINT AUTO_INCREMENT PRIMARY KEY,
    username VARCHAR(64),
    email VARCHAR(256),
    password_hash VARCHAR(256),
    reason VARCHAR(4096),
    timestamp DATETIME
)`, pfx),
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_snippet (
    absid BIGINT AUTO_INCREMENT PRIMARY KEY,
    string_id VARCHAR(256),
    name VARCHAR(64),
    username VARCHAR(64) REFERENCES %s_user(user_name),
    description VARCHAR(4096),
    timestamp DATETIME,
    status SMALLINT,
    shared_user JSON
)`, pfx, pfx),
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_webhook_log (
    uuid VARCHAR(48) UNIQUE,
    repo_namespace VARCHAR(64),
    repo_name VARCHAR(64),
    commit_id VARCHAR(96),
    webhook_result JSON
)`, pfx),
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_id_counter (
    repo_namespace VARCHAR(64),
    repo_name VARCHAR(64),
    kind SMALLINT,
    next_id BIGINT,
    PRIMARY KEY (repo_namespace, repo_name, kind)
)`, pfx),
	}
	for _, stmt := range statements {
		if _, err := dbif.connection.ExecContext(ctx, stmt); err != nil { return err }
	}
	return ensureIndexes(ctx, dbif.connection, pfx)
}

// indexDefinition is one of the indexes the listing and lookup queries rely on
// (P4 of the audit). MySQL index names are scoped to their table, so they do
// not need the gitus table prefix and stay well below the 64 character limit.
// (repository(repo_namespace, repo_name) already has a UNIQUE constraint.)
type indexDefinition struct {
	name string
	table string
	columns string
}

var indexDefinitions = []indexDefinition{
	{"idx_issue_repo_namespace_repo_name", "issue", "(repo_namespace, repo_name)"},
	{"idx_issue_event_issue_absid", "issue_event", "(issue_absid)"},
	{"idx_pull_request_receiver_namespace_receiver_name", "pull_request", "(receiver_namespace, receiver_name)"},
	{"idx_user_email_email", "user_email", "(email)"},
	{"idx_user_email_username", "user_email", "(username)"},
	{"idx_snippet_username", "snippet", "(username)"},
	{"idx_namespace_ns_owner", "namespace", "(ns_owner)"},
	{"idx_repository_repo_owner", "repository", "(repo_owner)"},
}

type sqlContextExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func ensureIndexes(ctx context.Context, q sqlContextExecutor, pfx string) error {
	for _, def := range indexDefinitions {
		tableName := fmt.Sprintf("%s_%s", pfx, def.table)
		var n int
		err := q.QueryRowContext(ctx, `
SELECT COUNT(*) FROM information_schema.statistics
WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?
`, tableName, def.name).Scan(&n)
		if err != nil { return err }
		if n > 0 { continue }
		_, err = q.ExecContext(ctx, fmt.Sprintf("CREATE INDEX `%s` ON `%s` %s", def.name, tableName, def.columns))
		if err != nil { return err }
	}
	return nil
}

// EnsureSchema creates the tables and indexes that were added after the
// database was installed. It runs once at startup and is idempotent; an index
// that already exists is checked with information_schema instead of being
// rebuilt, so a restart does not take DDL locks it does not need.
func (dbif *MysqlGitusDatabaseInterface) EnsureSchema() error {
	pfx := dbif.config.Database.TablePrefix
	ctx := context.Background()
	_, err := dbif.connection.ExecContext(ctx, fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s_id_counter (
    repo_namespace VARCHAR(64),
    repo_name VARCHAR(64),
    kind SMALLINT,
    next_id BIGINT,
    PRIMARY KEY (repo_namespace, repo_name, kind)
)`, pfx))
	if err != nil { return err }
	return ensureIndexes(ctx, dbif.connection, pfx)
}
