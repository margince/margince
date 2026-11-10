// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/originalstore"
)

func originalStorageHTTP(pool *pgxpool.Pool, blob blobstore.Store, next http.Handler) http.Handler {
	db := InstallationDB(pool)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := originalstore.Bind(r.Context(), db, blob)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
