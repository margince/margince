import type { MessageKey } from "../../i18n/en";
import type { DisplayField } from "../../lib/approvaldisplay.types";

// The display rows of the two kinds that place a contact at a company: a
// LinkedIn match and an employer change. approvaldisplay.ts spreads them into
// its catalogue.

// A field whose value is already a word, under its caption.
export function textRow(field: string, label: MessageKey): DisplayField {
  return { field, label, as: "text" };
}

export const CONTACT_EMPLOYER_DISPLAY: Readonly<
  Record<string, readonly DisplayField[]>
> = {
  // Two records that look like one contact. The names are the comparison; the
  // position and the title beside them are what "Use as job title" weighs.
  linkedin_match: [
    textRow("connection_name", "approval.field.connection_name"),
    textRow("connection_company", "approval.field.connection_company"),
    textRow("connection_position", "approval.field.connection_position"),
    textRow("contact_name", "approval.field.contact_name"),
    textRow("contact_title", "approval.field.contact_title"),
  ],
  // Who moved, from where to where, and the mail that says so.
  employer_change: [
    textRow("contact_name", "approval.field.contact_name"),
    textRow("from_company_name", "approval.field.from_company"),
    textRow("to_company_name", "approval.field.to_company"),
    textRow("from_address", "approval.field.from_address"),
    {
      field: "sender_check",
      label: "approval.field.sender_check",
      as: "enum",
      optionLabels: { authenticated: "approval.senderCheck.authenticated" },
    },
    textRow("domain", "approval.field.domain"),
    textRow("signature", "approval.field.signature"),
    { field: "observed_on", label: "approval.field.observed_on", as: "date" },
  ],
};
