import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Field } from "../design-system/atoms";
import { Select } from "../design-system/select";
import { formatDate } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";

type ImportWindow = components["schemas"]["StartBackfillRequest"]["window"];
type Preview = components["schemas"]["BackfillPreview"];

export const IMPORT_WINDOW_LABELS = {
  "3m": "backfill.window3m",
  "6m": "backfill.window6m",
  "12m": "backfill.window12m",
  "24m": "backfill.window24m",
  "36m": "backfill.window36m",
  "60m": "backfill.window60m",
  "84m": "backfill.window84m",
  "120m": "backfill.window120m",
} satisfies Record<ImportWindow, MessageKey>;

export function isImportWindow(value: unknown): value is ImportWindow {
  return (
    typeof value === "string" && Object.hasOwn(IMPORT_WINDOW_LABELS, value)
  );
}

/**
 * The windows to offer: what this installation admits, or the whole supported
 * set when it says nothing.
 *
 * An empty or absent list is a fallback rather than an empty picker. The server
 * refuses a window above the cap at both the preview and the start, so the only
 * cost of offering too many is a refusal that names the real set — where
 * offering none would leave somebody unable to import at all because a field
 * was missing.
 */
function offeredOrAll(offered?: readonly ImportWindow[]): ImportWindow[] {
  const all = Object.keys(IMPORT_WINDOW_LABELS) as ImportWindow[];
  if (!offered || offered.length === 0) return all;
  return all.filter((window) => offered.includes(window));
}

export function ImportWindowPicker({
  value,
  onChange,
  preview,
  offered,
}: Readonly<{
  value: ImportWindow;
  onChange: (value: ImportWindow) => void;
  preview?: Preview;
  /** The windows this installation admits; absent means the whole set. */
  offered?: readonly ImportWindow[];
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  // The server supplies the queried calendar date; browser clocks and month-end
  // arithmetic cannot silently change the period the estimate describes.
  // The shared formatter preserves this calendar day in the record zone.
  const after = preview?.window === value ? preview.after_date : undefined;
  return (
    <Field
      label={t("backfill.windowLabel")}
      hint={
        after
          ? t("backfill.since", { date: formatDate(after, locale, zone) })
          : undefined
      }
      hintLive
    >
      {(control) => (
        <Select
          {...control}
          value={value}
          options={offeredOrAll(offered).map((window) => ({
            value: window,
            label: t(IMPORT_WINDOW_LABELS[window]),
          }))}
          onChange={(next) => {
            // Select emits strings; narrow to the contract-backed option set.
            if (isImportWindow(next)) onChange(next);
          }}
        />
      )}
    </Field>
  );
}
