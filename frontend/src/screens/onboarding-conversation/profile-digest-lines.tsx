// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type KeyboardEvent, useState } from "react";
import { Button, Textarea, TextInput } from "../../design-system/atoms";
import { ordinalNumber } from "../../format/format";
import { useT } from "../../i18n";
import type { CompanyFieldName } from "../onboarding";
import type { ReviewRow } from "./company-review-state";
import type { Fact, LegalEntity, Person } from "./profile-digest-data";

// The line types the article and the companion both draw from, kept apart
// from their layout: a record line (with or without a value), a fact the
// crawl found under no field of the record, a person the crawl found
// published, and a legal entity the site's own notice named.

/**
 * One line of the article: what the record says, and the page that says it.
 *
 * A BLANK line with `onSettle` is an action, not a fact: the two-column
 * document renders it as its own dashed row with a way to the field's card in
 * the deck, because reading the whole record is exactly where a reader
 * notices what is still missing. The companion beside the deck never passes
 * `onSettle` — the deck asking about that same field is already on screen, so
 * a "Settle it" pointed at the card behind it would be a button aimed at
 * itself — and keeps the plainer active/blank treatment it always had.
 */
export function DigestLine({
  row,
  n,
  active,
  onSettle,
  onField,
}: Readonly<{
  row: ReviewRow;
  n?: number;
  active?: boolean;
  onSettle?: (field: CompanyFieldName) => void;
  /** Lets the value be corrected where it stands. The document passes it;
   * the companion beside the deck never does, since the deck is already
   * the control for the field it is asking about. */
  onField?: (field: CompanyFieldName, value: string) => void;
}>) {
  const t = useT();
  const empty = row.value.trim() === "";
  if (empty && onSettle !== undefined) {
    return <UnansweredLine row={row} onSettle={onSettle} />;
  }
  return (
    <p className="pdigest-line" data-active={active} data-empty={empty}>
      <span className="pdigest-label">{row.label}</span>{" "}
      {empty ? (
        <span className="pdigest-blank">
          {active ? t("ob.digest.deciding") : t("ob.digest.blank")}
        </span>
      ) : (
        <>
          {onField === undefined ? (
            <span className="pdigest-value">{row.value}</span>
          ) : (
            <EditableValue row={row} onField={onField} />
          )}
          {n === undefined ? (
            // Typed by a person, or carried in from a profile that already
            // existed. It gets no number because there is no page to open, and
            // saying so is the honest half of citing everything else.
            <span className="pdigest-yours">{t("ob.digest.yours")}</span>
          ) : (
            <sup className="pdigest-ref">{ordinalNumber(n)}</sup>
          )}
        </>
      )}
    </p>
  );
}

/**
 * A value a reader can correct in place: the text itself is the control, and
 * pressing it opens the one field under the caret. No pencil, no mode — the
 * article stays an article until a line is touched, and the change lands in
 * the same draft the deck writes to, so the one Save under the document
 * carries every correction at once.
 *
 * Committed on blur and on Enter (Escape puts the old value back); a
 * multiline field takes a box, as the deck's own card for it does, because a
 * paragraph edited in a one-line field is edited blind.
 */
function EditableValue({
  row,
  onField,
}: Readonly<{
  row: ReviewRow;
  onField: (field: CompanyFieldName, value: string) => void;
}>) {
  const t = useT();
  const [editing, setEditing] = useState(false);
  const [text, setText] = useState(row.value);
  const commit = () => {
    setEditing(false);
    if (text !== row.value) {
      onField(row.field, text);
    }
  };
  const onKeyDown = (event: KeyboardEvent<HTMLElement>) => {
    if (event.key === "Escape") {
      setText(row.value);
      setEditing(false);
    }
    if (event.key === "Enter" && !(row.multiline && !event.metaKey)) {
      event.preventDefault();
      commit();
    }
  };
  if (!editing) {
    return (
      <button
        type="button"
        className="pdigest-value pdigest-editable"
        aria-label={t("ob.digest.editLine", { label: row.label })}
        onClick={() => {
          setText(row.value);
          setEditing(true);
        }}
      >
        {row.value}
      </button>
    );
  }
  // Focus follows the press that opened the field: the reader asked for the
  // caret to be here, so this is the one autofocus that is not a hijack.
  const control = {
    autoFocus: true,
    className: "pdigest-edit-control",
    "aria-label": row.label,
    value: text,
    onBlur: commit,
    onKeyDown,
  };
  return row.multiline ? (
    <Textarea
      {...control}
      rows={3}
      onChange={(event) => setText(event.target.value)}
    />
  ) : (
    <TextInput {...control} onChange={(event) => setText(event.target.value)} />
  );
}

// The unanswered row: a dashed box in the agent's colour (staged, not yet
// accepted — see the provenance rule in design-system/README.md) holding the
// field's name, that it is not written down, and the one press that settles
// it. `1.5px dashed` rather than a solid border for the same reason a staged
// card everywhere else in the product carries one: nothing here is real until
// a person writes it, and the deck is where that happens.
function UnansweredLine({
  row,
  onSettle,
}: Readonly<{
  row: ReviewRow;
  onSettle: (field: CompanyFieldName) => void;
}>) {
  const t = useT();
  return (
    <p className="pdigest-open">
      <span className="pdigest-label">{row.label}</span>
      <span className="pdigest-blank">{t("ob.digest.notWritten")}</span>
      <Button variant="ghost" small onClick={() => onSettle(row.field)}>
        {t("ob.digest.settle")}
      </Button>
    </p>
  );
}

// One fact the crawl found, filed under no field of the record. Cited the
// same way as a record line: a page backing both shares the one number.
export function FactLine({
  fact,
  label,
  n,
}: Readonly<{ fact: Fact; label: string; n?: number }>) {
  return (
    <p className="pdigest-line">
      <span className="pdigest-label">{label}</span>{" "}
      <span className="pdigest-value">{fact.value}</span>
      {n === undefined ? null : (
        <sup className="pdigest-ref">{ordinalNumber(n)}</sup>
      )}
    </p>
  );
}

// One person the crawl found published on the site. Never a contact or a
// company-context row on its own — confirming the profile leaves people as a
// separate lead proposal — so the article states only what the page printed.
export function PersonLine({
  person,
  n,
}: Readonly<{ person: Person; n?: number }>) {
  return (
    <p className="pdigest-line">
      <span className="pdigest-value">{person.name}</span>{" "}
      <span className="pdigest-label">{person.role}</span>
      {person.published_email ? (
        <span className="pdigest-label"> · {person.published_email}</span>
      ) : null}
      {person.linkedin_url ? (
        <span className="pdigest-label"> · {person.linkedin_url}</span>
      ) : null}
      {n === undefined ? null : (
        <sup className="pdigest-ref">{ordinalNumber(n)}</sup>
      )}
    </p>
  );
}

// One legal entity the site's own legal notice named, folded into the
// article's Identity section rather than kept as a section of its own: a
// group publishes several and the read does not guess which one this
// installation is, so every detail the page happened to print rides along
// rather than being narrowed to one.
export function LegalEntityLine({
  entity,
  n,
}: Readonly<{ entity: LegalEntity; n?: number }>) {
  const details = [
    entity.registered_address,
    entity.register_number,
    entity.vat_number,
  ].filter((detail): detail is string => Boolean(detail));
  return (
    <p className="pdigest-line">
      <span className="pdigest-value">{entity.name}</span>
      {details.length === 0 ? null : (
        <span className="pdigest-label"> · {details.join(" · ")}</span>
      )}
      {n === undefined ? null : (
        <sup className="pdigest-ref">{ordinalNumber(n)}</sup>
      )}
    </p>
  );
}
