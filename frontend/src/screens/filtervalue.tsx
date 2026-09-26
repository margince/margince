// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A filter operand's control, shared by the Filters and Analytics builders.

import { useId } from "react";
import { SegmentedControl } from "../design-system/atoms";
import { DateInput } from "../design-system/dateinput";
import { Select } from "../design-system/select";
import { TokenInput } from "../design-system/tokeninput";
import { useT } from "../i18n";
import "./filterbuilder.css";
import { RosterPartialNote } from "./entityref";
import type { VocabularyField } from "./filterdata";
import {
  boundedReference,
  type Reference,
  useReferenceOptions,
} from "./filterreference";
import {
  ChosenRecords,
  chosenIDs,
  SearchedRecordValue,
} from "./filtervalue.records";
import type { FilterOp, LeafValue } from "./segmentpredicate";

/**
 * The value half of a clause, chosen by operator first and field type second.
 *
 * Operator first because `in` takes a list whatever the field's type is, and
 * `exists` takes no value a human types at all — it is answered by the operator
 * they picked. Only the scalar comparisons defer to the type.
 */
export function ValueControl({
  type,
  references,
  options,
  op,
  value,
  onChange,
  label,
}: Readonly<{
  type: VocabularyField["type"];
  /** What an id field's values point at, when the vocabulary named one. */
  references: Reference | undefined;
  /** A picklist's allowed values, when the vocabulary carried them. */
  options: readonly string[] | undefined;
  op: FilterOp;
  value: LeafValue;
  onChange: (next: LeafValue) => void;
  label?: string;
}>) {
  const t = useT();
  const name = label ?? t("filters.value");
  if (op === "exists") {
    // `exists` carries its own boolean, and the two readings ARE the question
    // rather than an operand: "has a value" or "is empty".
    return (
      <BooleanValue
        value={value}
        onChange={onChange}
        labels={{ true: t("filters.hasValue"), false: t("filters.isEmpty") }}
        label={t("filters.existsLabel")}
      />
    );
  }
  // A REFERENCE IS ANSWERED BEFORE THE OPERATOR IS, and the order is the whole
  // point. `in` used to win, so every id field's list was a free-text token box
  // — a uuid typed wrong there compiles, matches nothing, and reads as a
  // settled "no rows" exactly as the single case did. The list operator changes
  // how many records are named, not whether they are chosen.
  // `exists` has already returned above, so every operator reaching here takes
  // an operand and a reference answers all of them.
  if (references !== undefined) {
    return (
      <ReferenceValue
        label={label}
        reference={references}
        type={type}
        many={op === "in"}
        value={value}
        onChange={onChange}
      />
    );
  }
  if (op === "in") {
    return (
      <TokenInput
        values={Array.isArray(value) ? value.map(String) : []}
        onChange={(next) => onChange(next)}
        aria-label={label ?? t("filters.values")}
        placeholder={t("filters.addValue")}
      />
    );
  }
  if (options !== undefined && options.length > 0) {
    // A closed set is picked, not typed. Typing it invites the failure this whole
    // surface exists to prevent: a value outside the set compiles, matches
    // nothing, and reads as a settled answer rather than as a mistake.
    //
    // The label IS the value. These are the wire words a workspace admin chose,
    // or a contract enum; inventing display copy here would be a second
    // vocabulary for a reader to reconcile against what the record page shows.
    return (
      <Select
        options={options.map((option) => ({ value: option, label: option }))}
        value={typeof value === "string" ? value : ""}
        onChange={onChange}
        placeholder={t("filters.pickValue")}
        aria-label={name}
      />
    );
  }
  return (
    <ScalarValue type={type} value={value} onChange={onChange} label={name} />
  );
}

/**
 * An id comparison as the RECORDS it names, whether it names one or a list.
 *
 * Which control depends on the target and not on the operator: a set this
 * module can read whole is a list to pick from, and the one that cannot be —
 * companies, as many as the workspace has customers — is searched. `many`
 * only decides whether picking replaces the value or appends to it.
 */
function ReferenceValue({
  reference,
  type,
  many,
  value,
  onChange,
  label,
}: Readonly<{
  label?: string;
  reference: Reference;
  type: VocabularyField["type"];
  many: boolean;
  value: LeafValue;
  onChange: (next: LeafValue) => void;
}>) {
  if (boundedReference(reference)) {
    return (
      <RecordValue
        label={label}
        reference={reference}
        type={type}
        many={many}
        value={value}
        onChange={onChange}
      />
    );
  }
  return (
    <SearchedRecordValue
      many={many}
      value={value}
      onChange={onChange}
      label={label}
    />
  );
}

/**
 * An id comparison as the RECORD it names, not its uuid.
 *
 * Only for a target the options module can enumerate. A company reference
 * falls through to the plain box, because a workspace's accounts are as many as
 * its customers and a dropdown cannot hold them — the async picker that case
 * needs is its own change, and a half-filled list would be worse than a box.
 *
 * A stored id whose record is gone still shows: Select renders its placeholder
 * for a value matching no option, so the clause reads as needing attention
 * rather than silently appearing empty.
 */
function RecordValue({
  reference,
  type,
  many,
  value,
  onChange,
  label,
}: Readonly<{
  label?: string;
  reference: Reference | undefined;
  type: VocabularyField["type"];
  many: boolean;
  value: LeafValue;
  onChange: (next: LeafValue) => void;
}>) {
  const t = useT();
  const noteId = useId();
  const name = label ?? t("filters.value");
  const { options, loading, failed, partial } = useReferenceOptions(reference);
  const ids = chosenIDs(value);
  // A read that failed falls back to the plain box rather than to an empty
  // dropdown. An empty list would tell the reader this workspace has no such
  // records — a confident answer to a question that never got one — and would
  // leave them unable to write the clause at all.
  if (failed) {
    return (
      <ScalarValue type={type} value={value} onChange={onChange} label={name} />
    );
  }
  // A list names each record the same way a single comparison does, one pick at
  // a time, and shows what it already holds above the picker. The picker is
  // left EMPTY after each pick rather than showing the last one: it is the way
  // in for the next record, and a value sitting in it would read as a selection
  // that had not been added.
  const chosen = many ? "" : (ids[0] ?? "");
  const labels = new Map(options.map((option) => [option.value, option.label]));
  // The caveat stacks UNDER the picker inside the clause's own value column
  // rather than becoming another item in the clause row: as a row item it would
  // sit between this control and the button that removes the clause, and it is
  // where the row wraps on a narrow viewport.
  return (
    <div className="filter-value">
      {many && (
        <ChosenRecords
          ids={ids}
          labels={labels}
          onRemove={(id) => onChange(ids.filter((held) => held !== id))}
        />
      )}
      <Select
        options={many ? options.filter((o) => !ids.includes(o.value)) : options}
        value={chosen}
        onChange={(next) => onChange(many ? [...ids, String(next)] : next)}
        disabled={loading}
        placeholder={
          loading ? t("filters.loadingRecords") : t("filters.pickRecord")
        }
        aria-label={name}
        aria-describedby={partial ? noteId : undefined}
      />
      {/* A clause written against a list that stopped short of the workspace is
          a clause about the wrong set, and nothing else on this row would say
          so. */}
      <RosterPartialNote partial={partial} id={noteId} />
    </div>
  );
}

/** The two-way choice that `exists` and a boolean field both need. */
function BooleanValue({
  value,
  onChange,
  labels,
  label,
}: Readonly<{
  value: LeafValue;
  onChange: (next: LeafValue) => void;
  labels: Record<"true" | "false", string>;
  label: string;
}>) {
  return (
    <SegmentedControl
      options={["true", "false"] as const}
      value={value === false ? "false" : "true"}
      onChange={(next) => onChange(next === "true")}
      labels={labels}
      label={label}
    />
  );
}

/**
 * One scalar operand, drawn as its field's type.
 *
 * A date gets the date control because the engine's date operand and that
 * control's value are the same YYYY-MM-DD string, so a value round-trips with no
 * parse step for anyone to get wrong.
 */
function ScalarValue({
  type,
  value,
  onChange,
  label,
}: Readonly<{
  label: string;
  type: VocabularyField["type"];
  value: LeafValue;
  onChange: (next: LeafValue) => void;
}>) {
  const t = useT();
  if (type === "date") {
    const iso = typeof value === "string" ? value : "";
    return (
      <DateInput
        value={iso as "" | `${number}-${number}-${number}`}
        onChange={(event) => onChange(event.target.value)}
        aria-label={label}
      />
    );
  }
  if (type === "boolean") {
    return (
      <BooleanValue
        value={value}
        onChange={onChange}
        labels={{ true: t("filters.yes"), false: t("filters.no") }}
        label={label}
      />
    );
  }
  const numeric = type === "number" || type === "currency";
  return (
    <input
      className="input"
      value={
        typeof value === "string" || typeof value === "number"
          ? String(value)
          : ""
      }
      onChange={(event) =>
        onChange(
          numeric ? numberOrText(event.target.value) : event.target.value,
        )
      }
      aria-label={label}
      inputMode={numeric ? "decimal" : undefined}
    />
  );
}

/**
 * A numeric field's operand as a number when it is one, and as the raw text
 * otherwise.
 *
 * Half-typed input is the reason: "-" and "1." are neither valid numbers nor
 * mistakes, and coercing them to 0 or NaN would either move the count or refuse a
 * clause somebody is still typing. Leaving the text alone lets `isComplete` hold
 * the preview until the value is a number the engine will accept.
 */
function numberOrText(raw: string): LeafValue {
  if (raw === "") {
    return "";
  }
  const parsed = Number(raw);
  return Number.isFinite(parsed) ? parsed : raw;
}
