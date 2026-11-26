// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package recordmerge

import (
	"errors"
	"net/http"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// MergedIntoKey is the wire key a merged-away record's redirect pointer travels
// under, on the 409 and in the audit rows.
const MergedIntoKey = "merged_into_id"

// WriteAlreadyMerged answers 409 `already_merged` with the pointer to the
// survivor when err is an AlreadyMergedError, and reports whether it did. The
// pointer sits on the source row the caller just proved they can address, so
// echoing it discloses nothing new.
func WriteAlreadyMerged(w http.ResponseWriter, r *http.Request, err error) bool {
	alreadyMerged, ok := errors.AsType[*AlreadyMergedError](err)
	if !ok {
		return false
	}
	e := &httperr.DetailedError{
		Status: http.StatusConflict, Code: "already_merged", Detail: alreadyMerged.Error(),
	}
	if !alreadyMerged.IntoID.IsZero() {
		e.Details = map[string]any{MergedIntoKey: alreadyMerged.IntoID.String()}
	}
	httperr.Write(w, r, e)
	return true
}
