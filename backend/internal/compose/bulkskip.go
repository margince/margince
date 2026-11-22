// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// bulkSkipFor turns a row's refusal into why it is left alone. A refusal from
// a single-record rule carries that rule's own code, so a client can say it in
// the reader's language. The English message stays beside it for a code the
// client does not know. An error that is no refusal, such as a lost connection
// or a bug, aborts the change and is not reported as one row's fault.
func bulkSkipFor(err error) (crmcontracts.BulkSkip, error) {
	var anchor *contacts.AnchorProtectedError
	if restore, ok := errors.AsType[*storekit.RestoreRefusal](err); ok {
		return restoreSkip(restore), nil
	}
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		return skipped(crmcontracts.BulkSkipReasonNotFound), nil
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return skipped(crmcontracts.BulkSkipReasonNotWritable), nil
	case errors.Is(err, apperrors.ErrVersionSkew):
		return skipped(crmcontracts.BulkSkipReasonChangedSincePreview), nil
	case errors.As(err, &anchor):
		return skipped(crmcontracts.BulkSkipReasonAnchorCompany), nil
	}
	fault, classified := httperr.Classify(err)
	if !classified || fault.Transient() || fault.Status < http.StatusBadRequest || fault.Status >= http.StatusInternalServerError {
		return crmcontracts.BulkSkip{}, err
	}
	code := fault.Code
	if len(fault.Fields) > 0 {
		code = fault.Fields[0].Code
	}
	skip := crmcontracts.BulkSkip{Reason: crmcontracts.BulkSkipReasonRefused, Code: &code, Message: &fault.Detail}
	if len(fault.Details) > 0 {
		skip.Params = &fault.Details
	}
	return skip, nil
}

func affectedIDs(changed []crmcontracts.BulkSampleRow) []openapi_types.UUID {
	out := make([]openapi_types.UUID, len(changed))
	for i, row := range changed {
		out[i] = row.Id
	}
	return out
}

func wireOwner(id *ids.UUID) *openapi_types.UUID {
	if id == nil {
		return nil
	}
	wire := openapi_types.UUID(*id)
	return &wire
}
