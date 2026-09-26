import { Field } from "../design-system/atoms";
import { MultiSelect, Select } from "../design-system/select";
import { useT } from "../i18n";
import { useBookingCalendar } from "./booking-calendar-state";
import { QueryGate } from "./common";
import { missingCalendarWriteGrant } from "./connector-status";

export function BookingCalendars({
  provider,
  calendar,
  blocking,
  onCalendar,
  onBlocking,
}: Readonly<{
  provider: "" | "gcal" | "graphcal";
  calendar: string;
  blocking: readonly string[];
  onCalendar: (id: string) => void;
  onBlocking: (ids: string[]) => void;
}>) {
  const t = useT();
  const {
    connections,
    connection,
    ready,
    calendars: query,
  } = useBookingCalendar(provider);
  if (provider === "") return null;
  if (connections.isPending) return <p>{t("common.loading")}</p>;
  if (connections.error)
    return (
      <QueryGate query={connections} pendingLabel={t("common.loading")}>
        {() => null}
      </QueryGate>
    );
  if (!ready)
    return (
      <div className="book-form">
        {connection?.account_label && <p>{connection.account_label}</p>}
        <p>
          {t(
            connection?.status === "reauth_required"
              ? "scheduling.expiredCalendar"
              : connection && missingCalendarWriteGrant(connection)
                ? "scheduling.readOnlyCalendar"
                : "scheduling.disconnectedCalendar",
          )}
        </p>
        <a href="#/settings/connections" target="_blank" rel="noreferrer">
          {t("scheduling.manageConnection")}
        </a>
      </div>
    );
  return (
    <div className="book-form">
      <p>
        {t("scheduling.calendarConnected", {
          account:
            connection?.account_label ??
            (provider === "gcal" ? "Google Calendar" : "Microsoft Outlook"),
        })}
      </p>
      <QueryGate query={query} pendingLabel={t("common.loading")}>
        {(calendars) =>
          calendars.some((item) => item.writable) ? (
            <div className="book-form">
              <Field label={t("scheduling.calendar")}>
                {(control) => (
                  <Select
                    {...control}
                    value={
                      calendar === "primary"
                        ? (calendars.find((item) => item.primary)?.id ??
                          calendar)
                        : calendar
                    }
                    options={calendars
                      .filter((item) => item.writable)
                      .map((item) => ({ value: item.id, label: item.name }))}
                    onChange={onCalendar}
                  />
                )}
              </Field>
              <Field label={t("scheduling.blockingCalendars")}>
                {(control) => (
                  <MultiSelect
                    {...control}
                    values={blocking}
                    options={calendars
                      .filter((item) => item.id !== calendar)
                      .map((item) => ({ value: item.id, label: item.name }))}
                    onChange={onBlocking}
                  />
                )}
              </Field>
              <p className="t-caption">{t("scheduling.blockingHelp")}</p>
            </div>
          ) : (
            <p>{t("scheduling.noWritableCalendar")}</p>
          )
        }
      </QueryGate>
      {query.isError && (
        <a href="#/settings/connections" target="_blank" rel="noreferrer">
          {t("scheduling.manageConnection")}
        </a>
      )}
    </div>
  );
}
