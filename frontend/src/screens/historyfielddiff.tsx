import { FieldDiff } from "../design-system/trust";
import { type HistoryValueCtx, historyValue } from "./historyvalues";

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
  values,
}: Readonly<{
  field: string;
  oldValue: string | null | undefined;
  newValue: string | null | undefined;
  // Everything a stored value needs to be read as what it MEANS: the record's
  // currency for a minor-unit column, its zone for a timestamp, and a resolver
  // for the ids a change row holds. One object because they travel together —
  // a row that scaled its money and still printed a uuid would be half-read.
  values: HistoryValueCtx;
}>) {
  return (
    <FieldDiff
      oldValue={historyValue(field, oldValue, values)}
      newValue={historyValue(field, newValue, values)}
    />
  );
}
