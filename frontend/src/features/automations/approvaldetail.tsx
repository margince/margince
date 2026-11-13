import { useId } from "react";
import { AppLink } from "../../design-system/applink";
import {
  SummaryFields,
  summaryPartsWithout,
  summarySentence,
} from "../../design-system/approvalsummary";
import { Disclosure, Modal } from "../../design-system/atoms";
import { DecisionEvidence } from "../../design-system/decisioncard.content";
import {
  decisionEvidence,
  withoutRepeatedLead,
} from "../../design-system/decisioncard.payload";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";
import { QueryGate } from "../../design-system/querystates";
import { formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { approvalHref } from "../../lib/approvalhref";
import { summaryWords } from "../../lib/approvalpreviewfields";
import { useApproval } from "../../lib/approvalread";
import type { Approval } from "../../lib/approvals.queries";
import { useViewerZone } from "../../lib/viewerzone";
import { resolveDisplay, stagedDayFormatter } from "./approvaldisplay";
import { useApprovalNames } from "./approvalresponsibility";
import { ApprovalDecider, ApprovalTarget } from "./approvaltarget";
import "./approvalrow.css";
import "../../design-system/querystates.css";

/**
 * The row's "view everything" dialog: the proposal's fields, the evidence
 * behind them and the timestamps the row leaves out.
 */
export function ApprovalDetailModal({
  approvalId,
  open,
  onClose,
}: Readonly<{ approvalId: string; open: boolean; onClose: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  // An approval is a thing the reader must act on, so its timestamps belong on
  // the reader's own clock rather than the record's.
  const zone = useViewerZone();
  const headingId = useId();
  const detail = useApproval(approvalId, open);
  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId} intent="drawer">
      <DrawerHead>
        <Heading size="large" id={headingId} className="t-h2">
          {t("decision.detail")}
        </Heading>
      </DrawerHead>
      <DrawerBody>
        {open && (
          <QueryGate query={detail} pendingLabel={t("decision.detailLoading")}>
            {(approval) => (
              <>
                <ApprovalDetailBody
                  approval={approval}
                  locale={locale}
                  zone={zone}
                />
                {/* A pending request is answered on its decision card; the
                    detail only reads it. */}
                {approval.status === "pending" && (
                  <AppLink
                    href={approvalHref(approval.id)}
                    className="link-button"
                    onClick={onClose}
                  >
                    {t("decision.openToDecide")}
                  </AppLink>
                )}
              </>
            )}
          </QueryGate>
        )}
      </DrawerBody>
    </Modal>
  );
}

/**
 * What "view everything" shows, in the order a reader reads a decision: the
 * question, the named fields, when it was asked, then the quoted evidence.
 *
 * The summary leads, because it is the sentence a human reads before deciding.
 * The target version and the proposing agent are left out. They name no
 * contact and describe no change.
 */
function ApprovalDetailBody({
  approval,
  locale,
  zone,
}: Readonly<{
  approval: Approval;
  locale: ReturnType<typeof useLocale>["locale"];
  zone: string;
}>) {
  const t = useT();
  const change = (approval.proposed_change ?? {}) as Record<string, unknown>;
  const approvalNames = useApprovalNames([approval]);
  const readerZone = useViewerZone();
  const display = resolveDisplay(
    approval.kind,
    change,
    t,
    stagedDayFormatter(locale, readerZone),
    approvalNames,
    approval.requested_decider_id,
    approval.assignee_inactive,
    approval.preview,
    locale,
    readerZone,
  );
  const sentence = summarySentence(approval);
  const named = withoutRepeatedLead(display, sentence).filter(
    (entry) => entry.value !== null,
  );
  // The raw payload shows once. A kind that named its fields keeps the keys
  // as reference material; a kind that named none shows them as the content.
  const rawPayload = <RawPayload change={change} />;
  return (
    <div className="form-stack">
      {sentence && <p className="t-body">{sentence}</p>}
      <SummaryFields
        parts={summaryPartsWithout(
          approval.summary_parts,
          new Set(named.map((entry) => entry.field)),
        )}
        words={summaryWords(t)}
      />
      <ApprovalTarget approval={approval} />
      <ApprovalDecider approval={approval} />
      {named.length > 0
        ? named.map((entry) => (
            <FieldLine
              key={entry.field}
              name={entry.label}
              value={entry.value ?? ""}
            />
          ))
        : rawPayload}
      {askedOn(approval, locale, zone, t).map(([label, value]) => (
        <FieldLine key={label} name={label} value={value} />
      ))}
      <DecisionEvidence
        evidence={decisionEvidence(approval)}
        collapsed={false}
      />
      {named.length > 0 && (
        <Disclosure summary={t("decision.detailTechnical")}>
          <div className="form-stack">{rawPayload}</div>
        </Disclosure>
      )}
    </div>
  );
}

// The payload as the wire spells it. A nested document prints as its JSON
// rather than as "[object Object]".
function RawPayload({
  change,
}: Readonly<{ change: Readonly<Record<string, unknown>> }>) {
  return (
    <>
      {Object.entries(change).map(([key, value]) => (
        <FieldLine
          key={key}
          name={key}
          value={typeof value === "string" ? value : JSON.stringify(value)}
        />
      ))}
    </>
  );
}

/**
 * When the question was raised, and when it was settled. How long a proposal
 * has waited is something a reader weighs, so it is shown, not hidden.
 */
function askedOn(
  approval: Approval,
  locale: ReturnType<typeof useLocale>["locale"],
  zone: string,
  t: ReturnType<typeof useT>,
): [string, string][] {
  const asked: [string, string][] = [
    [
      t("decision.detailAsked"),
      formatDateTime(approval.created_at, locale, zone),
    ],
  ];
  if (approval.decided_at) {
    asked.push([
      t("decision.detailDecided"),
      formatDateTime(approval.decided_at, locale, zone),
    ]);
  }
  return asked;
}

// One label over one value, set as prose whether the value is a contact's own
// words or a uuid the wire spells.
function FieldLine({ name, value }: Readonly<{ name: string; value: string }>) {
  return (
    <div className="field">
      <span className="t-label">{name}</span>
      <p className="approval-detail-value t-body">{value}</p>
    </div>
  );
}
