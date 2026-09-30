import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { KIND_LABEL } from "./approvalkindlabel";
import { WON_REASON_LABELS, WON_REASONS } from "./winreason";

// Re-exported so the surfaces that read the map by name keep their import, and
// so this file stays the one place a reader looks for approval-kind vocabulary.
export { KIND_LABEL };

// What a reader may CHANGE before accepting, per kind.
//
// The inline editor's default is every string field of the proposed_change,
// rendered as a text box. That default is right for a rename — the value IS
// prose — and wrong for a proposal built out of identifiers and enums. Editing
// `company_id` re-aims the proposal at another record, and the server
// refuses that (assertSameEntityRefs); editing `proposed_lifecycle` by typing
// produces an invalid stage, and the server refuses that too. Both refusals
// are correct and neither is a thing to show a reader who was only trying to
// answer the question in front of them.
//
// So a kind may declare which fields it offers and what each one accepts. A
// kind that declares nothing keeps the default, which is why adding this
// changed no existing surface.
export type EditableField =
  | { readonly field: string; readonly as: "text"; readonly label?: MessageKey }
  | {
      readonly field: string;
      /**
       * A date-only wire value. It gets the calendar control rather than a text
       * box: the payload wants `2026-09-27`, and a reader typing the date the
       * way they say it out loud writes something the server refuses.
       */
      readonly as: "date";
      readonly label?: MessageKey;
    }
  | {
      readonly field: string;
      /**
       * Prose that runs to paragraphs rather than a line. An email body in a
       * single-line input is technically editable and practically unreadable:
       * the reader can see about eight words of what they are being asked to
       * put their name on.
       */
      readonly as: "textarea";
      readonly label?: MessageKey;
    }
  | {
      readonly field: string;
      readonly as: "choice";
      /**
       * What the field is CALLED. The wire name is a payload path, not a
       * caption — without this the editor asks a reader to set
       * "proposed_lifecycle".
       */
      readonly label?: MessageKey;
      readonly options: readonly string[];
      /**
       * What each option is CALLED. Without it the editor offers the wire
       * enum, so a German inbox asks a reader to choose "former_customer".
       * Optional so a choice field whose values are already words needs
       * nothing.
       */
      readonly optionLabels?: Readonly<Record<string, MessageKey>>;
    };

const COMPANY_LIFECYCLE_STAGES = [
  "unknown",
  "target",
  "prospect",
  "opportunity",
  "customer",
  "former_customer",
  "disqualified",
] as const;

// The same catalog keys the account page's stage badge reads, so the inbox and
// the record cannot call one stage two things. Keyed off the list above:
// a stage added there with no entry here fails the type.
const COMPANY_LIFECYCLE_LABELS: Readonly<
  Record<(typeof COMPANY_LIFECYCLE_STAGES)[number], MessageKey>
> = {
  unknown: "company.lifecycle.unknown",
  target: "company.lifecycle.target",
  prospect: "company.lifecycle.prospect",
  opportunity: "company.lifecycle.opportunity",
  customer: "company.lifecycle.customer",
  former_customer: "company.lifecycle.former_customer",
  disqualified: "company.lifecycle.disqualified",
};

export const EDITABLE_FIELDS: Readonly<
  Record<string, readonly EditableField[]>
> = {
  // The stage is the whole question. Everything else in the payload — which
  // account, which signal, the stage it is in now — is what the question is
  // ABOUT, and a reader who disagrees with any of that says no rather than
  // editing it into a different question.
  lifecycle_change: [
    {
      field: "proposed_lifecycle",
      as: "choice",
      label: "company.lifecycle",
      options: COMPANY_LIFECYCLE_STAGES,
      optionLabels: COMPANY_LIFECYCLE_LABELS,
    },
  ],
  // An automation-composed email waiting for a human to read, correct and
  // release. The words are the whole question, so both of them are offered.
  //
  // Declaring the fields also NARROWS what the editor shows, and here that is
  // the point rather than a side effect: the payload also carries the
  // addressee, the consent purpose and the anchor, and every one of those is
  // something the approver is agreeing TO rather than something to retype. The
  // server refuses an edited anchor outright (it is an entity reference, and
  // edit scope pins those), so offering it would only invite a refusal.
  held_draft: [
    { field: "subject", as: "text", label: "decision.draftSubject" },
    { field: "body", as: "textarea", label: "decision.draftBody" },
  ],
  // The date is the entire question, and it is the only thing here a contact may
  // change. Undeclared, the generic editor offered every string in the payload:
  // the deal's uuid as a text box to retype, the server's own reason sentence
  // as if it were the reader's to rewrite, and the previous date beside the
  // proposed one with nothing saying which was which.
  close_date_correction: [
    {
      field: "expected_close_date",
      as: "date",
      label: "approval.field.expected_close_date",
    },
  ],
  // Every flattened field here is a display copy of the card the server
  // parsed (vcardCreateProposal's Entry), and the create reads only that
  // nested struct — never the flattened strings a generic editor would offer
  // back. An edit to any of them would silently do nothing, so none is
  // offered rather than each looking like a live field that quietly is not.
  vcard_create: [],
  // The ONE thing on a stage card a rep may change, and only when the move
  // lands on a won stage with no signed agreement: why this deal was won
  // without paper. The proposer cannot answer it — no reading of a mailbox
  // establishes why there is no contract — so the card carries the question,
  // and the server refuses the move until it is answered.
  //
  // Everything else in the payload is what the question is ABOUT: which deal,
  // which stages, which criteria were met and what they rest on. A reader who
  // disagrees with any of that says no rather than editing it into a different
  // move.
  //
  // The vocabulary and its labels come from deals.tsx rather than a second copy
  // here: both are derived from the generated contract type, so a reason added
  // to crm.yaml stops this file compiling until it has a label.
  stage_progression: [
    {
      field: "won_without_contract_reason",
      as: "choice",
      label: "deals.winReason",
      options: WON_REASONS,
      optionLabels: WON_REASON_LABELS,
    },
    {
      field: "won_without_contract_detail",
      as: "text",
      label: "deals.winReasonDetail",
    },
  ],
};

/** humanize turns an unmapped wire enum into readable words. */
export function humanizeKind(kind: string): string {
  return kind.replaceAll("_", " ");
}

export function approvalKindLabel(kind: string, t: Translator): string {
  const key = KIND_LABEL[kind];
  return key ? t(key) : humanizeKind(kind);
}
