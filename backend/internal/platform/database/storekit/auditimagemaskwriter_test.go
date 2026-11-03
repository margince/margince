// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// errSettingRefused stands for whatever the database answers when the setting
// cannot be put on. The writer must not reach the rewrite after it.
var errSettingRefused = errors.New("the setting was refused")

// A row whose images hold no credential spends no rewrite.
//
// The trail admits one change per row, and only under its setting. A pass that
// wrote every row it read would spend that on rows with nothing to mask.
//
// The fake fails the test if any statement reaches it.
func TestMaskingAnAuditRowWithNoCredentialWritesNothing(t *testing.T) {
	tx := &fakeTx{}
	wrote, err := MaskAuditImagesTx(context.Background(), tx, AuditImageRow{
		ID:         ids.NewV7(),
		EntityType: "activity",
		Before:     []byte(`{"body":"the quarterly figures"}`),
		After:      []byte(`{"body":"the quarterly figures, corrected"}`),
	})
	if err != nil {
		t.Fatalf("masking a clean row: %v", err)
	}
	if wrote {
		t.Error("a row with no credential reported a rewrite")
	}
	if tx.execSQL != "" {
		t.Errorf("a clean row reached the trail with %q", tx.execSQL)
	}
}

// A record type whose audit rows carry no mail text is left alone, images and
// all. The masking rules never run against a shape they do not know.
func TestMaskingAnAuditRowOfAnUnmaskedTypeWritesNothing(t *testing.T) {
	tx := &fakeTx{}
	wrote, err := MaskAuditImagesTx(context.Background(), tx, AuditImageRow{
		ID:         ids.NewV7(),
		EntityType: "deal",
		Before:     []byte(`{"body":"Passwort: Hunter2-Example!"}`),
		After:      nil,
	})
	if err != nil {
		t.Fatalf("masking a row of an unmasked type: %v", err)
	}
	if wrote || tx.execSQL != "" {
		t.Errorf("a %q row was rewritten: wrote=%v sql=%q", "deal", wrote, tx.execSQL)
	}
}

// A malformed image is reported, not skipped.
//
// Skipping would leave a credential in a row the pass counted as done. The next
// run reads the same stored state, so it would skip the row again.
func TestMaskingARowWithAMalformedImageSaysSo(t *testing.T) {
	tx := &fakeTx{}
	_, err := MaskAuditImagesTx(context.Background(), tx, AuditImageRow{
		ID:         ids.NewV7(),
		EntityType: "activity",
		Before:     []byte(`{"body": NOT JSON`),
		After:      nil,
	})
	if err == nil {
		t.Fatal("a malformed before image was accepted")
	}
	if !strings.Contains(err.Error(), "before image") {
		t.Errorf("the error does not name which image: %v", err)
	}
	if tx.execSQL != "" {
		t.Errorf("a malformed row reached the trail with %q", tx.execSQL)
	}
}

// The setting goes on before the rewrite, carrying the name the trigger reads.
// A rewrite admitted by another spelling is one the trail refuses.
func TestMaskingACredentialAdmitsItselfByTheTriggersOwnSetting(t *testing.T) {
	tx := &fakeTx{execErr: errSettingRefused}
	_, err := MaskAuditImagesTx(context.Background(), tx, AuditImageRow{
		ID:         ids.NewV7(),
		EntityType: "activity",
		Before:     []byte(`{"body":"Passwort: Hunter2-Example!"}`),
		After:      nil,
	})
	if err == nil {
		t.Fatal("a refused setting was accepted")
	}
	if !strings.Contains(tx.execSQL, "set_config") {
		t.Errorf("the first statement was not the setting: %q", tx.execSQL)
	}
	if len(tx.execArgs) == 0 || tx.execArgs[0] != AuditRepairSetting {
		t.Errorf("the setting bound %v, want %q", tx.execArgs, AuditRepairSetting)
	}
}
