// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemmail

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// ReadableMail is what a seat read asks of a mail beyond the content gate. It
// is an email the whole workspace may read, live, not held and not restricted.
func ReadableMail(alias string) string {
	return workspaceMail(alias) + fmt.Sprintf(` AND %s.archived_at IS NULL`, alias)
}

// workspaceMail is ReadableMail with archived mail kept, for a read that may ask
// for it.
func workspaceMail(alias string) string {
	return fmt.Sprintf(`%[1]s.kind = 'email' AND %[1]s.audience = 'workspace' AND %[2]s AND NOT %[3]s`,
		alias, auth.ActivityAvailableClause(alias), auth.ActivityRestrictedSQL(alias+".id"))
}

// Opened is the kind of the export row one open leaves. The daily cap counts
// these rows.
const Opened = "extension_attachment_open"

// fileColumns are the columns of a MailAttachment, in scan order.
const fileColumns = `at.id, at.entity_id, at.filename, at.content_type, at.byte_size, at.created_at`

// mailFileClause keeps a mail's own files that still have their bytes.
const mailFileClause = `at.entity_type = 'activity' AND at.category = 'email_attachment'
	AND at.archived_at IS NULL AND NOT at.bytes_withheld`

const listQuery = `
	SELECT ` + fileColumns + `
	  FROM attachment at
	 WHERE at.entity_id = $1 AND ` + mailFileClause + `
	 ORDER BY at.created_at, at.id
	 LIMIT $2`

const openQuery = `
	SELECT ` + fileColumns + `, at.storage_key
	  FROM attachment at
	  JOIN activity a ON a.id = at.entity_id
	 WHERE at.id = $1 AND ` + mailFileClause + ` AND %s`

// capLock serializes the last pass of one unit's opens, so two at once cannot
// both take the last place under the cap.
const capLock = `SELECT pg_advisory_xact_lock(hashtextextended('extension_attachment_open:' || $1, 0))`

const opensToday = `
	SELECT count(*) FROM system_log
	 WHERE action = 'export' AND occurred_at >= date_trunc('day', now() AT TIME ZONE 'UTC') AT TIME ZONE 'UTC'
	   AND detail->>'kind' = $1 AND detail->>'extension' = $2`

// Files reads a mail's attachments for one unit, as its service member. The
// context carries that member's principal.
type Files struct {
	Unit  string
	ActAs ids.UUID
	Blob  blobstore.Store
	Pool  *pgxpool.Pool
}

var errTooLarge = fmt.Errorf("%w: the attachment holds more than %d bytes", extension.ErrInvalid, extension.MaxInboundFileBytes)

var errNoStore = errors.New("systemmail: no object store is bound, so no attachment can be opened")

// List answers the files of one readable mail and writes its export row.
func (f Files) List(ctx context.Context, mail ids.UUID) ([]extension.MailAttachment, error) {
	var out []extension.MailAttachment
	err := database.WithWorkspaceTx(ctx, f.Pool, func(tx pgx.Tx) error {
		if err := ensureReadableMail(ctx, tx, mail); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, listQuery, mail, extension.MaxReadPage)
		if err != nil {
			return err
		}
		if out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (extension.MailAttachment, error) { return scanFile(row) }); err != nil {
			return err
		}
		return f.logExport(ctx, tx, map[string]any{"kind": "extension_read", "row_count": len(out), "mail_id": mail.String()})
	})
	return out, err
}

// Open reads one file with its bytes. The gate and the cap run before the
// bytes are fetched, so a refusal costs no read. They run again after it, under
// the unit's lock, because the file may have been withheld or its mail
// restricted meanwhile. Only that second pass writes the export row, so a read
// that fails uses no place under the cap.
func (f Files) Open(ctx context.Context, id ids.UUID) (extension.MailAttachmentFile, error) {
	if f.Blob == nil {
		return extension.MailAttachmentFile{}, errNoStore
	}
	var (
		file extension.MailAttachment
		key  string
	)
	err := database.WithWorkspaceTx(ctx, f.Pool, func(tx pgx.Tx) (err error) {
		if err := f.underCap(ctx, tx); err != nil {
			return err
		}
		file, key, err = f.admit(ctx, tx, id)
		return err
	})
	if err != nil {
		return extension.MailAttachmentFile{}, err
	}
	content, err := f.read(ctx, key)
	if err != nil {
		return extension.MailAttachmentFile{}, err
	}
	err = database.WithWorkspaceTx(ctx, f.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, capLock, f.Unit); err != nil {
			return err
		}
		if _, again, err := f.admit(ctx, tx, id); err != nil || again != key {
			return cmp.Or(err, apperrors.ErrNotFound)
		}
		if err := f.underCap(ctx, tx); err != nil {
			return err
		}
		return f.logExport(ctx, tx, map[string]any{"kind": Opened, "row_count": 1, "attachment_id": file.ID, "mail_id": file.MailID})
	})
	if err != nil {
		return extension.MailAttachmentFile{}, err
	}
	file.ContentType = extension.SniffContentType(content)
	return extension.MailAttachmentFile{MailAttachment: file, Content: content}, nil
}

// admit reads one file and its key, under the seat's grants, the mail rule and
// the size bound.
func (f Files) admit(ctx context.Context, tx pgx.Tx, id ids.UUID) (extension.MailAttachment, string, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return extension.MailAttachment{}, "", err
	}
	var key string
	file, err := scanFile(tx.QueryRow(ctx, fmt.Sprintf(openQuery, ReadableMail("a")), id), &key)
	if err != nil {
		return extension.MailAttachment{}, "", err
	}
	mail, err := ids.Parse(file.MailID)
	if err != nil {
		return extension.MailAttachment{}, "", err
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, mail); err != nil {
		return extension.MailAttachment{}, "", err
	}
	if file.ByteSize > extension.MaxInboundFileBytes {
		return extension.MailAttachment{}, "", errTooLarge
	}
	return file, key, nil
}

// read fetches a file's bytes and refuses more than MaxInboundFileBytes,
// whatever its row recorded.
func (f Files) read(ctx context.Context, key string) ([]byte, error) {
	body, _, err := f.Blob.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	content, err := io.ReadAll(io.LimitReader(body, extension.MaxInboundFileBytes+1))
	if closeErr := body.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}
	if len(content) > extension.MaxInboundFileBytes {
		return nil, errTooLarge
	}
	return content, nil
}

// Published answers every way a file or its mail is hidden as
// extension.ErrNotFound, and passes a published refusal on. It answers nil for
// a fault of the core's own.
func Published(err error) error {
	switch {
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, apperrors.ErrNotFound):
		return extension.ErrNotFound
	case errors.Is(err, extension.ErrInvalid), errors.Is(err, extension.ErrAttachmentCapReached):
		return err
	}
	return nil
}

// ensureReadableMail admits a mail MailContent().Get would read.
func ensureReadableMail(ctx context.Context, tx pgx.Tx, mail ids.UUID) error {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, mail); err != nil {
		return err
	}
	var readable bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id = $1 AND `+ReadableMail("a")+`)`, mail).Scan(&readable)
	if err == nil && !readable {
		return apperrors.ErrNotFound
	}
	return err
}

// underCap refuses an open past the unit's daily number.
func (f Files) underCap(ctx context.Context, tx pgx.Tx) error {
	var opened int
	if err := tx.QueryRow(ctx, opensToday, Opened, f.Unit).Scan(&opened); err != nil {
		return err
	}
	if opened >= extension.MaxAttachmentOpensPerDay {
		return extension.ErrAttachmentCapReached
	}
	return nil
}

// logExport writes the export row of one list or open: the unit, the seat it
// ran as, and what it read.
func (f Files) logExport(ctx context.Context, tx pgx.Tx, detail map[string]any) error {
	detail["extension"], detail["act_as"], detail["table"] = f.Unit, f.ActAs.String(), "attachment"
	_, err := storekit.LogSystem(ctx, tx, "export", detail)
	return err
}

// scanFile scans fileColumns, then any columns after them into extra.
func scanFile(row pgx.Row, extra ...any) (extension.MailAttachment, error) {
	var (
		id, mail    ids.UUID
		contentType *string
		size        *int64
		created     time.Time
		out         extension.MailAttachment
	)
	if err := row.Scan(append([]any{&id, &mail, &out.Filename, &contentType, &size, &created}, extra...)...); err != nil {
		return extension.MailAttachment{}, err
	}
	out.ID, out.MailID, out.CreatedAt = id.String(), mail.String(), created
	if contentType != nil {
		out.ContentType = *contentType
	}
	if size != nil {
		out.ByteSize = *size
	}
	return out, nil
}
