import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { navigate } from "../app/router";
import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { throwProblem } from "./common";
import { useConnectAccount } from "./connect-account";
import { missingCalendarWriteGrant } from "./connector-status";
import { VENDOR_SERVICES } from "./connectorproviders";
import { useConnectors } from "./connectors";

type Profile = components["schemas"]["SchedulingProfile"];
type CaptureConnection = components["schemas"]["CaptureConnection"];

export function useBookingCalendar(provider: string, loadCalendars = true) {
  const connections = useConnectors({ refetchOnWindowFocus: "always" });
  const connection = connections.data?.data.find(
    (item) => item.provider === provider,
  );
  const ready =
    !!connection &&
    (connection.status === "connected" || connection.status === "error") &&
    !missingCalendarWriteGrant(connection);
  const calendars = useQuery({
    queryKey: ["scheduling-calendars", provider],
    enabled:
      loadCalendars &&
      ready &&
      (provider === "gcal" || provider === "graphcal"),
    refetchOnWindowFocus: "always",
    queryFn: async () => {
      if (provider !== "gcal" && provider !== "graphcal") return [];
      const { data, error } = await api.GET("/scheduling/calendars", {
        params: { query: { provider } },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  return { connections, connection, ready, calendars };
}

export type CalendarProvider = "gcal" | "graphcal";

const CALENDAR_NAME: Readonly<Record<CalendarProvider, MessageKey>> = {
  gcal: "connectors.provGcal",
  graphcal: "connectors.provGraphCal",
};

export function calendarName(t: Translator, provider: string): string {
  return provider === "gcal" || provider === "graphcal"
    ? t(CALENDAR_NAME[provider])
    : "";
}

function liveCalendars(connections: readonly CaptureConnection[]) {
  return connections.filter(
    (item) =>
      (item.provider === "gcal" || item.provider === "graphcal") &&
      item.status !== "disconnected",
  );
}

/**
 * Where bookings stand with the calendar they depend on. `unsaved` is a
 * calendar connected but not the one the profile names; `unknown` is a roster
 * that could not be read, which claims nothing either way.
 */
export type CalendarStanding =
  | "loading"
  | "unknown"
  | "none"
  | "unsaved"
  | "ready"
  | "expired"
  | "readOnly";

export function useCalendarStanding(profile: Profile) {
  const { connections, connection, ready } = useBookingCalendar(
    profile.provider,
    false,
  );
  const live = liveCalendars(connections.data?.data ?? []);
  let standing: CalendarStanding = "unsaved";
  if (connections.isPending) standing = "loading";
  else if (connections.isError) standing = "unknown";
  else if (live.length === 0) standing = "none";
  else if (ready) standing = "ready";
  else if (connection?.status === "reauth_required") standing = "expired";
  else if (
    connection &&
    connection.status !== "disconnected" &&
    missingCalendarWriteGrant(connection)
  )
    standing = "readOnly";
  return { standing, connection };
}

/** Bookings are on and the calendar behind them cannot take them. */
export function needsAttention(profile: Profile, standing: CalendarStanding) {
  return (
    profile.enabled &&
    standing !== "ready" &&
    standing !== "loading" &&
    standing !== "unknown"
  );
}

type ConnectorsData = ReturnType<typeof useConnectors>["data"];

/**
 * The calendar a single Connect press can ask for: the one beside a vendor the
 * member already uses, else the one vendor this installation can connect.
 * Null means the choice belongs on Connections.
 */
export function calendarToConnect(
  data: ConnectorsData,
): CalendarProvider | null {
  if (!data) return null;
  const vendors = ["google", "microsoft"] as const;
  const live = new Set(
    data.data
      .filter((item) => item.status !== "disconnected")
      .map((item) => item.provider),
  );
  const used = vendors.filter(
    (vendor) =>
      live.has(VENDOR_SERVICES[vendor].mail) ||
      live.has(VENDOR_SERVICES[vendor].calendar),
  );
  if (used.length === 1) return VENDOR_SERVICES[used[0]].calendar;
  if (used.length > 1) return null;
  const ready = vendors.filter((vendor) =>
    data.providers.some(
      (row) =>
        row.provider === VENDOR_SERVICES[vendor].mail && row.reason === "ready",
    ),
  );
  return ready.length === 1 ? VENDOR_SERVICES[ready[0]].calendar : null;
}

/** Connect or reconnect a calendar from Meetings, landing back on Meetings. */
export function useCalendarConnect() {
  const connect = useConnectAccount();
  const connectors = useConnectors();
  return {
    connect,
    target: calendarToConnect(connectors.data),
    start: (provider: CalendarProvider | null) => {
      if (provider === null) {
        navigate({ screen: "settings", id: "connections" });
        return;
      }
      connect.mutate({ provider, returnTo: "meetings" });
    },
  };
}

export function useMeetingCalendar(form: Profile, baseline: Profile) {
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
