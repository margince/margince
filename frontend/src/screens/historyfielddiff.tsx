import { FieldDiff } from "../design-system/trust";
import type { Locale } from "../i18n";
import { historyValue } from "./historyvalues";

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
  currency,
  locale,
}: Readonly<{
  field: string;
  oldValue: string | null | undefined;
  newValue: string | null | undefined;
  // The record's ISO currency, which is what gives a minor-unit column its
  // scale. Absent on a record type that holds no money.
  currency: string | null | undefined;
  locale: Locale;
}>) {
  return (
    <FieldDiff
      oldValue={historyValue(field, oldValue, currency, locale)}
      newValue={historyValue(field, newValue, currency, locale)}
    />
  );
}
