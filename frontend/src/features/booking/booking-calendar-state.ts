import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { unwrap } from "../../lib/problem";
import { missingCalendarWriteGrant } from "../../screens/connector-status";
import { useConnectors } from "../../screens/connectors";

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
      return unwrap(
        await api.GET("/scheduling/calendars", {
          params: { query: { provider } },
        }),
      );
    },
  });
  return { connections, connection, ready, calendars };
}
