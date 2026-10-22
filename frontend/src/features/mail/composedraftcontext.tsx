import { Sparkles } from "lucide-react";
import type { components } from "../../api/schema";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import { useCompany360 } from "../companies/company360";
import { Citations } from "../records/record360";
import type { DraftUnavailable } from "./composedraftcall";

// The account-started draft: choosing what a message is about before a model
// writes it, and showing what the model read. Extracted from compose.tsx
// unchanged.
//
// It hangs together because it answers one question — what does this draft
// stand on — from three sides: the pickers that say which record it is about,
// the reasons panel that says what was read, and the offer that turns a refusal
// into a next step.
//
// MOVED, NOT REWRITTEN, for composeschedule.tsx's reason.

// The account-started path's three choices: who this is to, which open deal
// it is about, and which project it belongs to.
//
// All are read off the account's own 360 rather than a fresh search, for the
// reason the whole draft is: the endpoint grounds itself in the caller's view
// of the account, so a contact this picker offers that the view does not carry
// would be one the draft then refuses.
export function AccountDraftContext({
  companyId,
  recipientId,
  onRecipientChange,
  dealId,
  onDealChange,
}: Readonly<{
  companyId: string;
  recipientId: string;
  onRecipientChange: (next: string) => void;
  dealId: string;
  onDealChange: (next: string) => void;
}>) {
  const t = useT();
  const query = useCompany360(companyId);
  const view = query.data;
  const contacts = view?.contacts?.data ?? [];
  const deals = view?.deals?.data ?? [];

  // No contact on the account is an honest dead end for the DRAFT — the model
  // has no relationship to write from — and saying so beats an empty picker the
  // rep tries and cannot use. They can still type an address into To and write
  // the mail themselves.
  //
  // The PROJECT picker survives that dead end, and must: which body of work a
  // message is about has nothing to do with whether the account has a contact
  // yet. Returning early here took the project choice away from exactly the
  // message that most needs it — a check-in to an account nobody has spoken to
  // in a while, which is the first mail on a fresh delivery and the one with no
  // thread to inherit a project from. It would land unfiled, and the ladder
  // would ask about it in Approvals afterwards instead.
  if (query.isSuccess && contacts.length === 0) {
    return <p>{t("compose.noGroundableRecipient")}</p>;
  }
  return (
    <>
      <Field label={t("compose.draftTo")}>
        {(control) => (
          <Select
            {...control}
            options={[
              { value: "", label: t("compose.draftToUnset") },
              ...contacts.map((contact) => ({
                value: contact.contact_id,
                label: contact.full_name,
              })),
            ]}
            value={recipientId}
            onChange={onRecipientChange}
          />
        )}
      </Field>
      {deals.length > 0 && (
        <Field label={t("compose.relatedTo")}>
          {(control) => (
            <Select
              {...control}
              options={[
                { value: "", label: t("compose.relatedToNone") },
                ...deals.map((deal) => ({
                  value: deal.deal_id,
                  label: deal.name,
                })),
              ]}
              value={dealId}
              onChange={onDealChange}
            />
          )}
        </Field>
      )}
    </>
  );
}

// What the draft was written from, in the two shapes State D draws: a "Based
// on" line naming the inputs in order, and a row of "Why this draft?" chips.
//
// Both render the SAME reasons — they are one answer read two ways, which is
// why the server sends parts rather than a sentence. The line is for scanning
// before reading the draft; the chips are for checking one input after.
//
// A reason carrying evidence is pressable and opens the record it names. One
// without — the rep's own instruction — is flat, because there is nothing to
// open and a chip that looks pressable and is not is worse than a plain one.
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
    <div className="compose-reasons">
      <p className="t-caption">
        {t("compose.basedOn", {
          inputs: reasons.map((reason) => reason.label).join(" · "),
        })}
      </p>
      <p>{t("compose.whyThisDraft")}</p>
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
    </div>
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
}: Readonly<{
  replying?: boolean;
  intent: string;
  onIntentChange: (next: string) => void;
  draft: PendingAction;
  unavailable: DraftUnavailable | null;
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
      {!unavailable && draft.error && <ErrorLine>{draft.error}</ErrorLine>}
    </div>
  );
}
