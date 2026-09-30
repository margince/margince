// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Removing an entry is a correction of the sheet, so it takes the update grant
// and nothing else opens it: not create, which only adds, and not delete, which
// no role holds on the sheet at all.
func TestPrepareModelRateKeyTakesTheUpdateGrantOnly(t *testing.T) {
	key := ModelRateKey{Provider: "anthropic", ModelID: "m", Lane: LaneChat}
	if _, err := prepareModelRateKey(modelRateCtx(principal.ObjectGrant{Update: true}), key); err != nil {
		t.Fatalf("update grant: prepareModelRateKey = %v, want admitted", err)
	}
	for name, g := range map[string]principal.ObjectGrant{
		"create only": {Create: true},
		"delete only": {Delete: true},
		"read only":   {Read: true},
		"no grant":    {},
	} {
		t.Run("refuses "+name, func(t *testing.T) {
			_, err := prepareModelRateKey(modelRateCtx(g), key)
			if !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("prepareModelRateKey = %v, want ErrPermissionDenied", err)
			}
		})
	}
}

// A blank half of the key, or a lane the sheet does not file under, is a 422
// naming the field: the caller supplied it, so the caller can fix it. The
// trimmed key is what the delete runs on, so surrounding whitespace cannot
// turn a real entry into a miss.
func TestPrepareModelRateKeyRefusesABlankOrUnknownField(t *testing.T) {
	ctx := modelRateCtx(principal.ObjectGrant{Update: true})
	for field, key := range map[string]ModelRateKey{
		"provider": {Provider: " ", ModelID: "m", Lane: LaneChat},
		"model_id": {Provider: "anthropic", ModelID: "", Lane: LaneChat},
		"lane":     {Provider: "anthropic", ModelID: "m", Lane: "vision"},
	} {
		t.Run(field, func(t *testing.T) {
			_, err := prepareModelRateKey(ctx, key)
			var invalid *RateValidationError
			if !errors.As(err, &invalid) {
				t.Fatalf("prepareModelRateKey = %v, want a validation error", err)
			}
			if invalid.Field != field {
				t.Errorf("validation names field %q, want %q", invalid.Field, field)
			}
		})
	}
	got, err := prepareModelRateKey(ctx, ModelRateKey{Provider: " anthropic ", ModelID: " m ", Lane: LaneEmbeddings})
	if err != nil {
		t.Fatalf("prepareModelRateKey = %v, want admitted", err)
	}
	if want := (ModelRateKey{Provider: "anthropic", ModelID: "m", Lane: LaneEmbeddings}); got != want {
		t.Errorf("prepared key = %+v, want %+v", got, want)
	}
}
