// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package personalchannels

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

var pngHeader = []byte("\x89PNG\r\n\x1a\n0000000000000000")

func (e *channelEnv) on(t *testing.T, user ids.UUID) {
	t.Helper()
	if _, err := e.store.SetEnabled(e.as(user, repGrant), true); err != nil {
		t.Fatal(err)
	}
}

func TestAnUploadIsTypedFromItsBytesAndHashed(t *testing.T) {
	e := setupChannels(t)
	e.on(t, e.rep)
	up, err := e.store.StageUpload(e.as(e.rep, repGrant), "../../invoice.png", bytes.NewReader(pngHeader))
	if err != nil {
		t.Fatalf("staging: %v", err)
	}
	if up.ContentType != "image/png" || up.Size != int64(len(pngHeader)) || len(up.SHA256) != 64 {
		t.Fatalf("upload = %+v, want image/png, its size and a sha256", up)
	}
	if up.Filename == "../../invoice.png" {
		t.Fatal("the stored filename kept its path")
	}
}

func TestAnEmptyOrOversizedUploadIsRefused(t *testing.T) {
	e := setupChannels(t)
	e.on(t, e.rep)
	if _, err := e.store.StageUpload(e.as(e.rep, repGrant), "empty.txt", bytes.NewReader(nil)); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("empty: err = %v, want invalid argument", err)
	}
	big := bytes.NewReader(make([]byte, maxUploadBytes+1))
	if _, err := e.store.StageUpload(e.as(e.rep, repGrant), "big.bin", big); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("oversized: err = %v, want invalid argument", err)
	}
}

func TestAnUploadIsRefusedWhileTheConnectionIsOff(t *testing.T) {
	e := setupChannels(t)
	if _, err := e.store.StageUpload(e.as(e.rep, repGrant), "a.png", bytes.NewReader(pngHeader)); !errors.Is(err, ErrOff) {
		t.Fatalf("err = %v, want ErrOff", err)
	}
}

func TestAnotherMembersUploadIsNotFound(t *testing.T) {
	e := setupChannels(t)
	e.on(t, e.rep)
	e.on(t, e.other)
	up, err := e.store.StageUpload(e.as(e.rep, repGrant), "a.png", bytes.NewReader(pngHeader))
	if err != nil {
		t.Fatal(err)
	}
	read := func(user ids.UUID) ([]extension.InboundFile, error) {
		var files []extension.InboundFile
		err := e.store.db.Tx(e.as(user, repGrant), func(tx pgx.Tx) error {
			var err error
			files, err = readUploadsTx(e.as(user, repGrant), tx, user, []ids.UUID{up.ID})
			return err
		})
		return files, err
	}
	if _, err := read(e.other); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want not found for another member's upload", err)
	}
	files, err := read(e.rep)
	if err != nil || len(files) != 1 || !bytes.Equal(files[0].Body, pngHeader) {
		t.Fatalf("own upload = %v, %v", files, err)
	}
}

func TestAnUnusedUploadIsSweptAfterADay(t *testing.T) {
	e := setupChannels(t)
	e.on(t, e.rep)
	if _, err := e.store.StageUpload(e.as(e.rep, repGrant), "a.png", bytes.NewReader(pngHeader)); err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE personal_channel_upload SET created_at = $1`, e.now.Add(-23*time.Hour-59*time.Minute))
	if n, err := sweepFiles(e); err != nil || n != 0 {
		t.Fatalf("at 23h59 swept %d, %v; want 0", n, err)
	}
	e.exec(t, `UPDATE personal_channel_upload SET created_at = $1`, e.now.Add(-24*time.Hour-time.Minute))
	if n, err := sweepFiles(e); err != nil || n != 1 {
		t.Fatalf("at 24h01 swept %d, %v; want 1", n, err)
	}
}

func TestOnlyTheSweepMayDeleteStagedFiles(t *testing.T) {
	e := setupChannels(t)
	if _, err := sweepFilesAs(e.as(e.rep, repGrant), e); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a member ran the sweep: err = %v", err)
	}
}

func sweepFilesAs(ctx context.Context, e *channelEnv) (int, error) {
	files, _, err := Sweep(ctx, e.store.db, &notDelivered{settled: map[ids.UUID]string{}}, e.now)
	return files, err
}

func sweepFiles(e *channelEnv) (int, error) { return sweepFilesAs(e.system(), e) }
