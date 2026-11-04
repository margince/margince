// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// mailCaptureSeam is capture's answer to identity's role changes: the one
// decider connect and sync ask, and the connections a role change reaches.
type mailCaptureSeam struct{}

func (mailCaptureSeam) Refuses(perms principal.Permissions, seat principal.SeatType) bool {
	return capture.CaptureRefusal(perms, seat) != nil
}

func (mailCaptureSeam) LiveMailboxesTx(ctx context.Context, tx pgx.Tx, userIDs []ids.UUID) (map[ids.UUID]int, error) {
	return capture.LiveConnectionsTx(ctx, tx, userIDs)
}

func (mailCaptureSeam) RecheckSeatsTx(ctx context.Context, tx pgx.Tx, userIDs []ids.UUID) error {
	return capture.RecheckMembersTx(ctx, tx, userIDs)
}

type memberManager struct{ service *identity.Service }

func newMemberManager(db *database.DB) memberManager {
	engine := newBulkEngine(db, nil)
	return memberManager{identity.NewServiceFor(db).WithMemberWork(memberWork{newHandoverSeam(engine)})}
}

func (m memberManager) Preview(ctx context.Context, source ids.UUID, target *ids.UUID, merge bool) (crmcontracts.MemberRetirementPreview, error) {
	actor, err := m.service.MemberAdministrator(ctx)
	if err != nil {
		return crmcontracts.MemberRetirementPreview{}, err
	}
	return m.service.PreviewMemberRetirement(ctx, actor, ids.From[ids.UserKind](source), target, merge)
}

func (m memberManager) Merge(ctx context.Context, source, target ids.UUID, token string) error {
	actor, err := m.service.MemberAdministrator(ctx)
	if err != nil {
		return err
	}
	return m.service.RetireMember(ctx, actor, ids.From[ids.UserKind](source), &target, token, true)
}

// MergeMembers uses the confirmed tool command so HTTP and MCP share admission.
func (s Server) MergeMembers(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.MergeMembersParams) {
	serveToolCommand(w, r, s.toolRegistry.Invoke, "merge_members", reservedArgs{params.IdempotencyKey, params.XApprovalToken}, pathArgument{memberSourceArgument, id.String()})
}

// PreviewMemberRetirement exposes the same dependency preview as the agent tool.
func (s Server) PreviewMemberRetirement(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.PreviewMemberRetirementParams) {
	serveTool(w, r, s.toolRegistry.Invoke, "preview_member_retirement", struct {
		SourceID crmcontracts.Id  `json:"source_id"`
		TargetID *crmcontracts.Id `json:"to_user_id,omitempty"`
		Mode     string           `json:"mode"`
	}{id, params.ToUserId, retirementPreviewMode(params.Mode)})
}

func retirementPreviewMode(mode *crmcontracts.PreviewMemberRetirementParamsMode) string {
	if mode != nil {
		return string(*mode)
	}
	return "remove"
}
