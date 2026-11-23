// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Ref } from "react";
import type { components } from "../../api/schema";
import { Field, type FieldControl, TextInput } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import { lockFor } from "../../format/settinglocks";
import { useT } from "../../i18n";

type InstallationSettings = components["schemas"]["InstallationSettings"];

// Which remaining deals a projected landing is built from, in the order the
// server offers them: strictest first, then the two that soften it.
//
// Typed as the schema's own union with `satisfies`. A measure added to the
// contract and not to this list fails the typecheck, rather than going unoffered
// with no sign.
type ForwardMeasure = NonNullable<
  components["schemas"]["InstallationSettings"]["forecast_forward_measure"]
>;

const FORWARD_MEASURES = [
  "likely",
  "weighted",
  "manager_call",
] as const satisfies readonly ForwardMeasure[];

/** The Forecast tab's facts, as the installation dialog edits them. */
export function ForecastRuleFields({
  locks,
  draft,
  canManage,
  refused,
  describe,
  thresholdRef,
  onChange,
}: Readonly<{
  locks?: InstallationSettings["locks"];
  draft: InstallationSettings;
  canManage: boolean;
  refused: ReadonlyMap<string, string>;
  describe: (control: FieldControl) => string | undefined;
  /** The threshold's input, when focus is to land there. */
  thresholdRef?: Ref<HTMLInputElement>;
  onChange: (next: InstallationSettings) => void;
}>) {
  const t = useT();
  const forwardMeasure = lockFor(
    locks,
    "installation.forecast_forward_measure",
    t,
  );
  const likelyThreshold = lockFor(
    locks,
    "installation.forecast_likely_threshold",
    t,
  );
  return (
    <>
      <div data-fact="forecast_forward_measure">
        <Field
          label={t("installationSettings.forwardMeasure")}
          hint={
            forwardMeasure.disabled
              ? forwardMeasure.reason
              : t("installationSettings.forwardMeasureHint")
          }
          error={refused.get("forecast_forward_measure")}
        >
          {(control) => (
            <Select
              {...control}
              aria-describedby={describe(control)}
              value={draft.forecast_forward_measure}
              disabled={!canManage || forwardMeasure.disabled}
              options={FORWARD_MEASURES.map((measure) => ({
                value: measure,
                label: t(`installationSettings.forwardMeasure.${measure}`),
              }))}
              // Narrowed back through the offered set rather than cast. A value
              // this build does not know never reaches the draft. So it is not
              // saved as a measure the server would refuse on every read.
              onChange={(next) => {
                const picked = FORWARD_MEASURES.find(
                  (measure) => measure === next,
                );
                if (picked) {
                  onChange({
                    ...draft,
                    forecast_forward_measure: picked,
                  });
                }
              }}
            />
          )}
        </Field>
      </div>

      <div data-fact="forecast_likely_threshold">
        <Field
          label={t("installationSettings.likelyThreshold")}
          hint={
            likelyThreshold.disabled
              ? likelyThreshold.reason
              : t("installationSettings.likelyThreshold.help")
          }
          error={refused.get("forecast_likely_threshold")}
        >
          {(control) => (
            <TextInput
              {...control}
              aria-describedby={describe(control)}
              ref={thresholdRef}
              type="number"
              inputMode="numeric"
              min={1}
              max={100}
              step={1}
              value={String(draft.forecast_likely_threshold || "")}
              disabled={!canManage || likelyThreshold.disabled}
              // A cleared or broken entry is held as 0, which the server
              // refuses on this field, rather than being dropped unseen.
              onChange={(event) => {
                const percent = Number(event.target.value);
                onChange({
                  ...draft,
                  forecast_likely_threshold: Number.isFinite(percent)
                    ? percent
                    : 0,
                });
              }}
            />
          )}
        </Field>
      </div>
    </>
  );
}
