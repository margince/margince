import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { useUnsavedGuard } from "../app/unsaved";
import { Button, Checkbox, Field, TextInput } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { useBookingCalendar } from "./booking-calendar-state";
import { BookingCalendars } from "./booking-calendars";
import { BookingProfileScreen } from "./booking-profile";
import { QueryGate, throwProblem, useMe } from "./common";
import { useConnectors } from "./connectors";
import { useSchedulingProfile } from "./scheduling-profile-query";
import { WorkingHoursCard } from "./working-hours";
import "./book.css";

type Profile = components["schemas"]["SchedulingProfile"];
export function MeetingSettings() {
  const t = useT();
  const me = useMe();
  const canBook = useCanWrite("activity", "create");
  return (
    <>
      {canBook && <BookingProfileScreen embedded />}
      <WorkingHoursCard />
      {canBook ? (
        <MeetingPreferences />
      ) : (
        !me.isPending && <ErrorLine>{t("scheduling.bookRefused")}</ErrorLine>
      )}
    </>
  );
}
function MeetingPreferences() {
  const t = useT();
  const query = useSchedulingProfile();
  return (
    <QueryGate pendingLabel={t("common.loading")} query={query}>
      {(profile) => <MeetingSettingsForm profile={profile} />}
    </QueryGate>
  );
}
function MeetingSettingsForm({ profile }: Readonly<{ profile: Profile }>) {
  const t = useT();
  const client = useQueryClient();
  const toast = useToast();
  const [baseline, setBaseline] = useState(profile);
  const [form, setForm] = useState(profile);
  const [noticeHours, setNoticeHours] = useState(
    String(Number((profile.notice_minutes / 60).toFixed(2))),
  );
  useUnsavedGuard(JSON.stringify(form) !== JSON.stringify(baseline));
  const {
    connections,
    providers,
    provider,
    calendarID,
    blocking,
    ready,
    calendars,
    selected,
    needsCalendar,
  } = useMeetingCalendar({ ...form, enabled: profile.enabled }, baseline);
  const save = useMutation({
    mutationFn: async ({
      original,
      next,
    }: {
      original: Profile;
      next: Profile;
    }) => {
      const latest = await api.GET("/scheduling/profile");
      if (latest.error) throwProblem(latest.error);
      const changes = changedMeetingPreferences(original, next);
      const { data, error } = await api.PUT("/scheduling/profile", {
        body: { ...latest.data, ...changes },
      });
      if (error) throwProblem(error);
      return data;
    },
    onSuccess: async (value) => {
      setBaseline(value);
      setForm(value);
      setNoticeHours(String(Number((value.notice_minutes / 60).toFixed(2))));
      client.setQueryData(["scheduling-profile"], value);
      await client.invalidateQueries({ queryKey: ["reliable-availability"] });
      toast.show(t("settings.saved"));
    },
  });
  return (
    <div className="book-form">
      <Panel title={t("scheduling.settings")}>
        <PanelBody>
          <form
            className="book-form"
            onSubmit={(e) => {
              e.preventDefault();
              const canChoose =
                ready && selected?.writable && !calendars.isFetching;
              save.mutate({
                original: baseline,
                next: {
                  ...form,
                  provider: canChoose ? provider : form.provider,
                  calendar_id: canChoose ? selected.id : form.calendar_id,
                  blocking_calendars: canChoose
                    ? blocking
                    : form.blocking_calendars,
                },
              });
            }}
          >
            {providers.length > 1 && (
              <Field label={t("scheduling.provider")}>
                {(control) => (
                  <Select
                    {...control}
                    value={provider}
                    options={[
                      { value: "", label: t("scheduling.chooseProvider") },
                      ...providers.map((item) => ({
                        value: item.provider,
                        label:
                          item.provider === "gcal"
                            ? "Google Calendar"
                            : "Microsoft Outlook",
                      })),
                    ]}
                    onChange={(value) => {
                      if (
                        value === "" ||
                        value === "gcal" ||
                        value === "graphcal"
                      )
                        setForm({
                          ...form,
                          provider: value,
                          calendar_id: "primary",
                          blocking_calendars: [],
                        });
                    }}
                  />
                )}
              </Field>
            )}
            {!connections.isPending &&
              !connections.error &&
              providers.length === 0 && (
                <p>{t("scheduling.disconnectedCalendar")}</p>
              )}
            {connections.isPending && <p>{t("common.loading")}</p>}
            <ErrorLine error={connections.error} />
            {provider ? (
              <BookingCalendars
                provider={provider}
                calendar={calendarID}
                blocking={blocking}
                onCalendar={(value) =>
                  setForm({
                    ...form,
                    provider,
                    calendar_id: value,
                    blocking_calendars: blocking.filter((id) => id !== value),
                  })
                }
                onBlocking={(value) =>
                  setForm({
                    ...form,
                    provider,
                    calendar_id: calendarID,
                    blocking_calendars: value,
                  })
                }
              />
            ) : (
              <a href="#/settings/connections" target="_blank" rel="noreferrer">
                {t("scheduling.manageConnection")}
              </a>
            )}
            {ready && calendars.data && !selected && (
              <ErrorLine>{t("scheduling.missingSavedCalendar")}</ErrorLine>
            )}
            {form.provider && provider !== form.provider && (
              <p className="t-caption">{t("scheduling.providerChanged")}</p>
            )}
            <Field label={t("scheduling.hostName")}>
              {(control) => (
                <TextInput
                  {...control}
                  readOnly
                  value={profile.host_name ?? ""}
                />
              )}
            </Field>
            <p className="t-caption">
              {t("scheduling.accountName")}{" "}
              <a href="#/settings/account">{t("settings.tab.account")}</a>
            </p>
            <p className="t-caption">
              {t("scheduling.anchorBrand")}{" "}
              <a href="#/settings/company">{t("settings.companyTitle")}</a>
            </p>
            <Field label={t("scheduling.subject")}>
              {(control) => (
                <TextInput
                  {...control}
                  required
                  value={form.title}
                  onChange={(e) => setForm({ ...form, title: e.target.value })}
                />
              )}
            </Field>
            <Field
              label={t("scheduling.location")}
              hint={t("scheduling.locationHelp")}
            >
              {(control) => (
                <TextInput
                  {...control}
                  placeholder={t("scheduling.locationExample")}
                  value={form.location}
                  onChange={(e) =>
                    setForm({ ...form, location: e.target.value })
                  }
                />
              )}
            </Field>
            <Checkbox
              label={t("scheduling.emailReminder")}
              checked={form.email_reminder ?? false}
              onChange={(event) =>
                setForm({ ...form, email_reminder: event.target.checked })
              }
            />
            <p className="t-caption">{t("scheduling.reminderHelp")}</p>
            <div className="book-policy-fields">
              <Field label={t("scheduling.duration")}>
                {(control) => (
                  <TextInput
                    {...control}
                    type="number"
                    min={15}
                    max={480}
                    value={form.duration_minutes}
                    onChange={(e) =>
                      setForm({
                        ...form,
                        duration_minutes: Number(e.target.value),
                      })
                    }
                  />
                )}
              </Field>
              <Field
                label={t("scheduling.notice")}
                hint={t("scheduling.noticeHelp")}
              >
                {(control) => (
                  <TextInput
                    {...control}
                    type="number"
                    min={0}
                    max={168}
                    step="any"
                    required
                    value={noticeHours}
                    onChange={(e) => {
                      setNoticeHours(e.target.value);
                      setForm({
                        ...form,
                        notice_minutes: Math.round(Number(e.target.value) * 60),
                      });
                    }}
                  />
                )}
              </Field>
              <Field label={t("scheduling.buffer")}>
                {(control) => (
                  <TextInput
                    {...control}
                    type="number"
                    min={0}
                    max={120}
                    value={form.buffer_minutes}
                    onChange={(e) =>
                      setForm({
                        ...form,
                        buffer_minutes: Number(e.target.value),
                      })
                    }
                  />
                )}
              </Field>
              <Field label={t("scheduling.horizon")}>
                {(control) => (
                  <TextInput
                    {...control}
                    type="number"
                    min={1}
                    max={90}
                    value={form.horizon_days}
                    onChange={(e) =>
                      setForm({ ...form, horizon_days: Number(e.target.value) })
                    }
                  />
                )}
              </Field>
            </div>
            <div className="book-actions">
              <Button
                type="submit"
                variant="primary"
                disabled={
                  save.isPending ||
                  connections.isPending ||
                  calendars.isFetching ||
                  (needsCalendar && (!ready || !selected?.writable))
                }
              >
                {t("scheduling.save")}
              </Button>
            </div>
          </form>
          <ErrorLine error={save.error} />
        </PanelBody>
      </Panel>
    </div>
  );
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
    "duration_minutes",
    "notice_minutes",
    "buffer_minutes",
    "horizon_days",
  ] as const;
  for (const field of fields) {
    if (JSON.stringify(original[field]) !== JSON.stringify(next[field]))
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
