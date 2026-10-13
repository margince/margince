// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useUnsavedGuard } from "../app/unsaved";
import {
  Badge,
  Button,
  EmptyState,
  Field,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { ChoiceList } from "../design-system/choicelist";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { TimezoneSelect } from "../design-system/timezoneselect";
import { useToast } from "../design-system/toast";
import { formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, QueryGate, throwProblem } from "./common";
import { amountLabel } from "./meeting-phrases";
import { putSchedulingChanges } from "./scheduling-profile-query";
import { dayLabelKey, narrowing, useWorkingHours, WEEK } from "./working-hours";
import "./meeting-cards.css";

type Profile = components["schemas"]["SchedulingProfile"];
type WorkingHours = components["schemas"]["WorkingHours"];
type RuleField = "notice_minutes" | "buffer_minutes" | "horizon_days";
type Rules = Readonly<Record<RuleField, number>>;

/** The card's id, so the link drawer's Change hours can bring it into view. */
export const AVAILABILITY_ID = "meeting-availability";

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
const RULES: readonly RuleSpec[] = [
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

/**
 * Working days and hours, and with the booking grant the rules a guest books
 * under, behind one Save. The calendar's state never blocks it: the rules are
 * the host's own, and saving them does not ask the calendar anything.
 */
export function AvailabilityCard({
  profile,
  hoursOnly = false,
  attention = false,
}: Readonly<{
  /** Absent for a reader who may set hours but not book. */
  profile?: Profile;
  hoursOnly?: boolean;
  /** Bookings are live while the calendar is down. */
  attention?: boolean;
}>) {
  const t = useT();
  const query = useWorkingHours();
  const answer = query.data;
  return (
    <div id={AVAILABILITY_ID} className="meeting-anchor" tabIndex={-1}>
      {answer?.working_hours ? (
        <AvailabilityForm
          chosen={answer.chosen}
          hours={answer.working_hours}
          rules={hoursOnly || !profile ? null : rulesOf(profile)}
          attention={attention}
        />
      ) : (
        <Panel title={t("scheduling.whenTitle")}>
          <PanelBody>
            <QueryGate query={query} pendingLabel={t("scheduling.whenTitle")}>
              {() => <EmptyState>{t("state.unavailable")}</EmptyState>}
            </QueryGate>
          </PanelBody>
        </Panel>
      )}
    </div>
  );
}

function rulesOf(profile: Profile): Rules {
  return {
    notice_minutes: profile.notice_minutes,
    buffer_minutes: profile.buffer_minutes,
    horizon_days: profile.horizon_days,
  };
}

// Days in order: the list reports picks in week order already, and a saved
// copy compared unsorted would stay unsaved after its own save.
function sortedHours(hours: WorkingHours): WorkingHours {
  return { ...hours, days: [...hours.days].sort((a, b) => a - b) };
}

type Draft = Readonly<{
  hours: WorkingHours | null;
  rules: Partial<Rules>;
}>;

function AvailabilityForm({
  chosen,
  hours,
  rules,
  attention,
}: Readonly<{
  chosen: boolean;
  hours: WorkingHours;
  rules: Rules | null;
  attention: boolean;
}>) {
  const t = useT();
  const [savedHours, setSavedHours] = useState(sortedHours(hours));
  const [draftHours, setDraftHours] = useState(sortedHours(hours));
  const [savedRules, setSavedRules] = useState(rules);
  const [draftRules, setDraftRules] = useState(rules);
  const [other, setOther] = useState<Partial<Record<RuleField, string>>>({});
  const [narrowed, setNarrowed] = useState(false);
  const save = useSaveAvailability(setSavedHours, (value) => {
    setSavedRules(value);
    setDraftRules(value);
    setOther({});
  });
  const hoursDirty =
    JSON.stringify(sortedHours(draftHours)) !== JSON.stringify(savedHours);
  const next = draftRules && {
    ...draftRules,
    ...Object.fromEntries(
      // An emptied field is no answer, not zero: Number("") would save 0.
      Object.entries(other).map(([field, text]) => [
        field,
        text.trim() === "" ? Number.NaN : Number(text),
      ]),
    ),
  };
  const invalid = RULES.some(
    (spec) =>
      other[spec.field] !== undefined &&
      !inRange(spec, next?.[spec.field] ?? Number.NaN),
  );
  const ruleChanges = changedRules(savedRules, next);
  const dirty = hoursDirty || Object.keys(ruleChanges).length > 0;
  useUnsavedGuard(dirty);
  const submit = () => {
    // Never-saved hours are saved as offered: accepting the defaults is an answer.
    const nextHours = hoursDirty || !chosen ? sortedHours(draftHours) : null;
    setNarrowed(nextHours !== null && narrowing(savedHours, nextHours));
    save.mutate({ hours: nextHours, rules: ruleChanges });
  };
  let foot: MessageKey | null = null;
  if (rules)
    foot = attention ? "scheduling.rulesFootAttention" : "scheduling.rulesFoot";
  return (
    <Panel
      title={t("scheduling.whenTitle")}
      titleAction={
        chosen ? undefined : <Badge>{t("workingHours.unsetTitle")}</Badge>
      }
      actions={
        <div className="meeting-card-save">
          {foot && <span className="t-caption">{t(foot)}</span>}
          <Button
            variant="primary"
            disabled={(chosen && !dirty) || invalid}
            pending={save.isPending}
            onClick={submit}
          >
            {t(rules ? "scheduling.saveAvailability" : "workingHours.save")}
          </Button>
        </div>
      }
    >
      <PanelBody className="form-stack">
        <HoursFields
          chosen={chosen}
          value={draftHours}
          onChange={setDraftHours}
        />
      </PanelBody>
      {next && (
        <PanelBody>
          <Heading as="h3" size="xsmall" className="meeting-subhead">
            {t("scheduling.bookingRules")}
          </Heading>
          <SettingList>
            {RULES.map((spec) => (
              <RuleRow
                key={spec.field}
                spec={spec}
                value={next[spec.field]}
                other={other[spec.field]}
                onPreset={(value) => {
                  setOther(
                    Object.fromEntries(
                      Object.entries(other).filter(
                        ([field]) => field !== spec.field,
                      ),
                    ),
                  );
                  setDraftRules({ ...next, [spec.field]: value });
                }}
                onOther={(text) => setOther({ ...other, [spec.field]: text })}
              />
            ))}
          </SettingList>
        </PanelBody>
      )}
      <PanelBody>
        {/* After the save, never before: it says what the change will do. */}
        {narrowed && save.isSuccess && (
          <Callout kind="outcome" title={t("workingHours.narrowedTitle")}>
            {t("workingHours.narrowed")}
          </Callout>
        )}
        {save.isError && (
          <Callout
            tone="danger"
            kind="outcome"
            title={t(
              hoursDirty || !chosen
                ? "workingHours.saveFailed"
                : "scheduling.rulesSaveFailed",
            )}
          >
            {problemMessageOf(save.error, t)}
          </Callout>
        )}
      </PanelBody>
    </Panel>
  );
}

function inRange(spec: RuleSpec, value: number): boolean {
  return Number.isInteger(value) && value >= spec.min && value <= spec.max;
}

function changedRules(saved: Rules | null, next: Rules | null): Partial<Rules> {
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
            value={other === undefined ? String(value) : "other"}
            options={[
              ...amounts.map((amount) => ({
                value: String(amount),
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

function HoursFields({
  chosen,
  value,
  onChange,
}: Readonly<{
  chosen: boolean;
  value: WorkingHours;
  onChange: (next: WorkingHours) => void;
}>) {
  const t = useT();
  const browserZone = viewerZone();
  return (
    <>
      {!chosen && <Callout kind="standing" title={t("workingHours.unset")} />}
      <ChoiceList
        multiple
        layout="row"
        legend={t("scheduling.days")}
        choices={WEEK.map((day) => ({
          value: String(day),
          label: t(dayLabelKey(day)),
        }))}
        value={value.days.map(String)}
        onChange={(days) => onChange({ ...value, days: days.map(Number) })}
      />
      <div className="form-row">
        <Field label={t("workingHours.from")}>
          {(control) => (
            <TextInput
              {...control}
              type="time"
              value={value.start_time}
              onChange={(event) =>
                onChange({ ...value, start_time: event.target.value })
              }
            />
          )}
        </Field>
        <Field label={t("workingHours.until")}>
          {(control) => (
            <TextInput
              {...control}
              type="time"
              value={value.end_time}
              onChange={(event) =>
                onChange({ ...value, end_time: event.target.value })
              }
            />
          )}
        </Field>
        <Field
          label={t("workingHours.timezone")}
          hint={t("workingHours.zoneHelp")}
        >
          {(control) => (
            <TimezoneSelect
              {...control}
              value={value.timezone}
              onChange={(timezone) => onChange({ ...value, timezone })}
            />
          )}
        </Field>
      </div>
      {!chosen && browserZone !== value.timezone && (
        <p className="t-caption">
          {t("workingHours.browserZone", { zone: browserZone })}
        </p>
      )}
    </>
  );
}

// Hours and the booking rules are two resources, saved as one: hours first,
// and stored the moment the server has them, so a rules write that fails
// after it leaves the hours showing as saved rather than as a draft.
function useSaveAvailability(
  onHoursSaved: (hours: WorkingHours) => void,
  onRulesSaved: (rules: Rules) => void,
) {
  const t = useT();
  const client = useQueryClient();
  const toast = useToast();
  return useMutation({
    scope: { id: "working-hours" },
    mutationFn: async ({ hours, rules }: Draft) => {
      if (hours) {
        const saved = await api.PUT("/me/working-hours", { body: hours });
        if (saved.error) throwProblem(saved.error);
        client.setQueryData(["working-hours"], saved.data);
        onHoursSaved(hours);
        await client.invalidateQueries({ queryKey: ["reliable-availability"] });
      }
      if (Object.keys(rules).length === 0) return null;
      return putSchedulingChanges(rules);
    },
    onSuccess: async (profile) => {
      if (profile) {
        onRulesSaved(rulesOf(profile));
        client.setQueryData(["scheduling-profile"], profile);
        await client.invalidateQueries({ queryKey: ["reliable-availability"] });
      }
      toast.show(t("settings.saved"));
    },
  });
}
