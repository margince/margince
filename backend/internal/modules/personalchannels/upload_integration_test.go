// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package personalchannels

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
	if _, err := e.store.ReadUploads(e.as(e.other, repGrant), []ids.UUID{up.ID}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want not found for another member's upload", err)
	}
	files, err := e.store.ReadUploads(e.as(e.rep, repGrant), []ids.UUID{up.ID})
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
	if n, err := e.store.SweepUploads(e.system(), e.now); err != nil || n != 0 {
		t.Fatalf("at 23h59 swept %d, %v; want 0", n, err)
	}
	e.exec(t, `UPDATE personal_channel_upload SET created_at = $1`, e.now.Add(-24*time.Hour-time.Minute))
	if n, err := e.store.SweepUploads(e.system(), e.now); err != nil || n != 1 {
		t.Fatalf("at 24h01 swept %d, %v; want 1", n, err)
	}
}

func TestOnlyTheSweepMayDeleteStagedFiles(t *testing.T) {
	e := setupChannels(t)
	if _, err := e.store.SweepUploads(e.as(e.rep, repGrant), e.now); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a member ran the sweep: err = %v", err)
	}
}
