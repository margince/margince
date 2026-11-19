import { AppLink } from "../../design-system/applink";
import { ErrorLine } from "../../design-system/errorline";
import type { useToast } from "../../design-system/toast";
import type { Translator } from "../../i18n";
import { useT } from "../../i18n";
import type { Approval } from "../../lib/approvals.queries";
import type { recordRoute } from "../../lib/entity";
import { navigate } from "../../lib/router";
import { editableSeed } from "./approvaleditor";
import type { EditableField } from "./approvalkind";

// What the decision row says about the mail it releases and the outcome of an
// approve. Apart from the row because none of it draws the proposal.

// What an approve did, read from the server's answer. An agent's staging only
// lets the agent repeat its call, so nothing is applied and nothing can be
// undone yet.
export function announceApproval(
  toast: ReturnType<typeof useToast>,
  t: Translator,
  decided: Approval,
  undoTarget: ReturnType<typeof recordRoute>,
) {
  if (decided.awaiting_agent) {
    toast.show(t("decision.waitingForAgent"));
    return;
  }
  if (!undoTarget) {
    return;
  }
  // The verb goes through `action`: the region draws it on its own ground, and
  // withdraws the message once it has been pressed.
  toast.show(t("decision.applied"), {
    action: {
      kind: "undo",
      label: t("decision.undoOnRecord"),
      onAct: () => navigate(undoTarget),
    },
  });
}

export function editedAwayFromProposal(
  editing: boolean,
  fields: readonly EditableField[],
  change: Readonly<Record<string, unknown>>,
  draft: Readonly<Record<string, string>>,
): boolean {
  return (
    editing &&
    fields.some(
      (entry) =>
        draft[entry.field] !== editableSeed(entry, change[entry.field]),
    )
  );
}

export function MailboxBlocked({ blocked }: Readonly<{ blocked: boolean }>) {
  const t = useT();
  if (!blocked) return null;
  return (
    <ErrorLine
      actions={
        <AppLink href="#/settings/connections" className="link-button">
          {t("compose.mailboxNotSendCapableGoto")}
        </AppLink>
      }
    >
      {t("decision.mailboxBlocked")}
    </ErrorLine>
  );
}
