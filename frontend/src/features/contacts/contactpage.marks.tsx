import type { ReactNode } from "react";
import type { ActivityGate } from "../../components/activitygate";
import { ContactCaseEntry } from "../../components/contactcase.entry";
import { ContactRestrictedMark } from "../../components/stops";
import { ViewOnly } from "../../components/viewonly";
import { Badge } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { dealRoleLabel } from "../../lib/recordlabels";
import { RecordAccess } from "../records/recordaccess";
import type { Contact360 } from "./contactpage";

// Role and record access do not infer an obligation from exchange order.
export function ContactMarks({
  view,
}: Readonly<{ view: Contact360 }>): ReactNode {
  const t = useT();
  const role = dealRoleLabel(view.commercial?.role ?? "", t);
  return (
    <>
      {role && <Badge>{role}</Badge>}
      <RecordAccess
        key={view.contact.id}
        kind="contact"
        record={view.contact}
      />
      <ContactRestrictedMark
        key={`restricted-${view.contact.id}`}
        contactId={view.contact.id}
      />
      <ContactCaseEntry
        key={`case-${view.contact.id}`}
        contactId={view.contact.id}
        followSurvivor
      />
    </>
  );
}

// Why logging is refused, under the id the Log verb points at. On a view-only
// contact the head's pill already says it, so the sentence is only described.
function activityRefusal(
  activity: ActivityGate,
  t: ReturnType<typeof useT>,
  hidden: boolean,
): ReactNode {
  if (activity.refusedId === undefined) {
    return undefined;
  }
  return (
    <span id={activity.sentenceId} className={hidden ? "sr-only" : undefined}>
      {t(activity.refusalKey ?? "record.logActivityRefused")}
    </span>
  );
}

// Where the Log refusal sits. Beside the View only pill it is hidden, since the
// pill says it. On the reader's own contact it sits under the left card.
export function placeLogRefusal(
  activity: ActivityGate,
  t: ReturnType<typeof useT>,
  readOnlyReason: string | undefined,
): Readonly<{ head?: ReactNode; footnote?: ReactNode }> {
  return readOnlyReason === undefined
    ? { footnote: activityRefusal(activity, t, false) }
    : { head: activityRefusal(activity, t, true) };
}

// The head's pills, then View only with the sentences its refused verbs
// point at.
export function ContactPulse({
  view,
  reasonId,
  activityRefused,
}: Readonly<{
  view: Contact360;
  reasonId: string;
  activityRefused: ReactNode;
}>) {
  return (
    <>
      <ContactMarks view={view} />
      <ViewOnly
        kind="contact"
        record={view.contact}
        name={view.contact.full_name}
        reasonId={reasonId}
      />
      {activityRefused}
    </>
  );
}
