// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EmptyState, Field } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ChoiceList } from "../../design-system/choicelist";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { useSaveDraft } from "../../design-system/savepage";
import { StatusBadge } from "../../design-system/statusbadge";
import { clockOrBlank, TimeInput } from "../../design-system/timeinput";
import { TimezoneSelect } from "../../design-system/timezoneselect";
import { useToast } from "../../design-system/toast";
import { timezoneOption, viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { unwrap } from "../../lib/problem";
import { putSchedulingChanges } from "../../lib/scheduling-profile-query";
import {
  BookingRules,
  changedRules,
  inRange,
  RULES,
  type RuleField,
  type Rules,
} from "./meeting-availability-rules";
import { dayLabelKey, narrowing, useWorkingHours, WEEK } from "./working-hours";
import "./meeting-cards.css";

type Profile = components["schemas"]["SchedulingProfile"];
type WorkingHours = components["schemas"]["WorkingHours"];

/** The card's id, so the link drawer's Change hours can bring it into view. */
export const AVAILABILITY_ID = "meeting-availability";

/**
 * Working days and hours, and with the booking grant the rules a guest books
 * under, saved together through the page's save bar. The calendar's state never blocks it: the rules are
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

type Draft = Readonly<{ hours: WorkingHours | null; rules: Partial<Rules> }>;

// The card's drafts against what was last saved, and the one write for both.
function useAvailabilityDraft(
  chosen: boolean,
  hours: WorkingHours,
  rules: Rules | null,
) {
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
  useSaveDraft({
    dirty,
    // Never-chosen hours are an answer waiting to be saved as offered.
    offered: !chosen,
    invalid,
    save: () => {
      const nextHours = hoursDirty || !chosen ? sortedHours(draftHours) : null;
      setNarrowed(nextHours !== null && narrowing(savedHours, nextHours));
      return save.mutateAsync({ hours: nextHours, rules: ruleChanges });
    },
    discard: () => {
      save.reset();
      setDraftHours(savedHours);
      setDraftRules(savedRules);
      setOther({});
    },
  });
  const pickPreset = (field: RuleField, value: number) => {
    const { [field]: _picked, ...kept } = other;
    setOther(kept);
    if (next) setDraftRules({ ...next, [field]: value });
  };
  return {
    draftHours,
    setDraftHours,
    next,
    other,
    setOther,
    pickPreset,
    narrowed,
    save,
  };
}

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
  const draft = useAvailabilityDraft(chosen, hours, rules);
  const { next, other, save } = draft;
  let foot: MessageKey | null = null;
  if (rules)
    foot = attention ? "scheduling.rulesFootAttention" : "scheduling.rulesFoot";
  return (
    <Panel
      title={t("scheduling.whenTitle")}
      titleAction={
        chosen ? undefined : (
          <StatusBadge kind="neutral" label={t("workingHours.unsetTitle")} />
        )
      }
      footer={foot && <span className="t-caption">{t(foot)}</span>}
    >
      <PanelBody className="form-stack">
        <HoursFields
          chosen={chosen}
          value={draft.draftHours}
          onChange={draft.setDraftHours}
        />
      </PanelBody>
      {next && (
        <BookingRules
          rules={next}
          other={other}
          onPreset={draft.pickPreset}
          onOther={(field, text) => draft.setOther({ ...other, [field]: text })}
        />
      )}
      {/* After the save, never before: it says what the change will do. */}
      {draft.narrowed && save.isSuccess && (
        <PanelBody>
          <Callout kind="outcome" title={t("workingHours.narrowedTitle")}>
            {t("workingHours.narrowed")}
          </Callout>
        </PanelBody>
      )}
    </Panel>
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
  const { locale } = useLocale();
  const [now] = useState(() => new Date());
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
      <div className="form-row meeting-hours">
        <Field label={t("workingHours.from")}>
          {(control) => (
            <TimeInput
              {...control}
              value={clockOrBlank(value.start_time)}
              onChange={(event) =>
                onChange({ ...value, start_time: event.target.value })
              }
            />
          )}
        </Field>
        <Field label={t("workingHours.until")}>
          {(control) => (
            <TimeInput
              {...control}
              untilMidnight
              value={clockOrBlank(value.end_time, true)}
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
          {t("workingHours.browserZone", {
            zone: timezoneOption(locale, browserZone, now).label,
          })}
        </p>
      )}
    </>
  );
}

// Hours and the booking rules are two resources, saved as one: hours first, and
// stored the moment the server has them. A rules write that fails after it
// leaves the hours showing as saved rather than as a draft.
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
        const saved = unwrap(
          await api.PUT("/me/working-hours", { body: hours }),
        );
        client.setQueryData(["working-hours"], saved);
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
