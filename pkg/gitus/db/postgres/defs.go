package postgres

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/GitusCodeForge/Gitus/pkg/gitus"
	pgx "github.com/jackc/pgx/v5"
	pgxpool "github.com/jackc/pgx/v5/pgxpool"
)


type PostgresGitusDatabaseInterface struct {
	config *gitus.GitusConfig
	pool *pgxpool.Pool
}

func NewPostgresGitusDatabaseInterface(cfg *gitus.GitusConfig) (*PostgresGitusDatabaseInterface, error) {
	u := &url.URL{
		Scheme: "postgres",
		User: url.UserPassword(cfg.Database.UserName, cfg.Database.Password),
		Host: cfg.Database.URL,
		Path: cfg.Database.DatabaseName,
	}
	pool, err := pgxpool.New(context.TODO(), u.String())
	if err != nil { return nil, err }
	return &PostgresGitusDatabaseInterface{
		config: cfg,
		pool: pool,
	}, nil
}

func (dbif *PostgresGitusDatabaseInterface) Dispose() error {
	dbif.pool.Close()
	return nil
}

var requiredTableList = []string{
	"user_authkey",
	"user_signkey",
	"user_email",
	"user",
	"namespace",
	"repository",
	"issue",
	"issue_event",
	"pull_request",
	"pull_request_event",
	"webhook_log",
}

func (dbif *PostgresGitusDatabaseInterface) IsDatabaseUsable() (bool, error) {
	ctx := context.Background()
	queryStr := `
SELECT EXISTS (SELECT FROM pg_tables WHERE schemaname = 'public' AND tablename = $1)
`
	for _, item := range requiredTableList {
		tableName := fmt.Sprintf("%s_%s", dbif.config.Database.TablePrefix, item)
		stmt := dbif.pool.QueryRow(ctx, queryStr, tableName)
		var a bool
		err := stmt.Scan(&a)
		if errors.Is(err, pgx.ErrNoRows) { return false, nil }
		if err != nil { return false, err }
		if (!a) { return false, nil }
	}
	return true, nil
}

