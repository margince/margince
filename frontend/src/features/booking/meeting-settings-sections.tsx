import { Circle, CircleCheck } from "lucide-react";
import type { components } from "../../api/schema";
import { Field, SegmentedControl, TextInput } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ErrorLine } from "../../design-system/errorline";
import { Select } from "../../design-system/select";
import { Switch } from "../../design-system/switch";
import { providerName } from "../../format/connectorprovider";
import { formatNumber, identifierNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { BookingCalendars } from "./booking-calendars";

type Profile = components["schemas"]["SchedulingProfile"];
type Provider = "" | "gcal" | "graphcal";

const LENGTHS = ["15", "30", "45", "60"];

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
            {ok ? <CircleCheck aria-hidden /> : <Circle aria-hidden />}
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
                  label: providerName(item.provider, t),
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
        <p>
          {!connectionsPending &&
            !connectionsError &&
            providers.length === 0 && <>{t("scheduling.noCalendar")} </>}
          {/* A new tab, so the draft here survives; this page reloads the
              connections when the reader comes back. */}
          <a href="#/settings/connections" target="_blank" rel="noreferrer">
            {t("scheduling.manageConnection")}
          </a>
        </p>
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

// The three limit fields as typed. A number kept only as a number cannot show a
// cleared input, and an emptied one would read as 0. So each is held as text,
// and the form takes the number only once there is one.
export type LimitTexts = { notice: string; buffer: string; horizon: string };

export function limitTextsOf(profile: Profile): LimitTexts {
  return {
    notice: String(Number((profile.notice_minutes / 60).toFixed(2))),
    buffer: String(profile.buffer_minutes),
    horizon: String(profile.horizon_days),
  };
}

export const limitIncomplete = (texts: LimitTexts) =>
  Object.values(texts).some((text) => text.trim() === "");

export function LimitFields({
  texts,
  onText,
}: Readonly<{
  texts: LimitTexts;
  onText: (field: keyof LimitTexts, text: string) => void;
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
            value={texts.notice}
            onChange={(e) => onText("notice", e.target.value)}
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
            required
            value={texts.buffer}
            onChange={(e) => onText("buffer", e.target.value)}
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
            required
            value={texts.horizon}
            onChange={(e) => onText("horizon", e.target.value)}
          />
        )}
      </Field>
    </div>
  );
}

// The limits crm.yaml gives SchedulingProfile.title and .location.
const TITLE_MAX = 200;
const LOCATION_MAX = 1000;

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
            maxLength={TITLE_MAX}
            value={form.title}
            onChange={(e) => onChange({ ...form, title: e.target.value })}
          />
        )}
      </Field>
      <div className="field meeting-length">
        <span className="t-label" aria-hidden>
          {t("scheduling.duration")}
        </span>
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
      </div>
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
            maxLength={LOCATION_MAX}
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
