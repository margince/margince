// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/mailsubject"
)

// Every reply and forward marker the mail code strips is refused as a key,
// read from the one list rather than a copy of it.
func TestEveryMailSubjectMarkerIsRefusedAsAKey(t *testing.T) {
	for _, word := range append(mailsubject.PrefixWords(), "sv") {
		err := keyWordRefusal(context.Background(), nil, strings.ToUpper(word))
		var refused *ProjectKeyRefusedError
		if !errors.As(err, &refused) || refused.Code != "key_is_common_word" {
			t.Errorf("key %q answered %v, want key_is_common_word", word, err)
		}
	}
}
