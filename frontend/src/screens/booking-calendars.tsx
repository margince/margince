import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import { Field } from "../design-system/atoms";
import { MultiSelect, Select } from "../design-system/select";
import { useT } from "../i18n";
import { QueryGate, throwProblem } from "./common";

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
  const query = useQuery({
    queryKey: ["scheduling-calendars", provider],
    enabled: provider !== "",
    queryFn: async () => {
      if (provider === "") return [];
      const { data, error } = await api.GET("/scheduling/calendars", {
        params: { query: { provider } },
      });
      if (error) throwProblem(error);
      return data;
    },
  });
  if (provider === "") return null;
  return (
    <QueryGate query={query} pendingLabel={t("common.loading")}>
      {(calendars) => (
        <div className="book-form">
          <Field label={t("scheduling.calendar")}>
            {(control) => (
              <Select
                {...control}
                value={
                  calendar === "primary"
                    ? (calendars.find((item) => item.primary)?.id ?? calendar)
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
      )}
    </QueryGate>
  );
}
