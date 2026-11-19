import { Button, EmptyState, PendingBody } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { navigate } from "../../lib/router";
import "./share.css";

import type { RecordType } from "../../components/share.parts";
import { ShareScreenBody } from "./share.body";
import { useShareStanding } from "./sharerecord";

// The record-share screen: grant a user or team read/write on this one record,
// list and revoke the manual grants. For a contact or company, the panel above
// the grants lists everyone who can see the record and why.

const RECORD_TYPES: readonly RecordType[] = [
  "contact",
  "company",
  "deal",
  "lead",
  "project",
];

function isRecordType(value: string): value is RecordType {
  return (RECORD_TYPES as readonly string[]).includes(value);
}

// The per-screen "Share" affordance, extracted from four verbatim copies that
// lived inline in the contact/company/deal/lead 360 action clusters
// (mirrors EditAction/ArchiveAction — a thin prop component owning its label
// and its navigation, nothing else). recordType is the narrow union, so a
// screen can't wire a share link to a record kind the route can't resolve.
export function ShareAction({
  recordType,
  recordId,
  disabledReasonId,
  contactsOnly = false,
}: Readonly<{
  contactsOnly?: boolean;
  recordType: RecordType;
  recordId: string;
  // Why this action is unavailable, when it is. STATE-4a settles the
  // absent-vs-disabled question by CAUSE: a control blocked by STATE
  // rather than permission — an archived record — stays visible and
  // disabled WITH the reason, because the reason is the information and
  // hiding the control hides a fact the reader needs.
  disabledReasonId?: string;
}>) {
  const t = useT();
  return (
    <Button
      reasonId={disabledReasonId}
      data-testid={contactsOnly ? "share-company-contacts" : "share-record"}
      onClick={() =>
        navigate({
          screen: "share",
          id: contactsOnly ? "company-contacts" : recordType,
          id2: recordId,
        })
      }
    >
      {t(contactsOnly ? "share.companyContacts" : "record.share")}
    </Button>
  );
}

// recordType arrives as the raw 3rd URL segment (App.tsx passes it straight
// through). Guard it before rendering the screen: an unknown kind gets an
// honest empty state, never a share form wired to a record type the contract
// cannot resolve. useT() is the only hook here, so the early return is
// rules-of-hooks-safe; all the query/mutation hooks live in ShareScreenBody,
// which mounts only for a valid kind.
export function ShareScreen({
  recordType,
  recordId,
}: Readonly<{ recordType: string; recordId: string }>) {
  const t = useT();
  const contactsOnly = recordType === "company-contacts";
  const kind = contactsOnly ? "company" : recordType;
  if (!isRecordType(kind)) {
    return (
      <div className="wrap">
        <EmptyState>{t("share.unknownRecord")}</EmptyState>
      </div>
    );
  }
  return (
    <ShareScreenGate
      recordType={kind}
      recordId={recordId}
      contactsOnly={contactsOnly}
    />
  );
}

// The record is read before anything is offered. One the reader cannot see is
// not offered for sharing. One they may not share shows who can see it, and why
// they cannot change that.
function ShareScreenGate({
  recordType,
  recordId,
  contactsOnly,
}: Readonly<{
  recordType: RecordType;
  recordId: string;
  contactsOnly: boolean;
}>) {
  const t = useT();
  const standing = useShareStanding(recordType, recordId, contactsOnly);
  if (standing.state === "missing") {
    return (
      <div className="wrap">
        <EmptyState>{t("share.missingRecord")}</EmptyState>
      </div>
    );
  }
  if (standing.state === "refused") {
    return (
      <div className="wrap">
        <EmptyState>{standing.refusal}</EmptyState>
      </div>
    );
  }
  if (standing.state === "pending") {
    return (
      <div className="wrap">
        <PendingBody label={t("share.title")} />
      </div>
    );
  }
  return (
    <ShareScreenBody
      recordType={recordType}
      recordId={recordId}
      contactsOnly={contactsOnly}
      refusal={standing.refusal}
      mayRetract={standing.mayRetract}
    />
  );
}
