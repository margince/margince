import { CheckSquare, FileText, Search, Send, Users } from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { useActivityGate } from "../../components/activitygate";
import { Button, type ButtonVariant } from "../../design-system/atoms";
import { CallAction } from "../../design-system/callaction";
import { primaryPhone } from "../../format/contacturi";
import { useT } from "../../i18n";
import { MOMENT_VERB_WRITES, momentVerbWrites } from "../../lib/writeverbs";
import { momentDrafts } from "./contactpage.drawers";

type Contact360 = components["schemas"]["Contact360"];
type ContactMomentAction = components["schemas"]["ContactMomentAction"];

// The bar has one filled verb, its lead; indigo when the model writes it.
function verbVariant(drafts: boolean, lead: boolean): ButtonVariant {
  if (lead) return drafts ? "ai" : "primary";
  return drafts ? "aiQuiet" : "ghost";
}

type ActionVerbProps = Readonly<{
  action: ContactMomentAction;
  contact: Contact360["contact"];
  primary?: boolean;
  onAction: (action: ContactMomentAction) => void;
}>;

// A call is a `tel:` link the server named; every other verb is a button
// that opens a surface.
export function ActionVerb(props: ActionVerbProps) {
  const { action, contact, primary } = props;
  if (action.kind === "call") {
    return (
      <CallAction
        appearance="button"
        label={action.label}
        number={primaryPhone(contact.phones)?.phone}
        filled={primary}
        record={{ entityType: "contact", entityId: contact.id }}
      />
    );
  }
  // A kind this build does not know, with no surface to open, would draw a
  // button that does nothing.
  if (!action.destination && !Object.hasOwn(MOMENT_VERB_WRITES, action.kind)) {
    return null;
  }
  return <SurfaceVerb {...props} />;
}

function SurfaceVerb({ action, contact, primary, onAction }: ActionVerbProps) {
  const t = useT();
  const permissionRefusal =
    action.kind === "schedule_meeting"
      ? "scheduling.bookRefused"
      : "record.logActivityRefused";
  const gate = useActivityGate(contact, permissionRefusal);
  const attaches =
    action.destination?.surface === "booking" ||
    action.destination?.surface === "activity_log";
  const blocked = action.state === "blocked";
  // A rule ladder picks the verb, not a model. Only the bar's lead verb is
  // filled; a press that has the model write the mail carries the ✦.
  const drafts = momentDrafts(action);
  // A sent mail files under this contact as a log or booking does.
  const files = attaches || drafts;
  // Not drawn: the press writes an activity, by the server's classification
  // of its kind, and this seat may not.
  if (momentVerbWrites(action.kind) && gate.refusalKey === permissionRefusal) {
    return null;
  }
  const variant = verbVariant(drafts, primary === true);
  return (
    <span className="today-verb">
      <Button
        variant={variant}
        // The server wrote the label, so it may be one long word that has to wrap.
        className="btn-valuelabel"
        onClick={() => onAction(action)}
        disabled={files && gate.pending}
        reason={
          blocked
            ? blockedReason(action, t)
            : files && gate.refusalKey
              ? t(gate.refusalKey)
              : undefined
        }
      >
        {!drafts && actionIcon(action.kind)}
        {action.label}
      </Button>
    </span>
  );
}

// A moment says what to do, never what carries it, so the reply verb is drawn
// as a send. An envelope would promise mail to a contact reached only by chat.
function actionIcon(kind: string): ReactNode {
  switch (kind) {
    case "schedule_meeting":
    case "open_meeting_brief":
    case "ask_colleague":
      return <Users size={15} aria-hidden="true" />;
    case "draft_reply":
      return <Send size={15} aria-hidden="true" />;
    case "complete_task":
      return <CheckSquare size={15} aria-hidden="true" />;
    case "open_research":
      return <Search size={15} aria-hidden="true" />;
    default:
      return <FileText size={15} aria-hidden="true" />;
  }
}

function blockedReason(
  action: ContactMomentAction,
  t: ReturnType<typeof useT>,
): string {
  return action.blocked_reason ?? t("contact.rail.blocked");
}
