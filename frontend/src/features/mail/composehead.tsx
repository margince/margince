// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The head of a message: how it travels, who it is to, and what it is about.
//
// Its own file because it is the part of the composer a rep touches on EVERY
// send, and because compose.tsx already carries the drafting, the consent gate,
// the refusal vocabulary and the send itself. What is here is only the form.

import { Mail, MessageSquare } from "lucide-react";
import { type ReactNode, useState } from "react";
import type { components } from "../../api/schema";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ErrorLine } from "../../design-system/errorline";
import { Select } from "../../design-system/select";
import {
  type TokenCard,
  TokenInput,
  type TokenParse,
  type TokenSuggestion,
} from "../../design-system/tokeninput";
import { INTL_LOCALE } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { addressKey } from "../../lib/addresskey";
import type { Transport } from "../../lib/contacttransports";
import { ENTITY } from "../../lib/entity";
import { routeHash } from "../../lib/router";
import { readAddressList } from "./composeaddresslist";
import type { RecipientCountries } from "./composecountries";
import { RecipientCountryControl } from "./composecountry";
import "./composehead.css";

type Contact = components["schemas"]["Contact"];
type Contact360 = components["schemas"]["Contact360"];
type Company360 = components["schemas"]["Company360"];
type User = components["schemas"]["User"];
type Team = components["schemas"]["Team"];

export type AddressLine = "to" | "cc" | "bcc";

const ADDRESS_LINES: readonly AddressLine[] = ["to", "cc", "bcc"];

const ADDRESS_KEYS = [",", ";"];

// A colleague is found by name, so colleagues wait for typed letters.
const IDLE_CONTACTS = 8;
const COLLEAGUES_AFTER = 2;

const NO_REPEATS: Readonly<Record<AddressLine, readonly string[]>> = {
  to: [],
  cc: [],
  bcc: [],
};

const MAILABLE_STATUS: ReadonlySet<User["status"]> = new Set([
  "active",
  "invited",
]);

const TO_FIELD = "compose-to-field";

/** The To line's text box inside a composer. */
export function toBox(
  composer: Element | null | undefined,
): HTMLElement | null {
  return composer?.querySelector<HTMLElement>(`.${TO_FIELD} input`) ?? null;
}

/** What a pressed Send is still waiting for, under the field it waits on. */
export function FieldNeed({
  show,
  need,
}: Readonly<{ show: boolean; need: string }>) {
  if (!show) {
    return null;
  }
  return <ErrorLine>{need}</ErrorLine>;
}

export type RecipientOffers = Readonly<{
  contacts: readonly TokenSuggestion[];
  colleagues: readonly TokenSuggestion[];
}>;

/**
 * The addresses this message can be built out of, from reads the composer already holds.
 * The label is the name a reader remembers; the address is the value and the hint.
 * Contacts: the contact, the contacts around it, then the account's roster.
 * A contact with no address is left out, because it would refuse at the send.
 */
export function recipientSuggestions({
  contact,
  related = [],
  company,
  colleagues = [],
  viewerId,
}: Readonly<{
  contact?: Contact360;
  related?: readonly Contact[];
  company?: Company360;
  colleagues?: readonly (User | Team)[];
  /** The sender, who is never a recipient of their own mail. */
  viewerId?: string;
}>): RecipientOffers {
  // The sender's own address stays out even where a contact holds it.
  const seen = new Set(
    colleagues.flatMap((member) =>
      member.id === viewerId && "email" in member
        ? [addressKey(member.email)]
        : [],
    ),
  );
  const contacts: TokenSuggestion[] = [];
  const members: TokenSuggestion[] = [];
  const offerInto =
    (out: TokenSuggestion[]) =>
    (value: string | null | undefined, label: string, card?: TokenCard) => {
      const address = value?.trim();
      if (!address || seen.has(addressKey(address))) {
        return;
      }
      seen.add(addressKey(address));
      out.push({ value: address, label, hint: address, card });
    };
  const offer = offerInto(contacts);
  const account = company?.company?.display_name;
  const contactCard = (
    id: string,
    title: string | null | undefined,
    employer: string | undefined,
  ) => ({
    identity: id,
    href: routeHash(ENTITY.contact.route(id)),
    about: [title, employer].filter(Boolean).join(" · ") || undefined,
  });
  const subject = contact?.contact;
  for (const address of subject?.emails ?? []) {
    offer(
      address.email,
      subject?.full_name ?? address.email,
      subject && contactCard(subject.id, subject.title, account),
    );
  }
  for (const around of related) {
    offer(
      around.primary_email,
      around.full_name,
      contactCard(around.id, around.title, around.employer?.company_name),
    );
  }
  for (const rostered of company?.contacts?.data ?? []) {
    offer(
      rostered.primary_email,
      rostered.full_name,
      contactCard(rostered.contact_id, rostered.title, account),
    );
  }
  const offerMember = offerInto(members);
  for (const member of colleagues) {
    // An agent seat has no mailbox anyone writes to, and a suspended or
    // deactivated member no longer reads one.
    if (
      !("email" in member) ||
      member.is_agent ||
      !MAILABLE_STATUS.has(member.status)
    ) {
      continue;
    }
    offerMember(member.email, member.display_name, {
      identity: member.id,
      photo: member.avatar_url,
    });
  }
  return { contacts, colleagues: members };
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
 * To stands; **Cc and Bcc are one link until they are asked for**. Copies are
 * the rare half of addressing. Two always-drawn rows made every ordinary mail
 * read as a form with empty fields in it. Once opened they stay open. They
 * open on their own when either holds an address, because a field with a value
 * in it may never be hidden.
 *
 * An address stands once on a message. One already in any field, in any case,
 * is refused and stays typed, with the field that holds it named.
 */
export function AddressBlock({
  to,
  onToChange,
  cc,
  onCcChange,
  bcc,
  onBccChange,
  copiesOpen,
  onOpenCopies,
  suggestions,
  onToEditing,
  refused,
  onRefuse,
  invalidTo,
  needTo,
  deadRecipients,
  disabled,
  marked,
  countries,
}: Readonly<{
  to: string[];
  onToChange: (next: string[]) => void;
  cc: string[];
  onCcChange: (next: string[]) => void;
  bcc: string[];
  onBccChange: (next: string[]) => void;
  copiesOpen: boolean;
  onOpenCopies: () => void;
  suggestions: RecipientOffers;
  /** The reader has taken the To line over — see TokenInput's own prop. */
  onToEditing: () => void;
  /** Text each line could not read as addresses. */
  refused: Readonly<Record<AddressLine, readonly string[]>>;
  onRefuse: (line: AddressLine, parts: readonly string[]) => void;
  invalidTo: boolean;
  needTo: string;
  /** The recipients on this draft that are known not to arrive. */
  deadRecipients: readonly string[];
  disabled?: boolean;
  /** Recipients a refused Send named, marked on their chips. */
  marked?: Readonly<{ values: ReadonlySet<string>; label: string }>;
  /** The countries stated for this message, offered on each chip. */
  countries?: RecipientCountries;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  // Repeats stay typed but hold no send: the address is on the message already.
  const [repeats, setRepeats] = useState(NO_REPEATS);
  const lines: Readonly<Record<AddressLine, readonly string[]>> = {
    to,
    cc,
    bcc,
  };
  const label: Readonly<Record<AddressLine, string>> = {
    to: t("compose.to"),
    cc: t("compose.cc"),
    bcc: t("compose.bcc"),
  };
  const holder = (address: string) =>
    ADDRESS_LINES.find((line) =>
      lines[line].some((held) => addressKey(held) === addressKey(address)),
    );
  const isHeld = (address: string) => holder(address) !== undefined;
  const aside = countries
    ? (address: string) => (
        <RecipientCountryControl
          address={address}
          countries={countries}
          disabled={disabled}
        />
      )
    : undefined;
  const parse = (line: AddressLine): TokenParse => ({
    keys: ADDRESS_KEYS,
    key: addressKey,
    read: readAddressList,
    refuses: isHeld,
    onRefuse: (parts) => {
      setRepeats((prev) => ({ ...prev, [line]: parts.filter(isHeld) }));
      onRefuse(
        line,
        parts.filter((part) => !isHeld(part)),
      );
    },
  });
  const colleagues = suggestions.colleagues.map((row) => ({
    ...row,
    group: t("compose.colleagues"),
  }));
  const named = [...suggestions.contacts, ...colleagues];
  const offerFor = (typed: string) => {
    const free = (row: TokenSuggestion) => !isHeld(row.value);
    const contacts = suggestions.contacts.filter(free);
    const words = typed.trim();
    if (words === "") {
      return contacts.slice(0, IDLE_CONTACTS);
    }
    return words.length < COLLEAGUES_AFTER
      ? contacts
      : [...contacts, ...colleagues.filter(free)];
  };
  const removeLabel = (value: string) => t("tokenInput.remove", { value });
  const refusal = (line: AddressLine) => {
    const holders = [
      ...new Set(repeats[line].flatMap((part) => holder(part) ?? [])),
    ];
    const sentences = [
      refused[line].length === 0
        ? undefined
        : plural("compose.notAnAddress", refused[line].length, {
            parts: refused[line].join(", "),
          }),
      holders.length === 0
        ? undefined
        : t("compose.alreadyIn", {
            field: new Intl.ListFormat(INTL_LOCALE[locale], {
              type: "conjunction",
            }).format(holders.map((held) => label[held])),
          }),
    ].filter((sentence) => sentence !== undefined);
    return sentences.length === 0 ? undefined : sentences.join(" ");
  };
  const commit = (next: readonly string[]) => [...next];
  const copies = copiesOpen || cc.length > 0 || bcc.length > 0;
  const tokenProps = {
    suggestions: named,
    offerFor,
    removeLabel,
    disabled,
    marked,
    aside,
  };
  return (
    <>
      <Field
        className={`compose-address-field ${TO_FIELD}`}
        label={label.to}
        error={refusal("to") ?? (invalidTo ? needTo : undefined)}
        trailing={
          copies ? undefined : (
            // Against the field it extends, not at the dialog's margin.
            <Button variant="link" onClick={onOpenCopies}>
              {t("compose.copies")}
            </Button>
          )
        }
      >
        {(control) => (
          <TokenInput
            {...control}
            {...tokenProps}
            values={to}
            onChange={(next) => onToChange(commit(next))}
            parse={parse("to")}
            onEditing={onToEditing}
            placeholder={t("compose.recipientHint")}
          />
        )}
      </Field>
      {copies && (
        <>
          <Field
            className="compose-address-field"
            label={label.cc}
            error={refusal("cc")}
          >
            {(control) => (
              <TokenInput
                {...control}
                {...tokenProps}
                values={cc}
                onChange={(next) => onCcChange(commit(next))}
                parse={parse("cc")}
              />
            )}
          </Field>
          <Field
            className="compose-address-field"
            label={label.bcc}
            hint={t("compose.bccHint")}
            error={refusal("bcc")}
          >
            {(control) => (
              <TokenInput
                {...control}
                {...tokenProps}
                values={bcc}
                onChange={(next) => onBccChange(commit(next))}
                parse={parse("bcc")}
              />
            )}
          </Field>
        </>
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
  aside,
  under,
}: Readonly<{
  subject: string;
  onChange: (next: string) => void;
  invalid: boolean;
  need: string;
  disabled?: boolean;
  /** A control on the subject's own line (the project whose tag it carries). */
  aside?: ReactNode;
  /** A line under the subject, about what the subject carries. */
  under?: ReactNode;
}>) {
  const t = useT();
  return (
    <div className="compose-subject">
      <Field
        className="compose-subject-field"
        label={t("compose.subject")}
        labelHidden
        error={invalid ? need : undefined}
      >
        {(control) => (
          <TextInput
            {...control}
            placeholder={t("compose.subject")}
            value={subject}
            disabled={disabled}
            onChange={(event) => onChange(event.target.value)}
          />
        )}
      </Field>
      {aside}
      {under && <div className="compose-subject-under">{under}</div>}
    </div>
  );
}
