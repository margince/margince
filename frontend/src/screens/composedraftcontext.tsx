import { Sparkles } from "lucide-react";
import type { components } from "../api/schema";
import { navigate } from "../app/router";
import { Button, TextInput } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { useCompany360 } from "./company360";
import type { DraftUnavailable } from "./compose";
import { Citations } from "./record360";

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
  // An overlay workspace has no native 360 to ground from; the endpoint
  // refuses there too, so the pickers simply have nothing to offer.
  const view = query.data?.state === "ready" ? query.data.view : undefined;
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
    return <p className="t-caption">{t("compose.noGroundableRecipient")}</p>;
  }
  return (
    <>
      <label className="t-body compose-check">
        {t("compose.draftTo")}
        <Select
          aria-label={t("compose.draftTo")}
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
      </label>
      {deals.length > 0 && (
        <label className="t-body compose-check">
          {t("compose.relatedTo")}
          <Select
            aria-label={t("compose.relatedTo")}
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
        </label>
      )}
    </>
  );
}

// A reason's chip opens the record it names. Only the two kinds that HAVE a
// screen are routed: a fact or a profile field has a receipt rather than a
// page, and this dialog is the wrong place to open one over.
export function openCited(entityType: string, entityId: string) {
  if (entityType === "deal") {
    navigate({ screen: "deals", id: entityId });
  }
  if (entityType === "contact") {
    navigate({ screen: "contacts", id: entityId });
  }
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
      <p className="t-caption">{t("compose.whyThisDraft")}</p>
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

// The drawer BEFORE a machine has written anything: what the rep wants said,
// and the one control that asks for it. It is the pre-draft face of the same
// block the disclosure band takes over once a draft exists — the two never
// show together, because the band's whole claim is about words that are on
// screen.
//
// Rejecting a draft is not here. It is a verdict on the finished words and it
// sits in the action row with the other verdicts (send, cancel), where a rep
// decides what happens to the message rather than how it gets written.
export function DraftOffer({
  intent,
  onIntentChange,
  draft,
  unavailable,
}: Readonly<{
  intent: string;
  onIntentChange: (next: string) => void;
  draft: PendingAction;
  unavailable: DraftUnavailable | null;
}>) {
  const t = useT();
  return (
    <div className="compose-offer">
      <div className="compose-draftbar">
        <TextInput
          // A NAME, not just a placeholder. The placeholder is the example and
          // disappears the moment the reader types; a field whose only name was
          // the example had none at all the instant it held anything.
          aria-label={t("compose.intentLabel")}
          placeholder={t("compose.intent")}
          value={intent}
          onChange={(event) => onIntentChange(event.target.value)}
        />
        {/* The agent's own verb, so it carries the agent's own colour and its
            mark. Drawn as an ordinary ghost button it read as the quietest
            control in the drawer when it is the one thing in here a machine
            does. Indigo means "Margince does this" everywhere else on the
            record; a composer that said it in grey is the one surface where
            the reader has to guess. */}
        <Button
          small
          variant="ai"
          onClick={draft.run}
          disabled={draft.disabled}
          pending={draft.pending}
          busyLabel={t("compose.drafting")}
        >
          <Sparkles aria-hidden="true" />
          {t("compose.draftWithAi")}
        </Button>
      </div>
      {unavailable && (
        <p className="t-caption">
          {unavailable === "no_model"
            ? t("compose.draftUnavailable")
            : t("compose.draftUnsupportedHere")}
        </p>
      )}
      {/* The failure appears without any navigation, so it is announced rather
          than merely coloured: a rep who cannot see the line has to be told
          the draft did not land, on the same terms the send refusals are. */}
      {!unavailable && draft.error && (
        <p
          className="t-caption"
          role="alert"
          style={{ color: "var(--dangerText)" }}
        >
          {draft.error}
        </p>
      )}
    </div>
  );
}
