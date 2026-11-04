// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// mailLetterheadReadTimeout bounds the letterhead read. A password reset has
// two minutes to reach the relay, and a slow object store must not spend them.
const mailLetterheadReadTimeout = 5 * time.Second

// mailLetterhead heads the mail the system relay sends with the
// installation's own company: its logo when one is stored, else its name.
// The name falls back to the installation-name setting until the company is
// described.
type mailLetterhead struct {
	pool    *pgxpool.Pool
	logo    signatureLogo
	timeout time.Duration
}

func newMailLetterhead(pool *pgxpool.Pool, blob blobstore.Store) mailLetterhead {
	return mailLetterhead{pool: pool, logo: newSignatureLogo(blob), timeout: mailLetterheadReadTimeout}
}

// Letterhead never fails the send. Anything it cannot read in time leaves the
// letterhead with less on it, and says so in the log.
func (m mailLetterhead) Letterhead(ctx context.Context, language string) (emailview.Letterhead, []connector.InlineImage) {
	head := emailview.Letterhead{Attribution: mailcopy.For(language).PoweredBy}
	if m.pool == nil {
		return head, nil
	}
	bounded, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	var key string
	err := InstallationDB(m.pool).Tx(bounded, func(tx pgx.Tx) error {
		var err error
		head.Name, key, err = letterheadNameTx(bounded, tx)
		return err
	})
	if err != nil {
		slog.WarnContext(ctx, "mail letterhead: the company could not be read; the mail is headed by Margince", "cause", err)
		return head, nil
	}
	if key == "" {
		return head, nil
	}
	image, ok, err := m.logo.SignatureLogo(bounded, key)
	if err != nil {
		slog.WarnContext(ctx, "mail letterhead: the company logo could not be read; the mail is headed by the name", "cause", err)
		return head, nil
	}
	if !ok {
		return head, nil
	}
	image.ContentID = connector.LetterheadLogoContentID
	head.LogoContentID = image.ContentID
	return head, []connector.InlineImage{image}
}

// letterheadNameTx is the one name system mail speaks as, and the stored logo
// key beside it: the installation's own company, else the installation-name
// setting until the company is described. Product mail and privacy notices
// both read it here, so the two cannot name different companies.
func letterheadNameTx(ctx context.Context, tx pgx.Tx) (string, string, error) {
	name, key, err := contacts.NewStore(nil).MailLetterheadTx(ctx, tx)
	if err != nil || name != "" {
		return name, key, err
	}
	name, err = config.StoredApplyTx(ctx, tx, loadedConfig(), identity.Name)
	return name, "", err
}
