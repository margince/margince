import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Checkbox, Field, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { useToast } from "../design-system/toast";
import { viewerZone } from "../format/timezone";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { QueryGate, throwProblem } from "./common";

// When this reader is bookable, and on whose clock.
//
// Personal, and the card says so: an admin does not set a colleague's working
// hours, which is why this sits on the account tab with no role gate and no
// refusal copy. One range and a set of days — the two shapes that prompted it
// (8-18 Monday to Saturday, 9-13 Monday to Thursday) are both that, and per-day
// hours can be added later without redoing this.
//
// docs/explanation/scheduling.md is the design.

type WorkingHours = components["schemas"]["WorkingHours"];

// ISO-8601 weekday numbers, Monday first — the order a week is read in, and the
// order the server stores.
const WEEK = [1, 2, 3, 4, 5, 6, 7] as const;

// dayLabelKey names each day's own key. Spelled out rather than composed,
// because `t` takes a key from a closed union: a computed key does not compile,
// and that is what keeps this honest about which days it has words for.
function dayLabelKey(day: (typeof WEEK)[number]): MessageKey {
  switch (day) {
    case 1:
      return "workingHours.day.1";
    case 2:
      return "workingHours.day.2";
    case 3:
      return "workingHours.day.3";
    case 4:
      return "workingHours.day.4";
    case 5:
      return "workingHours.day.5";
    case 6:
      return "workingHours.day.6";
    default:
      return "workingHours.day.7";
  }
}

export function useWorkingHours() {
  return useQuery({
    queryKey: ["working-hours"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/me/working-hours");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

// minutesOf reads `HH:MM` as minutes past midnight, so two times can be
// compared without a date to attach them to. NaN for anything else, which the
// caller treats as "cannot compare" rather than as zero.
function minutesOf(written: string): number {
  const [hour, minute] = written.split(":");
  const hours = Number(hour);
  const minutes = Number(minute);
  if (!Number.isInteger(hours) || !Number.isInteger(minutes)) {
    return Number.NaN;
  }
  return hours * 60 + minutes;
}

// narrowing reports whether the saved window is smaller than the one that was
// there before — fewer hours in a day, or fewer days in the week.
//
// It is the sentence the card owes the reader: a host who moves to 09:00-13:00
// will receive roughly half the bookings they do today and will not necessarily
// connect the two. Saying so at the moment they save is the difference between
// a setting and a trap.
function narrowing(before: WorkingHours, after: WorkingHours): boolean {
  const wasHours = minutesOf(before.end_time) - minutesOf(before.start_time);
  const nowHours = minutesOf(after.end_time) - minutesOf(after.start_time);
  if (Number.isNaN(wasHours) || Number.isNaN(nowHours)) {
    return false;
  }
  return nowHours < wasHours || after.days.length < before.days.length;
}

export function WorkingHoursCard() {
  const t = useT();
  const query = useWorkingHours();
  return (
    <Panel title={t("workingHours.title")}>
      <PanelBody className="form-stack">
        <p className="settings-panel-sub">{t("workingHours.sub")}</p>
        <QueryGate pendingLabel={t("workingHours.title")} query={query}>
          {(answer) => (
            <WorkingHoursForm
              chosen={answer.chosen}
              hours={answer.working_hours}
            />
          )}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

function WorkingHoursForm({
  chosen,
  hours,
}: Readonly<{ chosen: boolean; hours: WorkingHours }>) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  const [start, setStart] = useState(hours.start_time);
  const [end, setEnd] = useState(hours.end_time);
  const [days, setDays] = useState<readonly number[]>(hours.days);
  // The zone this browser is in, offered when the reader has chosen none. A
  // person confirming where they are is a better first write than a person
  // typing an IANA name, and the server stores whatever is confirmed.
  const [zone, setZone] = useState(chosen ? hours.timezone : viewerZone());
  const [narrowed, setNarrowed] = useState(false);

  const save = useMutation({
    scope: { id: "working-hours" },
    mutationFn: async (next: WorkingHours) => {
      const { data, error } = await api.PUT("/me/working-hours", {
        body: next,
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (data) => {
      queryClient.setQueryData(["working-hours"], data);
      toast.show(t("settings.saved"));
    },
  });

  const submit = () => {
    const next: WorkingHours = {
      start_time: start,
      end_time: end,
      days: [...days].sort((a, b) => a - b),
      timezone: zone,
    };
    // Computed against what was on the screen before this save, not against
    // the fallback: a reader who has already narrowed once and is now editing
    // a label should not be told again.
    setNarrowed(narrowing(hours, next));
    save.mutate(next);
  };

  return (
    <>
      {!chosen && <Callout tone="info">{t("workingHours.unset")}</Callout>}
      <div className="form-row">
        <Field label={t("workingHours.start")}>
          {(control) => (
            <TextInput
              {...control}
              type="time"
              value={start}
              onChange={(event) => setStart(event.target.value)}
            />
          )}
        </Field>
        <Field label={t("workingHours.end")}>
          {(control) => (
            <TextInput
              {...control}
              type="time"
              value={end}
              onChange={(event) => setEnd(event.target.value)}
            />
          )}
        </Field>
      </div>
      <fieldset className="form-stack">
        <legend>{t("workingHours.days")}</legend>
        {WEEK.map((day) => (
          <Checkbox
            key={day}
            label={t(dayLabelKey(day))}
            checked={days.includes(day)}
            onChange={(event) =>
              setDays(
                event.target.checked
                  ? [...days, day]
                  : days.filter((chosenDay) => chosenDay !== day),
              )
            }
          />
        ))}
      </fieldset>
      <Field
        label={t("workingHours.timezone")}
        hint={t("workingHours.timezoneHelp")}
      >
        {(control) => (
          <TextInput
            {...control}
            value={zone}
            onChange={(event) => setZone(event.target.value)}
          />
        )}
      </Field>
      <Button onClick={submit} disabled={save.isPending}>
        {t("workingHours.save")}
      </Button>
      {/* After the save, never before: the reader has made the change, and the
          sentence is about what it will do rather than a warning against
          making it. */}
      {narrowed && save.isSuccess && (
        <Callout tone="warn" live="status">
          {t("workingHours.narrowed")}
        </Callout>
      )}
      {save.isError && (
        <Callout tone="danger" live="alert">
          {t("workingHours.saveFailed")}
        </Callout>
      )}
    </>
  );
}
