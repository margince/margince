// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Unrecorded asks Require's question and answers its error, for a read whose
// refusal the call degrades over, such as a section it withholds. The ctx it
// returns carries no recorder, so the gates of that read record nothing: the
// call does not need what they ask.
func Unrecorded(ctx context.Context, object string, action principal.Action) (context.Context, error) {
	if err := probe(ctx, object, action); err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, checkRecorderKey{}, (*checkRecorder)(nil)), nil
}
