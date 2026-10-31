import { Sparkles } from "lucide-react";
import type { components } from "../../api/schema";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { Disclosure } from "../../design-system/disclosure";
import { ErrorLine } from "../../design-system/errorline";
import {
  type PickableProject,
  projectLabel,
} from "../../design-system/projectpicker";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { useCompany360 } from "../companies/company360";
import { Citations } from "../records/record360";
import type { DraftUnavailable } from "./composedraftcall";

// The account-started draft: choosing what a message is about before a model
// writes it, and showing what the model read.
//
// It answers one question from three sides: what does this draft stand on?
// The picker says which record it is about, the reasons panel says what was
// read, and the offer turns a refusal into a next step.

// What an account-started message is about, as one choice beside the subject:
// one of the account's projects, one of its open deals, or nothing specific.
//
// A project puts its key in the subject; a deal has no key, so choosing one
// takes the key out. The send links the choice and the draft reads it. Both
// lists come from the account's 360, the view the draft grounds itself in.
export function RelatedFiling({
  companyId,
  projects,
  projectId,
  onProjectChange,
  dealId,
  onDealChange,
}: Readonly<{
  companyId: string;
  projects: readonly PickableProject[];
  projectId: string;
  onProjectChange: (next: string) => void;
  dealId: string;
  onDealChange: (next: string) => void;
}>) {
  const t = useT();
  const deals = useCompany360(companyId).data?.deals?.data ?? [];
  const choose = (next: string) => {
    const [kind, id = ""] = next.split(":");
    onDealChange(kind === "deal" ? id : "");
    onProjectChange(kind === "project" ? id : "");
  };
  return (
    <Field label={t("compose.relatedTo")} labelHidden>
      {(control) => (
        <Select
          {...control}
          options={[
            { value: "", label: t("compose.relatedToNone") },
            ...projects.map((project) => ({
              value: `project:${project.project_id}`,
              label: projectLabel(project),
            })),
            ...deals.map((deal) => ({
              value: `deal:${deal.deal_id}`,
              label: t("compose.relatedDeal", { name: deal.name }),
            })),
          ]}
          value={relatedValue(dealId, projectId)}
          onChange={choose}
          disabled={projects.length + deals.length === 0}
        />
      )}
    </Field>
  );
}

// A deal names the message more narrowly than a project, so it is the face
// when a restored message carries both.
function relatedValue(dealId: string, projectId: string): string {
  if (dealId) {
    return `deal:${dealId}`;
  }
  return projectId ? `project:${projectId}` : "";
}

// What the draft was written from: a "Based on" line and one chip per input.
// It stays shut under a "Why this draft" hint, so the words come first. A chip
// with evidence opens its record; one without is flat.
export function DraftReasons({
  reasons,
  onOpenRecord,
  onOpenEmail,
}: Readonly<{
  reasons: readonly components["schemas"]["AccountDraftReason"][];
  onOpenRecord?: (entityType: string, entityId: string) => void;
  // Opens the message a reason rests on, in this modal's own drawer.
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  if (reasons.length === 0) {
    return null;
  }
  return (
    <Disclosure className="compose-reasons" summary={t("compose.whyThisDraft")}>
      <p className="t-caption">
        {t("compose.basedOn", {
          inputs: reasons.map((reason) => reason.label).join(" · "),
        })}
      </p>
      <ul className="chips">
        {reasons.map((reason) => (
          <li key={`${reason.kind}:${reason.label}`}>
            {reason.label}
            {/* The record behind the reason, through the one citation
                renderer. It used to be a link-button wrapping the LABEL and
                calling onOpenRecord with whatever kind the reason carried —
                so a reason grounded in a conversation offered a control that
                routed nowhere, and a reason grounded in nothing at all still
                looked pressable because the guard only asked whether a
                handler existed. The citation asks what the record IS. */}
            {reason.evidence_ref && (
              <Citations
                evidence={[reason.evidence_ref]}
                onOpenRecord={onOpenRecord}
                onOpenEmail={onOpenEmail}
              />
            )}
          </li>
        ))}
      </ul>
    </Disclosure>
  );
}

// One control's worth of mutation state, flattened so a presentational child
// renders a pending/failed action without speaking react-query. `disabled` is
// wider than `pending`: a control is also barred while a sibling action that
// would contradict it is in flight.
export type PendingAction = Readonly<{
  run: () => void;
  pending: boolean;
  disabled: boolean;
  error: string | null;
}>;

// What holds an account draft back that the rep can supply. That is a contact
// of the account in To, or a contact on the account at all.
export type DraftNeed = "recipient" | "contacts" | null;

// The ask for a draft: what the rep wants said, and the one control that asks
// for it. It stays once a draft exists, so a changed steer can ask again.
//
// Rejecting a draft sits beside the disclosure instead. It is a verdict on the
// finished words.
export function DraftOffer({
  replying = false,
  intent,
  onIntentChange,
  draft,
  unavailable,
  need = null,
}: Readonly<{
  replying?: boolean;
  intent: string;
  onIntentChange: (next: string) => void;
  draft: PendingAction;
  unavailable: DraftUnavailable | null;
  /** Who the draft still needs before it can be asked for. */
  need?: DraftNeed;
}>) {
  const t = useT();
  const ask = replying ? "compose.replyIntent" : "compose.newIntent";
  return (
    <div className="compose-offer">
      {/* One line, as a mail client draws its own: the ask names itself in its
          placeholder, and a screen reader hears the same name as the label. A
          new message says what the model reads, as the line's own hint. */}
      <Field
        label={t(ask)}
        labelHidden
        hint={
          // Read to a screen reader; on screen the placeholder says it, and a
          // second line made the band heavy.
          replying ? undefined : (
            <span className="sr-only">{t("compose.draftContextHint")}</span>
          )
        }
      >
        {(control) => (
          <div className="compose-draftbar">
            <Sparkles aria-hidden="true" />
            <TextInput
              {...control}
              placeholder={t(ask)}
              value={intent}
              onChange={(event) => onIntentChange(event.target.value)}
            />
            {/* The agent's own verb, in the agent's own colour and mark:
                indigo means "Margince does this" everywhere on the record. */}
            <Button
              variant="ai"
              onClick={draft.run}
              disabled={draft.disabled}
              pending={draft.pending}
              busyLabel={t("compose.drafting")}
            >
              {t(replying ? "compose.draftReply" : "compose.draftEmail")}
            </Button>
          </div>
        )}
      </Field>
      {unavailable && (
        <p>
          {unavailable === "no_model"
            ? t("compose.draftUnavailable")
            : t("compose.draftUnsupportedHere")}
        </p>
      )}
      {need && (
        <p className="t-caption">
          {t(
            need === "contacts"
              ? "compose.noGroundableRecipient"
              : "compose.draftNeedsRecipient",
          )}
        </p>
      )}
      {!unavailable && draft.error && <ErrorLine>{draft.error}</ErrorLine>}
    </div>
  );
}
