// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The head of a message: how it travels, who it is to, and what it is about.
//
// Its own file because it is the part of the composer a rep touches on EVERY
// send, and because compose.tsx already carries the drafting, the consent gate,
// the refusal vocabulary and the send itself. What is here is only the form.

import { Mail, MessageSquare } from "lucide-react";
import type { components } from "../api/schema";
import { Button, Field, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Select } from "../design-system/select";
import { TokenInput, type TokenSuggestion } from "../design-system/tokeninput";
import { useT } from "../i18n";
import type { Transport } from "./contacttransports";
import "./composehead.css";

type Contact360 = components["schemas"]["Contact360"];
type Company360 = components["schemas"]["Company360"];

/** What a pressed Send is still waiting for, under the field it waits on. */
export function FieldNeed({
  show,
  need,
}: Readonly<{ show: boolean; need: string }>) {
  if (!show) {
    return null;
  }
  return (
    <p className="compose-need" role="alert">
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
    <Field label={t("compose.transport")}>
      {(control) => (
        <div className="compose-transport">
          {isChannel ? (
            <MessageSquare size={15} aria-hidden="true" />
          ) : (
            <Mail size={15} aria-hidden="true" />
          )}
          <Select
            {...control}
            options={transports.map((transport) => ({
              value: transport.id,
              label: transport.label,
            }))}
            value={selected?.id ?? ""}
            onChange={onChange}
          />
        </div>
      )}
    </Field>
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
      <Field label={t("compose.to")} error={invalidTo ? needTo : undefined}>
        {(control) => (
          <TokenInput
            {...control}
            values={to}
            onChange={(next) => onToChange(commit(next))}
            suggestions={suggestions}
            onEditing={onToEditing}
            disabled={disabled}
            placeholder={t("compose.recipientHint")}
          />
        )}
      </Field>
      <Field
        label={t("compose.cc")}
        trailing={
          bccOpen ? undefined : (
            // Quiet, and against the field rather than at the drawer's margin:
            // it belongs to the Cc line it extends, not to the head as a whole.
            <Button variant="ghost" onClick={onOpenBcc}>
              {t("compose.bcc")}
            </Button>
          )
        }
      >
        {(control) => (
          <TokenInput
            {...control}
            values={cc}
            onChange={(next) => onCcChange(commit(next))}
            suggestions={suggestions}
            disabled={disabled}
          />
        )}
      </Field>
      {bccOpen && (
        // What "blind" MEANS, beside the field that does it. A rep who reads
        // Bcc as "a quieter Cc" has told somebody about a conversation the
        // named recipients believe is between them.
        <Field label={t("compose.bcc")} hint={t("compose.bccHint")}>
          {(control) => (
            <TokenInput
              {...control}
              values={bcc}
              onChange={(next) => onBccChange(commit(next))}
              suggestions={suggestions}
              disabled={disabled}
            />
          )}
        </Field>
      )}
      {/* Under the addresses, because it is about the ones standing there — and
          a warning rather than a refusal: the rep may know something the ledger
          does not, and a bounce is a fact about the past. */}
      {deadRecipients.length > 0 && (
        <Callout
          tone="warning"
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
}: Readonly<{
  subject: string;
  onChange: (next: string) => void;
  invalid: boolean;
  need: string;
  disabled?: boolean;
}>) {
  const t = useT();
  return (
    <Field label={t("compose.subject")} error={invalid ? need : undefined}>
      {(control) => (
        <TextInput
          {...control}
          placeholder={t("compose.subjectHint")}
          value={subject}
          disabled={disabled}
          onChange={(event) => onChange(event.target.value)}
        />
      )}
    </Field>
  );
}
