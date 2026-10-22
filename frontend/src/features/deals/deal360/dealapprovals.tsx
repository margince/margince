import { Trash2 } from "lucide-react";
import type { components } from "../../../api/schema";
import { ActionRow } from "../../../design-system/actionrow";
import { Button } from "../../../design-system/atoms";
import { IconAction } from "../../../design-system/iconaction";
import { Panel, PanelBody } from "../../../design-system/panel";
import { provenanceOf } from "../../../design-system/provenanceof";
import { Row } from "../../../design-system/stack";
import { AutonomyDot, ProvenanceTag } from "../../../design-system/trust";
import { useT } from "../../../i18n";
import { approvalLabel } from "../../../lib/approvalkindlabel";
import { approvalDotTier, useAgentTierMap } from "../../../lib/autonomy";
import { useViewerId } from "../../../lib/session";

type Approval = components["schemas"]["Approval"];

// The live confirm-first staging queue for this deal. Renders nothing while
// the deal has no pending approval.
export function DealApprovals({
  approvals,
  decide,
}: Readonly<{
  approvals: Approval[];
  decide: (input: {
    approvalId: string;
    verdict: "approve" | "reject";
  }) => void;
}>) {
  const t = useT();
  const tierMap = useAgentTierMap();
  const viewerId = useViewerId();
  if (approvals.length === 0) {
    return null;
  }
  return (
    <Panel title={t("deal.pendingApprovals")}>
      {/* The interval between two staged cards belongs to the stack rather
          than to each card, so the last one does not pay for a neighbour it
          does not have. */}
      <PanelBody className="form-stack">
        {approvals.map((approval) => (
          <div key={approval.id} className="staging-card">
            <Row gap="2" wrap={false}>
              <AutonomyDot tier={approvalDotTier(approval.kind, tierMap)} />
              {/* The same two facts the approvals inbox states, said the same
                way, not the wire names `advance_deal` and `agent:capture`. */}
              <span className="t-label">{approvalLabel(approval, t)}</span>
              <ProvenanceTag
                provenance={provenanceOf(approval.proposed_by, viewerId)}
              />
            </Row>
            <ActionRow
              className="approval-gate"
              primary={
                <Button
                  variant="primary"
                  onClick={() =>
                    decide({ approvalId: approval.id, verdict: "approve" })
                  }
                >
                  {t("trust.accept")}
                </Button>
              }
            >
              {/* Dismiss sends the `reject` verdict, the same answer the
                decision card's trash can gives, so it wears the same glyph. */}
              <IconAction
                label={t("trust.dismiss")}
                icon={<Trash2 aria-hidden />}
                onClick={() =>
                  decide({ approvalId: approval.id, verdict: "reject" })
                }
              />
            </ActionRow>
          </div>
        ))}
      </PanelBody>
    </Panel>
  );
}
