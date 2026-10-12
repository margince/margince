import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { useUnsavedGuard } from "../app/unsaved";
import { Button, EmptyState, PendingBody } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { SaveBar } from "../design-system/savebar";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { useBookingCalendar } from "./booking-calendar-state";
import { BookingProfileScreen } from "./booking-profile";
import { QueryGate, throwProblem, unwrap, useMe } from "./common";
import { useConnectors } from "./connectors";
import {
  CalendarSection,
  DefaultFields,
  LimitFields,
  QuickLink,
  SetupChecklist,
} from "./meeting-settings-sections";
import { useSchedulingProfile } from "./scheduling-profile-query";
import {
  narrowing,
  useWorkingHours,
  WorkingHoursCard,
  WorkingHoursFields,
  WorkingHoursOutcome,
} from "./working-hours";
import "./book.css";

type Profile = components["schemas"]["SchedulingProfile"];
type WorkingHours = components["schemas"]["WorkingHours"];
type Draft = Readonly<{
  original: Profile;
  next: Profile;
  hours: WorkingHours | null;
}>;

export function MeetingSettings() {
  const t = useT();
  const me = useMe();
  const canBook = useCanWrite("activity", "create");
  // Until the grant is known, neither form: the hours-only card would flash
  // and take a first edit with it when the full page replaced it.
  if (me.isPending) return <PendingBody label={t("scheduling.settings")} />;
  if (!canBook)
    return (
      <>
        <WorkingHoursCard />
        <ErrorLine>{t("scheduling.bookRefused")}</ErrorLine>
      </>
    );
  return <MeetingPreferences />;
}

function MeetingPreferences() {
  const t = useT();
  const profile = useSchedulingProfile();
  const hours = useWorkingHours();
  return (
    <QueryGate pendingLabel={t("common.loading")} query={profile}>
      {(saved) => (
        <QueryGate pendingLabel={t("workingHours.title")} query={hours}>
          {(answer) => (
            <MeetingSettingsForm
              profile={saved}
              hours={answer.working_hours}
              chosen={answer.chosen}
            />
          )}
        </QueryGate>
      )}
    </QueryGate>
  );
}

function noticeText(profile: Profile) {
  return String(Number((profile.notice_minutes / 60).toFixed(2)));
}

function MeetingSettingsForm({
  profile,
  hours,
  chosen,
}: Readonly<{ profile: Profile; hours?: WorkingHours; chosen: boolean }>) {
  const t = useT();
  const [baseline, setBaseline] = useState(profile);
  const [form, setForm] = useState(profile);
  const [noticeHours, setNoticeHours] = useState(noticeText(profile));
  const [savedHours, setSavedHours] = useState(hours);
  const [hoursDraft, setHoursDraft] = useState(hours);
  const [narrowed, setNarrowed] = useState(false);
  const calendar = useMeetingCalendar(
    { ...form, enabled: profile.enabled },
    baseline,
  );
  const { provider, ready, selected, calendars, connections } = calendar;
  const profileDirty =
    JSON.stringify(form) !== JSON.stringify(baseline) ||
    (provider !== "" && provider !== baseline.provider);
  const hoursDirty =
    JSON.stringify(hoursDraft && savedForm(hoursDraft)) !==
    JSON.stringify(savedHours);
  useUnsavedGuard(profileDirty || hoursDirty);
  const save = useSaveMeetingSettings(setSavedHours, (value) => {
    setBaseline(value);
    setForm(value);
    setNoticeHours(noticeText(value));
  });
  const discard = () => {
    setForm(baseline);
    setNoticeHours(noticeText(baseline));
    setHoursDraft(savedHours);
  };
  const blocked =
    profileDirty &&
    (connections.isPending ||
      calendars.isFetching ||
      (calendar.needsCalendar && (!ready || !selected?.writable)));
  return (
    <div className="book-form">
      <QuickLink profile={profile} />
      <SetupChecklist
        calendar={ready && !!selected?.writable}
        hours={chosen}
        link={profile.enabled}
      />
      <form
        className="book-form"
        onSubmit={(e) => {
          e.preventDefault();
          const draft = settingsDraft(
            form,
            baseline,
            calendar,
            hoursDirty ? hoursDraft : undefined,
          );
          if (draft.hours && savedHours)
            setNarrowed(narrowing(savedHours, draft.hours));
          save.mutate(draft);
        }}
      >
        <Panel title={t("scheduling.calendarTitle")}>
          <PanelBody className="book-form">
            <PanelIntro>{t("scheduling.calendarIntro")}</PanelIntro>
            <CalendarSection
              providers={calendar.providers}
              provider={provider}
              calendarID={calendar.calendarID}
              blocking={calendar.blocking}
              connectionsPending={connections.isPending}
              connectionsError={connections.error}
              missingSaved={ready && !!calendars.data && !selected}
              providerChanged={!!form.provider && provider !== form.provider}
              onProvider={(value) =>
                setForm({
                  ...form,
                  provider: value,
                  calendar_id: "primary",
                  blocking_calendars: [],
                })
              }
              onCalendar={(value) =>
                setForm({
                  ...form,
                  provider,
                  calendar_id: value,
                  blocking_calendars: calendar.blocking.filter(
                    (id) => id !== value,
                  ),
                })
              }
              onBlocking={(value) =>
                setForm({
                  ...form,
                  provider,
                  calendar_id: calendar.calendarID,
                  blocking_calendars: value,
                })
              }
            />
          </PanelBody>
        </Panel>
        <Panel title={t("scheduling.availabilityTitle")}>
          <PanelBody className="book-form">
            <PanelIntro>{t("workingHours.sub")}</PanelIntro>
            {hoursDraft ? (
              <WorkingHoursFields
                chosen={chosen}
                value={hoursDraft}
                onChange={setHoursDraft}
              />
            ) : (
              <EmptyState>{t("state.unavailable")}</EmptyState>
            )}
            <LimitFields
              form={form}
              noticeHours={noticeHours}
              onNotice={(value) => {
                setNoticeHours(value);
                setForm({
                  ...form,
                  notice_minutes: Math.round(Number(value) * 60),
                });
              }}
              onChange={setForm}
            />
            <WorkingHoursOutcome
              narrowed={narrowed}
              save={{ isSuccess: save.isSuccess, isError: false, error: null }}
            />
          </PanelBody>
        </Panel>
        <Panel title={t("scheduling.defaultsTitle")}>
          <PanelBody className="book-form">
            <PanelIntro>{t("scheduling.defaultsIntro")}</PanelIntro>
            <DefaultFields form={form} provider={provider} onChange={setForm} />
          </PanelBody>
        </Panel>
        {(profileDirty || hoursDirty) && (
          <SaveBar label={t("scheduling.unsaved")}>
            <div className="book-actions">
              <span className="t-name">{t("scheduling.unsaved")}</span>
              <Button disabled={save.isPending} onClick={discard}>
                {t("scheduling.discard")}
              </Button>
              <Button
                type="submit"
                variant="primary"
                disabled={blocked}
                pending={save.isPending}
              >
                {t("scheduling.save")}
              </Button>
            </div>
            <ErrorLine error={save.error} />
          </SaveBar>
        )}
      </form>
      <BookingProfileScreen embedded />
    </div>
  );
}

// Hours and the scheduling profile are two resources with two endpoints, saved
// here as one draft: hours first, because a profile saved without them would
// publish a link against the old window.
function useSaveMeetingSettings(
  onHoursSaved: (hours: WorkingHours) => void,
  onSaved: (profile: Profile) => void,
) {
  const t = useT();
  const client = useQueryClient();
  const toast = useToast();
  return useMutation({
    mutationFn: async ({ original, next, hours }: Draft) => {
      if (hours) {
        const saved = await api.PUT("/me/working-hours", { body: hours });
        if (saved.error) throwProblem(saved.error);
        // Settled the moment the server has them, so a profile write that
        // fails after this does not leave saved hours showing as unsaved.
        client.setQueryData(["working-hours"], saved.data);
        onHoursSaved(hours);
        await client.invalidateQueries({ queryKey: ["reliable-availability"] });
      }
      const latest = await api.GET("/scheduling/profile");
      if (latest.error) throwProblem(latest.error);
      const changes = changedMeetingPreferences(original, next);
      if (Object.keys(changes).length === 0) return latest.data;
      return unwrap(
        await api.PUT("/scheduling/profile", {
          body: { ...latest.data, ...changes },
        }),
      );
    },
    onSuccess: async (profile) => {
      onSaved(profile);
      client.setQueryData(["scheduling-profile"], profile);
      await client.invalidateQueries({ queryKey: ["reliable-availability"] });
      toast.show(t("settings.saved"));
    },
  });
}

// What a save sends: the calendar the page resolved (a sole provider is chosen
// for the host) only once it is known to accept invitations.
function settingsDraft(
  form: Profile,
  baseline: Profile,
  calendar: ReturnType<typeof useMeetingCalendar>,
  hours: WorkingHours | undefined,
): Draft {
  const { ready, selected, calendars, provider, blocking } = calendar;
  const choose = ready && selected?.writable && !calendars.isFetching;
  return {
    original: baseline,
    hours: hours ? savedForm(hours) : null,
    next: {
      ...form,
      provider: choose ? provider : form.provider,
      calendar_id: choose ? selected.id : form.calendar_id,
      blocking_calendars: choose ? blocking : form.blocking_calendars,
    },
  };
}

// Hours as the server keeps them, days in order: the picker appends a day
// re-ticked, and a draft compared unsorted would stay unsaved after its save.
function savedForm(hours: WorkingHours): WorkingHours {
  return { ...hours, days: [...hours.days].sort((a, b) => a - b) };
}

function changedMeetingPreferences(
  original: Profile,
  next: Profile,
): Partial<Profile> {
  let changes: Partial<Profile> = {};
  const fields = [
    "provider",
    "calendar_id",
    "blocking_calendars",
    "title",
    "location",
    "email_reminder",
    "video_call",
    "duration_minutes",
    "notice_minutes",
    "buffer_minutes",
    "horizon_days",
  ] as const;
  // An unset calendar list and an empty one are the same answer; treating them
  // as a change would write the profile on a save that only touched hours.
  const comparable = (profile: Profile, field: (typeof fields)[number]) =>
    JSON.stringify(
      field === "blocking_calendars"
        ? (profile.blocking_calendars ?? [])
        : profile[field],
    );
  for (const field of fields) {
    if (comparable(original, field) !== comparable(next, field))
      changes = { ...changes, [field]: next[field] };
  }
  return changes;
}

function useMeetingCalendar(form: Profile, baseline: Profile) {
  const connections = useConnectors({ refetchOnWindowFocus: "always" });
  const providers =
    connections.data?.data.filter(
      (item) =>
        (item.provider === "gcal" || item.provider === "graphcal") &&
        item.status !== "disconnected",
    ) ?? [];
  const sole = providers.length === 1 ? providers[0].provider : "";
  const provider = providers.some((item) => item.provider === form.provider)
    ? form.provider
    : sole === "gcal" || sole === "graphcal"
      ? sole
      : form.provider;
  const calendarID = provider === form.provider ? form.calendar_id : "primary";
  const blocking =
    provider === form.provider ? (form.blocking_calendars ?? []) : [];
  const { ready, calendars } = useBookingCalendar(provider);
  const selected = calendars.data?.find(
    (item) =>
      item.id === calendarID || (calendarID === "primary" && item.primary),
  );
  const calendarEdited =
    form.provider !== baseline.provider ||
    form.calendar_id !== baseline.calendar_id ||
    JSON.stringify(form.blocking_calendars) !==
      JSON.stringify(baseline.blocking_calendars);
  const needsCalendar = form.enabled || calendarEdited;
  return {
    connections,
    providers,
    provider,
    calendarID,
    blocking,
    ready,
    calendars,
    selected,
    needsCalendar,
  };
}
