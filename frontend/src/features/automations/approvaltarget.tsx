// What a request is about, and who answered it, as a reader needs them.
//
// Both open in a new tab because they sit inside a drawer. Following a link in
// place would close the request the reader is looking at.

import { EntityRef } from "../../components/entityref";
import { useT } from "../../i18n";
import type { Approval } from "../../lib/approvals.queries";
import { isEntityKind } from "../../lib/entity";

// ApprovalTarget names the record the request changes, as a link to it.
export function ApprovalTarget({ approval }: Readonly<{ approval: Approval }>) {
  const t = useT();
  const type = approval.target_entity_type;
  const id = approval.target_entity_id;
  if (!type || !id || !isEntityKind(type)) return null;
  return (
    <div className="field">
      <span className="t-label">{t("decision.target")}</span>
      <p className="approval-detail-value t-body">
        <EntityRef kind={type} id={id} name={approval.target_label} newTab />
      </p>
    </div>
  );
}

// ApprovalDecider names the colleague who approved or rejected the request.
// Expiry and withdrawal record nobody, so they show nothing here.
export function ApprovalDecider({
  approval,
}: Readonly<{ approval: Approval }>) {
  const t = useT();
  if (!approval.decided_by) return null;
  if (approval.status !== "approved" && approval.status !== "rejected") {
    return null;
  }
  return (
    <div className="field">
      <span className="t-label">
        {t(
          approval.status === "approved"
            ? "decision.approvedBy"
            : "decision.rejectedBy",
        )}
      </span>
      <p className="approval-detail-value t-body">
        <EntityRef kind="user" id={approval.decided_by} />
      </p>
    </div>
  );
}
