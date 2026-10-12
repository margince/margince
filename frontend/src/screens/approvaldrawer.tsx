import { useId } from "react";
import { routeHash } from "../app/router";
import { hashWithParams, useUrlParams } from "../app/urlstate";
import { Modal } from "../design-system/atoms";
import { DrawerBody, DrawerHead } from "../design-system/drawerbands";
import { Heading } from "../design-system/heading";
import { useT } from "../i18n";
import { ApprovalRow } from "./approvalrow";
import { ApprovalBundleReview } from "./worklist.bundle";
import { useApproval, worklistKey } from "./worklist.queries";

/** The address parameter that opens one decision over Home. */
export const APPROVAL_PARAM = "approval";

/**
 * The address of one pending decision. Every surface that announces a decision
 * links here, so the reader arrives at that decision and not at the queue.
 */
export function approvalHref(id: string): string {
  return hashWithParams(
    routeHash({ screen: "home" }),
    new Map([[APPROVAL_PARAM, id]]),
  );
}

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
  // Fetched only once open: a queue of decisions would otherwise fire one read
  // per row to fill cards nobody has opened.
  const approval = useApproval(approvalId, open && approvalId !== "");
  const usable = approval.data?.kind ? approval.data : undefined;
  // A notice outlives its decision, so a link can arrive after somebody
  // answered it. That reads as the answer, with nothing left to press.
  const pending = usable?.status === "pending";
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
        {usable?.bundle_id && pending ? (
          <ApprovalBundleReview approval={usable} />
        ) : usable ? (
          <ApprovalRow
            approval={usable}
            decided={!pending}
            extraInvalidateKeys={[worklistKey]}
            onAlreadyDecided={onClose}
          />
        ) : (
          // Said rather than left blank: a drawer that opens onto nothing reads
          // as a broken link, and a decision that is gone or not the reader's
          // reads the same as one still loading.
          <p>
            {approval.isPending
              ? t("worklist.decision.loading")
              : t("worklist.decision.unavailable")}
          </p>
        )}
      </DrawerBody>
    </Modal>
  );
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
