package platform

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func OpenPostgres(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 10
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour

	var pool *pgxpool.Pool
	for i := 0; i < 10; i++ {
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			err = pool.Ping(ctx)
		}
		if err == nil {
			return pool, nil
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("postgres connection: %w", err)
}
