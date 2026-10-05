import { CheckCircle2, Circle, Copy } from "lucide-react";
import type { components } from "../api/schema";
import {
  Badge,
  Button,
  Field,
  SegmentedControl,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { useClipboardCopy } from "../design-system/clipboardcopy";
import { ErrorLine } from "../design-system/errorline";
import { Select } from "../design-system/select";
import { Switch } from "../design-system/switch";
import { formatNumber, identifierNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { BookingCalendars } from "./booking-calendars";

type Profile = components["schemas"]["SchedulingProfile"];
type Provider = "" | "gcal" | "graphcal";

const LENGTHS = ["15", "30", "45", "60"];

// The link a host most often comes here to fetch, copyable before anything
// else on the page; its full controls stay in the booking link section.
export function QuickLink({ profile }: Readonly<{ profile: Profile }>) {
  const t = useT();
  const copy = useClipboardCopy(profile.public_url ?? "", {
    copy: t("scheduling.copyBookingLink"),
    copied: t("scheduling.copied"),
    remedy: t("scheduling.copyFallback"),
  });
  if (!profile.public_url) return null;
  return (
    <div className="meeting-quicklink">
      <div className="meeting-quicklink-text">
        <span className="t-name">{t("scheduling.myLink")}</span>
        <Badge tone={profile.enabled ? "success" : "default"}>
          {t(profile.enabled ? "scheduling.active" : "scheduling.paused")}
        </Badge>
        <span className="t-caption meeting-quicklink-url">
          {profile.public_url}
        </span>
      </div>
      <Button variant="primary" onClick={copy.copy}>
        <Copy aria-hidden />
        {copy.label}
      </Button>
      {copy.notice}
    </div>
  );
}

// What still stands between the host and a working booking link, in the order
// the page asks for it; gone once every step is done.
export function SetupChecklist({
  calendar,
  hours,
  link,
}: Readonly<{ calendar: boolean; hours: boolean; link: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const steps: [boolean, MessageKey][] = [
    [calendar, "scheduling.stepCalendar"],
    [hours, "scheduling.stepHours"],
    [link, "scheduling.stepLink"],
  ];
  const done = steps.filter(([ok]) => ok).length;
  if (done === steps.length) return null;
  return (
    <Callout
      tone="discovery"
      title={t("scheduling.setupTitle", {
        done: formatNumber(done, locale),
        total: formatNumber(steps.length, locale),
      })}
    >
      <ol className="meeting-checklist">
        {steps.map(([ok, key]) => (
          <li key={key} data-done={ok}>
            {ok ? <CheckCircle2 aria-hidden /> : <Circle aria-hidden />}
            <span>{t(key)}</span>
            <span className="sr-only">
              {t(ok ? "scheduling.stepDone" : "scheduling.stepOpen")}
            </span>
          </li>
        ))}
      </ol>
    </Callout>
  );
}

export function CalendarSection({
  providers,
  provider,
  calendarID,
  blocking,
  connectionsPending,
  connectionsError,
  missingSaved,
  providerChanged,
  onProvider,
  onCalendar,
  onBlocking,
}: Readonly<{
  providers: readonly { provider: string }[];
  provider: Provider;
  calendarID: string;
  blocking: readonly string[];
  connectionsPending: boolean;
  connectionsError: unknown;
  missingSaved: boolean;
  providerChanged: boolean;
  onProvider: (provider: Provider) => void;
  onCalendar: (id: string) => void;
  onBlocking: (ids: string[]) => void;
}>) {
  const t = useT();
  return (
    <>
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
                if (value === "" || value === "gcal" || value === "graphcal")
                  onProvider(value);
              }}
            />
          )}
        </Field>
      )}
      {!connectionsPending && !connectionsError && providers.length === 0 && (
        <p>{t("scheduling.disconnectedCalendar")}</p>
      )}
      {connectionsPending && <p>{t("common.loading")}</p>}
      <ErrorLine error={connectionsError} />
      {provider ? (
        <BookingCalendars
          provider={provider}
          calendar={calendarID}
          blocking={blocking}
          onCalendar={onCalendar}
          onBlocking={onBlocking}
        />
      ) : (
        <a href="#/settings/connections" target="_blank" rel="noreferrer">
          {t("scheduling.manageConnection")}
        </a>
      )}
      {missingSaved && (
        <ErrorLine>{t("scheduling.missingSavedCalendar")}</ErrorLine>
      )}
      {providerChanged && (
        <p className="t-caption">{t("scheduling.providerChanged")}</p>
      )}
    </>
  );
}

export function LimitFields({
  form,
  noticeHours,
  onNotice,
  onChange,
}: Readonly<{
  form: Profile;
  noticeHours: string;
  onNotice: (hours: string) => void;
  onChange: (form: Profile) => void;
}>) {
  const t = useT();
  return (
    <div className="book-policy-fields">
      <Field label={t("scheduling.notice")} hint={t("scheduling.noticeHelp")}>
        {(control) => (
          <TextInput
            {...control}
            type="number"
            min={0}
            max={168}
            step="any"
            required
            value={noticeHours}
            onChange={(e) => onNotice(e.target.value)}
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
              onChange({ ...form, buffer_minutes: Number(e.target.value) })
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
              onChange({ ...form, horizon_days: Number(e.target.value) })
            }
          />
        )}
      </Field>
    </div>
  );
}

export function videoDefaultLabel(provider: string): MessageKey {
  if (provider === "gcal") return "scheduling.videoDefaultGoogle";
  return provider === "graphcal"
    ? "scheduling.videoDefaultTeams"
    : "scheduling.videoDefaultGeneric";
}

export function DefaultFields({
  form,
  provider,
  onChange,
}: Readonly<{
  form: Profile;
  provider: string;
  onChange: (form: Profile) => void;
}>) {
  const t = useT();
  const plural = usePlural();
  const video = form.video_call ?? true;
  const duration = identifierNumber(form.duration_minutes);
  const lengths = LENGTHS.includes(duration)
    ? LENGTHS
    : [...LENGTHS, duration].sort((a, b) => Number(a) - Number(b));
  return (
    <>
      <Field label={t("scheduling.subject")}>
        {(control) => (
          <TextInput
            {...control}
            required
            value={form.title}
            onChange={(e) => onChange({ ...form, title: e.target.value })}
          />
        )}
      </Field>
      <SegmentedControl
        label={t("scheduling.duration")}
        options={lengths}
        value={duration}
        labels={Object.fromEntries(
          lengths.map((minutes) => [
            minutes,
            plural("scheduling.minutes", Number(minutes), { count: minutes }),
          ]),
        )}
        onChange={(minutes) =>
          onChange({ ...form, duration_minutes: Number(minutes) })
        }
      />
      <Switch
        label={t(videoDefaultLabel(provider))}
        hint={
          provider === "graphcal"
            ? `${t("scheduling.videoDefaultHelp")} ${t("scheduling.videoTeamsHelp")}`
            : t("scheduling.videoDefaultHelp")
        }
        checked={video}
        onChange={(on) => onChange({ ...form, video_call: on })}
      />
      <Field
        label={t("scheduling.location")}
        hint={t(
          video ? "scheduling.locationFallback" : "scheduling.locationHelp",
        )}
      >
        {(control) => (
          <TextInput
            {...control}
            placeholder={t("scheduling.locationExample")}
            value={form.location}
            onChange={(e) => onChange({ ...form, location: e.target.value })}
          />
        )}
      </Field>
      <Switch
        label={t("scheduling.emailReminder")}
        hint={t("scheduling.reminderHelp")}
        checked={form.email_reminder ?? false}
        onChange={(on) => onChange({ ...form, email_reminder: on })}
      />
    </>
  );
}
