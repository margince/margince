import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Field } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { useT } from "../i18n";
import { useBookingCalendar } from "./booking-calendar-state";
import { BookingCalendars } from "./booking-calendars";
import { throwProblem } from "./common";
import { useConnectors } from "./connectors";

type Profile = components["schemas"]["SchedulingProfile"];
export function BookingSetup({ profile }: Readonly<{ profile: Profile }>) {
  const t = useT();
  const client = useQueryClient();
  const [chosen, setChosen] = useState<Profile["provider"] | null>(
    profile.provider || null,
  );
  const connections = useConnectors();
  const calendars = connections.data?.data.filter(
    (item) =>
      (item.provider === "gcal" || item.provider === "graphcal") &&
      item.status !== "disconnected",
  );
  const suggested = calendars?.length === 1 ? calendars[0].provider : "";
  const provider =
    chosen ??
    (suggested === "gcal" || suggested === "graphcal" ? suggested : "");
  const { ready, calendars: choices } = useBookingCalendar(provider);
  const [calendar, setCalendar] = useState(profile.calendar_id);
  const [blocking, setBlocking] = useState(profile.blocking_calendars ?? []);
  const selected = choices.data?.find(
    (item) => item.id === calendar || (calendar === "primary" && item.primary),
  );
  const save = useMutation({
    mutationFn: async (
      selection: Pick<
        Profile,
        "provider" | "calendar_id" | "blocking_calendars"
      >,
    ) => {
      const latest = await api.GET("/scheduling/profile");
      if (latest.error) throwProblem(latest.error);
      const { data, error } = await api.PUT("/scheduling/profile", {
        body: { ...latest.data, ...selection },
      });
      if (error) throwProblem(error);
      return data;
    },
    onSuccess: async (value) => {
      client.setQueryData(["scheduling-profile"], value);
      await client.invalidateQueries({ queryKey: ["reliable-availability"] });
    },
  });
  return (
    <Panel title={t("scheduling.setupCalendar")}>
      <PanelBody>
        <div className="book-form">
          <p>{t("scheduling.selectCalendarHelp")}</p>
          <Field label={t("scheduling.provider")}>
            {(control) => (
              <Select
                {...control}
                value={provider}
                options={[
                  { value: "", label: t("scheduling.chooseProvider") },
                  { value: "gcal", label: "Google Calendar" },
                  { value: "graphcal", label: "Microsoft Outlook" },
                ]}
                onChange={(value) => {
                  if (
                    value === "" ||
                    value === "gcal" ||
                    value === "graphcal"
                  ) {
                    setChosen(value);
                    setCalendar("primary");
                    setBlocking([]);
                  }
                }}
              />
            )}
          </Field>
          <BookingCalendars
            provider={provider}
            calendar={calendar}
            blocking={blocking}
            onCalendar={setCalendar}
            onBlocking={setBlocking}
          />
          <Button
            variant="primary"
            disabled={!ready || !selected?.writable || choices.isFetching}
            pending={save.isPending}
            onClick={() =>
              save.mutate({
                provider,
                calendar_id: selected?.id ?? calendar,
                blocking_calendars: blocking,
              })
            }
          >
            {t("scheduling.useCalendar")}
          </Button>
          <ErrorLine error={save.error} />
        </div>
      </PanelBody>
    </Panel>
  );
}
