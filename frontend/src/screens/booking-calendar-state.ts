import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { throwProblem } from "./common";
import { missingCalendarWriteGrant } from "./connector-status";
import { useConnectors } from "./connectors";

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
