import { FieldDiff } from "../../design-system/trust";
import type { OccurredPrecision } from "../../format/activitywhen";
import { type HistoryValueCtx, historyValue } from "../../lib/historyvalues";
import { useHistoryNameOf } from "./historynames";

// The caller's own resolver first, then the reader for the field's kind.
function useNamedValues(
  field: string,
  ids: readonly (string | null | undefined)[],
  values: HistoryValueCtx,
): HistoryValueCtx {
  const named = useHistoryNameOf(field, ids);
  return { ...values, nameOf: (id) => values.nameOf?.(id) ?? named(id) };
}

// One field's before and after, read the way the rest of the record page reads
// them.
//
// The invariant it holds is that a history value is scaled by its record's
// currency BEFORE a human sees it: the audit spine projects every value as a
// string, so a minor-unit column arrives as an integer count and rendered raw
// shows a figure a hundredfold wrong on most currencies. Every surface that
// draws a history diff goes through here, because the one place that must never
// disagree with the others is the number sitting beside a button that writes.
export function HistoryFieldDiff({
  field,
  oldValue,
  newValue,
  oldPrecision,
  newPrecision,
  values,
}: Readonly<{
  field: string;
  oldValue: string | null | undefined;
  newValue: string | null | undefined;
  // Each side's activity-time precision, when its image carries one.
  oldPrecision?: OccurredPrecision;
  newPrecision?: OccurredPrecision;
  // Everything a stored value needs to be read as what it MEANS: the record's
  // currency for a minor-unit column, its zone for a timestamp, and a resolver
  // for the ids a change row holds. One object because they travel together —
  // a row that scaled its money and still printed a uuid would be half-read.
  values: HistoryValueCtx;
}>) {
  const ctx = useNamedValues(field, [oldValue, newValue], values);
  return (
    <FieldDiff
      oldValue={historyValue(field, oldValue, ctx, oldPrecision)}
      newValue={historyValue(field, newValue, ctx, newPrecision)}
    />
  );
}

/** One side of a change, read as the diff reads it; `cleared` stands for none. */
export function HistoryFieldValue({
  field,
  value,
  precision,
  values,
  cleared,
}: Readonly<{
  field: string;
  value: string | null | undefined;
  precision?: OccurredPrecision;
  values: HistoryValueCtx;
  cleared: string;
}>) {
  const ctx = useNamedValues(field, [value], values);
  return <>{historyValue(field, value, ctx, precision) ?? cleared}</>;
}
