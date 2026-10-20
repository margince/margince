// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import "github.com/margince/margince/backend/internal/platform/database/storekit"

// dsrUpdateChangesNothing answers whether the update leaves the request as it
// is, so a save of the stored status, assignee and resolution writes no version
// and no audit row. It stages what the UPDATE would write, with the same rules
// for an absent value and for an explicit clear.
func dsrUpdateChangesNothing(current dsrRow, in UpdateDSRInput) bool {
	p := storekit.NewPatch()
	if in.Status != nil {
		p.Set(fieldStatus, current.Status, *in.Status)
	}
	switch {
	case in.ClearAssignee:
		p.Set(fieldAssigneeID, current.AssigneeID, nil)
	case in.AssigneeID != nil:
		p.Set(fieldAssigneeID, current.AssigneeID, *in.AssigneeID)
	}
	switch {
	case in.ClearResolution:
		p.Set(fieldResolution, current.Resolution, nil)
	case in.Resolution != nil:
		p.Set(fieldResolution, current.Resolution, *in.Resolution)
	}
	return p.Empty()
}
