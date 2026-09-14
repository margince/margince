// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The head of a message: how it travels, who it is to, and what it is about.
//
// Its own file because it is the part of the composer a rep touches on EVERY
// send, and because compose.tsx already carries the drafting, the consent gate,
// the refusal vocabulary and the send itself. What is here is only the form.

import { Mail, MessageSquare } from "lucide-react";
import type { components } from "../api/schema";
import { Button, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Select } from "../design-system/select";
import { TokenInput, type TokenSuggestion } from "../design-system/tokeninput";
import { useT } from "../i18n";
import type { Transport } from "./contacttransports";
import "./composehead.css";

type Contact360 = components["schemas"]["Contact360"];
type Company360 = components["schemas"]["Company360"];

/**
 * One line of the mail's head: what it is, then what it says.
 *
 * Label BESIDE the value, in a fixed column, because these answers are read as a
 * BLOCK — who it is to, what it is about, what travels with it — and a stack of
 * label-above-field turned five answers into ten lines a reader travels rather
 * than five they scan. `trailing` is the slot at the right end of the row for a
 * control that acts on the row itself; it keeps the value column's right edge,
 * so the fields still line up in one margin with or without one.
 */
export function MailRow({
  label,
  htmlFor,
  trailing,
  children,
}: Readonly<{
  label: string;
  /** The control's id, where it has one. Absent leaves the label a caption and
   *  the control names itself — which is what a token field does, since the box
   *  a reader types into is not the only thing inside it. */
  htmlFor?: string;
  trailing?: React.ReactNode;
  children: React.ReactNode;
}>) {
  return (
    <div className="mailrow">
      {htmlFor ? (
        <label className="mailrow-label t-caption" htmlFor={htmlFor}>
          {label}
        </label>
      ) : (
        <span className="mailrow-label t-caption">{label}</span>
      )}
      <div className="mailrow-value">{children}</div>
      {trailing && <div className="mailrow-trailing">{trailing}</div>}
    </div>
  );
}

/** What a pressed Send is still waiting for, under the field it waits on. */
export function FieldNeed({
  show,
  need,
}: Readonly<{ show: boolean; need: string }>) {
  if (!show) {
    return null;
  }
  return (
    <p className="t-caption compose-need" role="alert">
      {need}
    </p>
  );
}

/**
 * The addresses this message can be built out of.
 *
 * Read off the two 360s the composer ALREADY holds — the contact it was opened
 * on, and the account behind it — so the offer costs no request of its own and
 * cannot disagree with what the page behind the drawer is showing. A reader
 * remembers a colleague's NAME and not their address, which is why the contact is
 * the label; the address is the value AND the hint beside it, because one contact
 * can have several and a row that showed only the name would be a choice between
 * two identical-looking options.
 *
 * The record's own contact leads, because a message written from a contact's page
 * is overwhelmingly to that contact; the account's roster follows in the order
 * the 360 already put it in. A contact with no address on file is not offered —
 * a row that commits an empty recipient is help that refuses at the send.
 */
export function recipientSuggestions(
  contact: Contact360 | undefined,
  company: Company360 | undefined,
): readonly TokenSuggestion[] {
  const seen = new Set<string>();
  const out: TokenSuggestion[] = [];
  const offer = (value: string | null | undefined, label: string) => {
    const address = value?.trim();
    if (!address || seen.has(address)) {
      return;
    }
    seen.add(address);
    out.push({ value: address, label, hint: address });
  };
  const subject = contact?.contact;
  for (const address of subject?.emails ?? []) {
    offer(address.email, subject?.full_name ?? address.email);
  }
  for (const contact of company?.contacts?.data ?? []) {
    offer(contact.primary_email, contact.full_name);
  }
  return out;
}

/**
 * How this message travels, when the record can be written to more than one way.
 *
 * Drawn only where there is a CHOICE. One transport is not a decision, and a
 * dropdown holding a single option asks a reader to confirm something they were
 * never offered an alternative to — the record page's own verb already names it.
 */
export function TransportRow({
  transports,
  selected,
  onChange,
}: Readonly<{
  transports: readonly Transport[];
  selected: Transport | undefined;
  onChange: (id: string) => void;
}>) {
  const t = useT();
  if (transports.length < 2) {
    return null;
  }
  const isChannel = selected != null && selected.id !== "email";
  return (
    <MailRow label={t("compose.transport")}>
      <div className="compose-transport">
        {isChannel ? (
          <MessageSquare size={15} aria-hidden="true" />
        ) : (
          <Mail size={15} aria-hidden="true" />
        )}
        <Select
          aria-label={t("compose.transport")}
          options={transports.map((transport) => ({
            value: transport.id,
            label: transport.label,
          }))}
          value={selected?.id ?? ""}
          onChange={onChange}
        />
      </div>
    </MailRow>
  );
}

/**
 * Who the message is to.
 *
 * To and Cc stand; **Bcc is a button until it is asked for**. A blind copy is
 * the rare half of addressing and an always-drawn third row made every ordinary
 * mail read as a form with an empty field in it — while the field itself, being
 * the one that reaches somebody the recipients cannot see, is worth an explicit
 * press. It stays open once opened, and opens on its own when a caller arrives
 * holding one, because a field with a value in it may never be hidden.
 */
export function AddressBlock({
  to,
  onToChange,
  cc,
  onCcChange,
  bcc,
  onBccChange,
  bccOpen,
  onOpenBcc,
  suggestions,
  onToEditing,
  invalidTo,
  needTo,
  deadRecipients,
  disabled,
}: Readonly<{
  to: string[];
  onToChange: (next: string[]) => void;
  cc: string[];
  onCcChange: (next: string[]) => void;
  bcc: string[];
  onBccChange: (next: string[]) => void;
  bccOpen: boolean;
  onOpenBcc: () => void;
  suggestions: readonly TokenSuggestion[];
  /** The reader has taken the To line over — see TokenInput's own prop. */
  onToEditing: () => void;
  invalidTo: boolean;
  needTo: string;
  /** The recipients on this draft that are known not to arrive. */
  deadRecipients: readonly string[];
  disabled?: boolean;
}>) {
  const t = useT();
  const commit = (next: readonly string[]) => [...next];
  return (
    <>
      <MailRow label={t("compose.to")}>
        <TokenInput
          values={to}
          onChange={(next) => onToChange(commit(next))}
          suggestions={suggestions}
          onEditing={onToEditing}
          disabled={disabled}
          aria-label={t("compose.to")}
          aria-invalid={invalidTo || undefined}
          placeholder={t("compose.recipientHint")}
        />
        <FieldNeed show={invalidTo} need={needTo} />
      </MailRow>
      <MailRow
        label={t("compose.cc")}
        trailing={
          bccOpen ? undefined : (
            // Quiet, and against the field rather than at the drawer's margin:
            // it belongs to the Cc line it extends, not to the head as a whole.
            <Button small variant="ghost" onClick={onOpenBcc}>
              {t("compose.bcc")}
            </Button>
          )
        }
      >
        <TokenInput
          values={cc}
          onChange={(next) => onCcChange(commit(next))}
          suggestions={suggestions}
          disabled={disabled}
          aria-label={t("compose.cc")}
        />
      </MailRow>
      {bccOpen && (
        <MailRow label={t("compose.bcc")}>
          <TokenInput
            values={bcc}
            onChange={(next) => onBccChange(commit(next))}
            suggestions={suggestions}
            disabled={disabled}
            aria-label={t("compose.bcc")}
          />
          {/* What "blind" MEANS, beside the field that does it. A rep who reads
              Bcc as "a quieter Cc" has told somebody about a conversation the
              named recipients believe is between them. */}
          <p className="t-caption">{t("compose.bccHint")}</p>
        </MailRow>
      )}
      {/* Under the addresses, because it is about the ones standing there — and
          a warning rather than a refusal: the rep may know something the ledger
          does not, and a bounce is a fact about the past. */}
      {deadRecipients.length > 0 && (
        <Callout
          tone="warn"
          kind="standing"
          // Standing on open, yet it also appears as a rep TYPES a recipient,
          // and a reader who cannot see the field would otherwise never learn
          // the address they just added is dead.
          live="status"
          title={t("compose.deadRecipientsTitle")}
        >
          {t("compose.deadRecipients", {
            addresses: deadRecipients.join(", "),
          })}
        </Callout>
      )}
    </>
  );
}

/** What the message is about. Mail only: a channel carries no subject. */
export function SubjectRow({
  subject,
  onChange,
  invalid,
  need,
  disabled,
  id,
}: Readonly<{
  subject: string;
  onChange: (next: string) => void;
  invalid: boolean;
  need: string;
  disabled?: boolean;
  id: string;
}>) {
  const t = useT();
  return (
    <MailRow label={t("compose.subject")} htmlFor={id}>
      <TextInput
        id={id}
        aria-label={t("compose.subject")}
        placeholder={t("compose.subjectHint")}
        value={subject}
        disabled={disabled}
        aria-invalid={invalid || undefined}
        onChange={(event) => onChange(event.target.value)}
      />
      <FieldNeed show={invalid} need={need} />
    </MailRow>
  );
}
