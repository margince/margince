// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A filter operand's control, shared by the Filters and Analytics builders.

import { useId, useState } from "react";
import { SegmentedControl, TextInput } from "../../design-system/atoms";
import { DateInput, isoOrBlank } from "../../design-system/dateinput";
import { ErrorLine } from "../../design-system/errorline";
import { Select } from "../../design-system/select";
import { formatNumber } from "../../format/format";
import { INDUSTRY_FIELD, industryWords } from "../../format/industry";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import "./filterbuilder.css";
import { youThenByName } from "../../components/assigneepicker";
import { RosterPartialNote } from "../../components/entityref";
import {
  boundedReference,
  type Reference,
  useReferenceOptions,
} from "../../components/filterreference";
import type { VocabularyField } from "../../lib/filterdata";
import { valueFor } from "../../lib/labelfor";
import {
  type FilterOp,
  isRelativeDate,
  type LeafValue,
  MAX_DAYS_AGO,
} from "../../lib/segmentpredicate";
import { useMe } from "../../lib/session";
import { MoneyControl } from "./filtervalue.money";
import { ListValue, NumberValue } from "./filtervalue.number";
import { RadiusValue } from "./filtervalue.radius";
import {
  ChosenRecords,
  chosenIDs,
  SearchedRecordValue,
} from "./filtervalue.records";

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
  optionLabels,
  op,
  value,
  onChange,
  label,
  currency,
  field,
}: Readonly<{
  type: VocabularyField["type"];
  /** What an id field's values point at, when the vocabulary named one. */
  references: Reference | undefined;
  /** A picklist's allowed values, when the vocabulary carried them. */
  options: readonly string[] | undefined;
  /** The reader's names for some of those values; a value without one reads as itself. */
  optionLabels?: Readonly<Record<string, string>>;
  /** The currency a money field counts minor units of, when it is known. */
  currency?: string;
  op: FilterOp;
  value: LeafValue;
  onChange: (next: LeafValue) => void;
  label?: string;
  /** The vocabulary field being compared, so a coded set can be worded. */
  field?: string;
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
  if (op === "within_radius") {
    return <RadiusValue value={value} onChange={onChange} label={name} />;
  }
  // Before `in`: a uuid typed into a token box matches nothing and reads as a
  // settled "no rows". `in` changes how many records are named, not how.
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
  if (type === "currency") {
    return currency ? (
      <MoneyControl
        op={op}
        value={value}
        onChange={onChange}
        label={label}
        currency={currency}
      />
    ) : (
      <UnpricedAmount label={name} />
    );
  }
  if (op === "in") {
    return (
      <ListValue
        type={type}
        value={value}
        onChange={onChange}
        label={label ?? t("filters.values")}
      />
    );
  }
  if (options !== undefined && options.length > 0) {
    // A closed set is picked, not typed. Typing it invites the failure this whole
    // surface exists to prevent: a value outside the set compiles, matches
    // nothing, and reads as a settled answer rather than as a mistake.
    //
    // The label is the value, except for the industry codes, which the record
    // page words through format/industry.ts and so does this picker.
    return (
      <Select
        options={options.map((option) => ({
          value: option,
          label:
            field === INDUSTRY_FIELD
              ? industryWords(t, option)
              : (optionLabels?.[option] ?? option),
        }))}
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

/** What an empty picker asks for, named by the kind of record it lists. */
const PICK_PROMPTS: Readonly<Record<Reference, MessageKey>> = {
  tag: "filters.pickTag",
  app_user: "filters.pickOwner",
  team: "filters.pickTeam",
  company: "filters.pickCompany",
  pipeline: "filters.pickPipeline",
  stage: "filters.pickStage",
  project: "filters.pickProject",
  product: "filters.pickProduct",
};

function pickPrompt(reference: Reference): MessageKey {
  return valueFor(PICK_PROMPTS, reference, "filters.pickValue");
}

/**
 * An id comparison as the RECORDS it names, whether it names one or a list.
 *
 * Which control depends on the target (`boundedReference`), not on the
 * operator. `many` only decides whether picking replaces the value or appends.
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
  const t = useT();
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
      placeholder={t(pickPrompt(reference))}
    />
  );
}

/**
 * An id comparison as the RECORD it names, not its uuid.
 *
 * A member list starts with the reader.
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
  reference: Reference;
  type: VocabularyField["type"];
  many: boolean;
  value: LeafValue;
  onChange: (next: LeafValue) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const noteId = useId();
  const viewerId = useMe().data?.user.id;
  const name = label ?? t("filters.value");
  const read = useReferenceOptions(reference);
  const { loading, failed, partial } = read;
  const options =
    reference === "app_user"
      ? youThenByName(read.options, viewerId, t("filters.you"), locale)
      : read.options;
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
          loading ? t("filters.loadingRecords") : t(pickPrompt(reference))
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

/**
 * A money operand in a currency this reader cannot learn. Its minor units
 * would read as the amount, "125000" for €1,250, so none is shown or typed.
 */
function UnpricedAmount({ label }: Readonly<{ label: string }>) {
  const t = useT();
  const noteId = useId();
  return (
    <div className="filter-value">
      <TextInput disabled aria-label={label} aria-describedby={noteId} />
      <p className="t-caption" id={noteId}>
        {t("filters.amountUnpriced")}
      </p>
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
    return <DateValue value={value} onChange={onChange} label={label} />;
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
  if (type === "number" || type === "currency") {
    return <NumberValue value={value} onChange={onChange} label={label} />;
  }
  return (
    <input
      className="input"
      value={
        typeof value === "string" || typeof value === "number"
          ? String(value)
          : ""
      }
      onChange={(event) => onChange(event.target.value)}
      aria-label={label}
    />
  );
}

/** The two ways a date operand is written: a calendar day, or a count back. */
const DATE_MODES = ["on", "ago"] as const;

/**
 * A date operand: a calendar day, or a number of days before the day the
 * filter runs. The second is what a list that must stay current is built on —
 * "no activity in 45 days" — and a literal date would go stale overnight.
 *
 * Switching mode starts the other operand empty rather than converting: a
 * conversion would write a value the reader did not type.
 */
function DateValue({
  value,
  onChange,
  label,
}: Readonly<{
  label: string;
  value: LeafValue;
  onChange: (next: LeafValue) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const faultId = useId();
  // The count as typed in "days ago" mode, null in date mode. The tree holds ""
  // while a count is being corrected, and "" names no mode.
  const [days, setDays] = useState(() => daysTextOf(value));
  // A change the tree made without this row, such as Discard, replaces it.
  const [seen, setSeen] = useState<LeafValue>(value);
  if (seen !== value) {
    setSeen(value);
    if (!holds(days, value)) {
      setDays(daysTextOf(value));
    }
  }
  const invalid =
    days !== null && days.trim() !== "" && !isRelativeDate(daysOperand(days));
  return (
    <div className="filter-value">
      <span className="filter-date-value">
        <SegmentedControl
          options={DATE_MODES}
          value={days === null ? "on" : "ago"}
          onChange={(next) => {
            const operand = next === "ago" ? { days_ago: 0 } : "";
            setDays(daysTextOf(operand));
            onChange(operand);
          }}
          labels={{ on: t("filters.date.on"), ago: t("filters.date.daysAgo") }}
          label={t("filters.date.mode")}
        />
        {days === null ? (
          <DateInput
            value={typeof value === "string" ? isoOrBlank(value) : ""}
            onChange={(event) => onChange(event.target.value)}
            aria-label={label}
          />
        ) : (
          <TextInput
            inputMode="numeric"
            value={days}
            onChange={(event) => {
              setDays(event.target.value);
              onChange(daysOperand(event.target.value));
            }}
            aria-label={t("filters.date.daysAgoCount", { field: label })}
            aria-invalid={invalid ? true : undefined}
            aria-describedby={invalid ? faultId : undefined}
          />
        )}
      </span>
      {invalid && (
        <ErrorLine id={faultId}>
          {t("filters.date.daysAgoInvalid", {
            max: formatNumber(MAX_DAYS_AGO, locale),
          })}
        </ErrorLine>
      )}
    </div>
  );
}

function daysTextOf(value: LeafValue): string | null {
  return isRelativeDate(value) ? String(value.days_ago) : null;
}

/**
 * The count as an operand, or "" while the text is not one. Not the text
 * itself: a non-empty string passes as an `eq` operand and the count would run.
 */
function daysOperand(text: string): LeafValue {
  const trimmed = text.trim();
  const operand = { days_ago: Number(trimmed) };
  return /^\d+$/.test(trimmed) && isRelativeDate(operand) ? operand : "";
}

/** Whether the row as held already shows this value. */
function holds(days: string | null, value: LeafValue): boolean {
  if (days === null) {
    return !isRelativeDate(value);
  }
  const typed = daysOperand(days);
  if (isRelativeDate(typed)) {
    return isRelativeDate(value) && value.days_ago === typed.days_ago;
  }
  return value === typed;
}
