// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button, Field } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Select } from "../design-system/select";
import { formatNumber, identifierNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { problemCodeOf, problemMessageOf } from "./common";

// The task sheet's timeout control and the two ways a read or a save of the
// task's settings can fail.

const STEPS = [5, 10, 15, 20, 30, 45, 60, 90, 120, 180, 240, 300];

export function timeoutOptions(
  [low, high]: readonly [number, number],
  fallback: number,
  current: number = fallback,
): number[] {
  return [...new Set([...STEPS, fallback, current])]
    .filter((s) => s >= low && s <= high)
    .sort((a, b) => a - b);
}

/** The stored settings could not be read: the controls stay off until they are. */
export function ReadProblem({
  error,
  onRetry,
}: Readonly<{ error: Error | null; onRetry: () => void }>) {
  const t = useT();
  if (!error) return null;
  return (
    <Callout tone="danger" kind="outcome" title={t("aiTaskSheet.readFailed")}>
      {problemMessageOf(error, t)}{" "}
      <Button onClick={onRetry}>{t("common.retry")}</Button>
    </Callout>
  );
}

/** A refused save: a colleague's newer save, or the server's own reason. */
export function SaveProblem({ error }: Readonly<{ error: Error | null }>) {
  const t = useT();
  if (!error) return null;
  if (problemCodeOf(error) === "version_skew") {
    return (
      <Callout tone="warning" kind="standing" title={t("aiTaskSheet.conflict")}>
        {t("aiTaskSheet.conflict.help")}
      </Callout>
    );
  }
  return (
    <Callout tone="danger" kind="outcome" title={t("aiTaskSheet.saveFailed")}>
      {problemMessageOf(error, t)}
    </Callout>
  );
}

export function TimeoutField({
  label,
  hint,
  bounds,
  value,
  fallback,
  disabled,
  onChange,
}: Readonly<{
  label: string;
  hint: string;
  bounds: readonly [number, number];
  value: number;
  fallback: number;
  disabled: boolean;
  onChange: (seconds: number) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <Field label={label} hint={hint}>
      {(control) => (
        <Select
          {...control}
          value={identifierNumber(value)}
          disabled={disabled}
          options={timeoutOptions(bounds, fallback, value).map((s) => ({
            value: identifierNumber(s),
            label:
              s === fallback
                ? t("aiTaskSheet.seconds.default", {
                    seconds: formatNumber(s, locale),
                  })
                : t("aiTaskSheet.seconds", {
                    seconds: formatNumber(s, locale),
                  }),
          }))}
          onChange={(next) => onChange(Number(next))}
        />
      )}
    </Field>
  );
}
