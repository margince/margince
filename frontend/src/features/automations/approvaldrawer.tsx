import { useId } from "react";
import { Modal } from "../../design-system/atoms";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";
import { useT } from "../../i18n";
import { useUrlParams } from "../../lib/urlstate";
import { ApprovalBundleReview } from "../worklist/worklist.bundle";
import { useApproval, worklistKey } from "../worklist/worklist.queries";
import { ApprovalProposal, ApprovalRow } from "./approvalrow";

export { APPROVAL_PARAM, approvalHref } from "../../lib/approvalhref";

import { APPROVAL_PARAM } from "../../lib/approvalhref";

/**
 * One decision, fetched whole and answered in a drawer: the same ApprovalRow
 * the record page draws, posting to the same endpoint, so no surface that
 * opens it adds authority of its own.
 */
export function ApprovalDecisionDrawer({
  approvalId,
  open,
  onClose,
  returnFocusTo,
}: Readonly<{
  approvalId: string;
  open: boolean;
  onClose: () => void;
  returnFocusTo?: () => HTMLElement | null;
}>) {
  const t = useT();
  const titleId = useId();
  return (
    <Modal
      open={open}
      onClose={onClose}
      labelledBy={titleId}
      intent="drawer-reading"
      returnFocusTo={returnFocusTo}
    >
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2">
          {t("worklist.decision.title")}
        </Heading>
      </DrawerHead>
      <DrawerBody>
        <ApprovalDecision approvalId={approvalId} onAlreadyDecided={onClose} />
      </DrawerBody>
    </Modal>
  );
}

/**
 * One decision, answered where it is drawn: the drawer's body, and the body of
 * any surface that already has the room. Mounted only where it is shown, so a
 * queue of decisions fires no read per row nobody opened.
 */
export function ApprovalDecision({
  approvalId,
  onAlreadyDecided,
}: Readonly<{ approvalId: string; onAlreadyDecided?: () => void }>) {
  const t = useT();
  const approval = useApproval(approvalId);
  const usable = approval.data?.kind ? approval.data : undefined;
  // A notice outlives its decision, so a link can arrive after somebody
  // answered it. That reads as the answer, with nothing left to press.
  const pending = usable?.status === "pending";
  if (usable?.bundle_id && pending) {
    return <ApprovalBundleReview approval={usable} />;
  }
  if (usable) {
    return (
      <ApprovalRow
        approval={usable}
        decided={!pending}
        extraInvalidateKeys={[worklistKey]}
        onAlreadyDecided={onAlreadyDecided}
      />
    );
  }
  // Said rather than left blank, which reads as a broken link. A decision that
  // is gone or not the reader's would read as one still loading.
  return (
    <p>
      {approval.isPending
        ? t("worklist.decision.loading")
        : t("worklist.decision.unavailable")}
    </p>
  );
}

/**
 * What a pending decision would do, without its verbs, for a card that asks
 * the question and answers it in the drawer. The read is the one the drawer
 * makes, so opening it after costs nothing. Nothing until it has an answer.
 */
export function ApprovalProposalPreview({
  approvalId,
  className,
}: Readonly<{ approvalId: string; className?: string }>) {
  const approval = useApproval(approvalId);
  const usable = approval.data?.kind ? approval.data : undefined;
  return usable?.status === "pending" ? (
    <ApprovalProposal approval={usable} className={className} />
  ) : null;
}

/** The decision the address names, open over Home until the reader closes it. */
export function LinkedApprovalDrawer() {
  const [params, setParams] = useUrlParams();
  const approvalId = params.get(APPROVAL_PARAM) ?? "";
  const close = () => {
    const next = new Map(params);
    next.delete(APPROVAL_PARAM);
    setParams(next);
  };
  return (
    <ApprovalDecisionDrawer
      approvalId={approvalId}
      open={approvalId !== ""}
      onClose={close}
    />
  );
}
