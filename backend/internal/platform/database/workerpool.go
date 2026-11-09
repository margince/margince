// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"errors"
	"math"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WorkerControlConnections stay outside the budget for pinned session locks.
const WorkerControlConnections = 2

// WorkerListenerConnections accounts for River's connection hijacked outside pgxpool.
const WorkerListenerConnections = 1

// WorkerPoolConfig bounds job demand by the process budget and the DSN ceiling.
func WorkerPoolConfig(dsn string, concurrency, budget, cacheCapacity int) (*pgxpool.Config, error) {
	if concurrency < 1 || budget < 5 || cacheCapacity < 0 {
		return nil, errors.New("pg: worker needs positive job concurrency, at least five connections and a nonnegative cache capacity")
	}
	cfg, err := PoolConfig(dsn)
	if err != nil {
		return nil, errors.New("pg: invalid database connection settings; check the DSN and pool limits")
	}
	demand := min(concurrency, (math.MaxInt32-WorkerControlConnections)/2)*2 + WorkerControlConnections
	ceiling := min(budget-WorkerListenerConnections, demand)
	if ceiling < 4 || ceiling > math.MaxInt32 {
		return nil, errors.New("pg: worker connection limit must fit a pool of at least four connections")
	}
	cfg.MaxConns = min(cfg.MaxConns, int32(ceiling))
	if cfg.MaxConns < 4 {
		return nil, errors.New("pg: worker pool must allow at least five connections")
	}
	cfg.MinConns = min(cfg.MinConns, cfg.MaxConns)
	cfg.ConnConfig.StatementCacheCapacity = min(cfg.ConnConfig.StatementCacheCapacity, cacheCapacity)
	cfg.ConnConfig.DescriptionCacheCapacity = min(cfg.ConnConfig.DescriptionCacheCapacity, cacheCapacity)
	if cfg.ConnConfig.StatementCacheCapacity == 0 && cfg.ConnConfig.DefaultQueryExecMode == pgx.QueryExecModeCacheStatement ||
		cfg.ConnConfig.DescriptionCacheCapacity == 0 && cfg.ConnConfig.DefaultQueryExecMode == pgx.QueryExecModeCacheDescribe {
		return nil, errors.New("pg: disabled worker caches require default_query_exec_mode=exec, describe_exec or simple_protocol in the DSN")
	}
	return cfg, nil
}

// WorkerSessionCapacity keeps pinned locks from consuming the connections their work needs.
func WorkerSessionCapacity(connections int32) int {
	return max(0, (int(connections)-WorkerControlConnections)/2)
}

// NewWorkerPool opens and checks the worker pool with its resource limits.
func NewWorkerPool(ctx context.Context, dsn string, concurrency, budget, cacheCapacity int) (*pgxpool.Pool, error) {
	cfg, err := WorkerPoolConfig(dsn, concurrency, budget, cacheCapacity)
	if err != nil {
		return nil, err
	}
	return openPool(ctx, cfg)
}
