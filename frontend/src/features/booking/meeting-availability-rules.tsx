// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Field, TextInput } from "../../design-system/atoms";
import { PanelBody, PanelGroupHead } from "../../design-system/panel";
import { Select } from "../../design-system/select";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { formatNumber, identifierNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { amountLabel } from "./meeting-phrases";

export type RuleField = "notice_minutes" | "buffer_minutes" | "horizon_days";
export type Rules = Readonly<Record<RuleField, number>>;

type RuleSpec = Readonly<{
  field: RuleField;
  label: MessageKey;
  help: MessageKey;
  presets: readonly number[];
  min: number;
  max: number;
  unit: "minutes" | "days";
}>;

// Bounds are the contract's (SchedulingProfile in crm.yaml); the presets are
// the answers most hosts give, with Other for the rest.
export const RULES: readonly RuleSpec[] = [
  {
    field: "notice_minutes",
    label: "scheduling.noticeRow",
    help: "scheduling.noticeRowHelp",
    presets: [0, 60, 120, 240, 480, 1440, 2880, 10080],
    min: 0,
    max: 10080,
    unit: "minutes",
  },
  {
    field: "buffer_minutes",
    label: "scheduling.bufferRow",
    help: "scheduling.bufferRowHelp",
    presets: [0, 5, 10, 15, 30, 60],
    min: 0,
    max: 120,
    unit: "minutes",
  },
  {
    field: "horizon_days",
    label: "scheduling.horizonRow",
    help: "scheduling.horizonRowHelp",
    presets: [7, 14, 30, 60, 90],
    min: 1,
    max: 90,
    unit: "days",
  },
];

export function inRange(spec: RuleSpec, value: number): boolean {
  return Number.isInteger(value) && value >= spec.min && value <= spec.max;
}

export function changedRules(
  saved: Rules | null,
  next: Rules | null,
): Partial<Rules> {
  if (!saved || !next) return {};
  return Object.fromEntries(
    RULES.map((spec) => spec.field)
      .filter((field) => saved[field] !== next[field])
      .map((field) => [field, next[field]]),
  );
}

function RuleRow({
  spec,
  value,
  other,
  onPreset,
  onOther,
}: Readonly<{
  spec: RuleSpec;
  value: number;
  other: string | undefined;
  onPreset: (value: number) => void;
  onOther: (text: string) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  // A saved value off the presets stays a choice of its own, so opening the
  // list never silently offers to change it.
  const amounts = [...new Set([...spec.presets, value])]
    .filter((amount) => Number.isInteger(amount))
    .sort((a, b) => a - b);
  const valid = inRange(spec, value);
  return (
    <SettingRow
      label={t(spec.label)}
      description={t(spec.help)}
      control={(control) => (
        <div className="meeting-rule">
          <Select
            {...control}
            value={other === undefined ? identifierNumber(value) : "other"}
            options={[
              ...amounts.map((amount) => ({
                value: identifierNumber(amount),
                label: amountLabel(t, plural, locale, amount, spec.unit),
              })),
              { value: "other", label: t("scheduling.ruleOther") },
            ]}
            onChange={(picked) =>
              picked === "other"
                ? onOther(String(value))
                : onPreset(Number(picked))
            }
          />
          {other !== undefined && (
            <Field
              label={t(
                spec.unit === "days"
                  ? "scheduling.ruleDays"
                  : "scheduling.ruleMinutes",
              )}
              error={
                valid
                  ? undefined
                  : t("scheduling.ruleInvalid", {
                      min: formatNumber(spec.min, locale),
                      max: formatNumber(spec.max, locale),
                    })
              }
            >
              {(field) => (
                <TextInput
                  {...field}
                  type="number"
                  inputMode="numeric"
                  min={spec.min}
                  max={spec.max}
                  value={other}
                  onChange={(event) => onOther(event.target.value)}
                />
              )}
            </Field>
          )}
        </div>
      )}
    />
  );
}

/** The booking rules group of the availability card: one row per rule. */
export function BookingRules({
  rules,
  other,
  onPreset,
  onOther,
}: Readonly<{
  rules: Rules;
  other: Partial<Record<RuleField, string>>;
  onPreset: (field: RuleField, value: number) => void;
  onOther: (field: RuleField, text: string) => void;
}>) {
  const t = useT();
  return (
    <>
      <PanelGroupHead level="h3" title={t("scheduling.bookingRules")} />
      <PanelBody>
        <SettingList>
          {RULES.map((spec) => (
            <RuleRow
              key={spec.field}
              spec={spec}
              value={rules[spec.field]}
              other={other[spec.field]}
              onPreset={(value) => onPreset(spec.field, value)}
              onOther={(text) => onOther(spec.field, text)}
            />
          ))}
        </SettingList>
      </PanelBody>
    </>
  );
}
