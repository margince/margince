import { SegmentedControl } from "../design-system/atoms";
import { useT } from "../i18n";
import { CALL_OUTCOMES, type CallOutcome } from "../lib/activitybody";
import { CALL_OUTCOME_LABEL } from "../lib/calloutcome";

// "No outcome" first: the control always holds a value, and a call saved
// without an outcome is a whole call.
const OUTCOME_OPTIONS: readonly (CallOutcome | "")[] = ["", ...CALL_OUTCOMES];

/** How the call ended, picked with one click. */
export function CallOutcomeField({
  value,
  onChange,
}: Readonly<{
  value: CallOutcome | "";
  onChange: (outcome: CallOutcome | "") => void;
}>) {
  const t = useT();
  const labels = { "": t("log.callOutcomeNone") } as Record<
    CallOutcome | "",
    string
  >;
  for (const outcome of CALL_OUTCOMES) {
    labels[outcome] = t(CALL_OUTCOME_LABEL[outcome]);
  }
  return (
    <div className="field">
      <span className="t-label" aria-hidden>
        {t("log.callOutcome")}
      </span>
      <SegmentedControl
        label={t("log.callOutcome")}
        options={OUTCOME_OPTIONS}
        value={value}
        labels={labels}
        onChange={onChange}
      />
    </div>
  );
}
