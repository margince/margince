import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { Button, Field, TextInput } from "../design-system/atoms";
import { MeetingSlots } from "../design-system/meetingslots";
import { formatDateTime } from "../format/format";
import { dayInZone, startOfDayInZone, viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { BookingZone } from "./booking-common";
import { QueryGate, throwProblem } from "./common";

export function BookingReschedule({
  id,
  token,
  pending,
  onSelect,
}: Readonly<{
  id?: string;
  token?: string;
  pending: boolean;
  onSelect: (slot: { start: string; end: string }) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [zone, setZone] = useState(viewerZone);
  const [from, setFrom] = useState(() => new Date().toISOString());
  const [selected, setSelected] = useState<{
    start: string;
    end: string;
  } | null>(null);
  const query = useQuery({
    queryKey: ["meeting-alternatives", id, token, from],
    queryFn: async () => {
      const window = {
        from,
        to: new Date(new Date(from).getTime() + 7 * 86400000).toISOString(),
      };
      const result = token
        ? await api.GET("/public/meeting/{token}/availability", {
            params: { path: { token }, query: window },
          })
        : await api.GET("/scheduling/invitations/{id}/availability", {
            params: { path: { id: id ?? "" }, query: window },
          });
      if (result.error) throwProblem(result.error);
      return result.data;
    },
  });
  return (
    <div className="book-form">
      <p>{t("scheduling.rescheduleHelp")}</p>
      <BookingZone value={zone} onChange={setZone} />
      <Field label={t("scheduling.date")}>
        {(control) => (
          <TextInput
            {...control}
            type="date"
            value={dayInZone(new Date(from).getTime(), zone)}
            onChange={(event) => {
              if (event.target.value) {
                setFrom(startOfDayInZone(event.target.value, zone));
                setSelected(null);
              }
            }}
          />
        )}
      </Field>
      <QueryGate pendingLabel={t("common.loading")} query={query}>
        {(value) => (
          <>
            <MeetingSlots
              slots={value.slots.map((slot) => ({
                ...slot,
                label: formatDateTime(slot.start, locale, zone),
              }))}
              selected={selected?.start}
              onSelect={setSelected}
              empty={t("scheduling.noTimes")}
            />
            {value.truncated && (
              <Button
                onClick={() => {
                  const last = value.slots.at(-1);
                  if (last) {
                    setFrom(
                      new Date(
                        new Date(last.start).getTime() + 15 * 60000,
                      ).toISOString(),
                    );
                    setSelected(null);
                  }
                }}
              >
                {t("scheduling.next")}
              </Button>
            )}
          </>
        )}
      </QueryGate>
      <Button
        variant="primary"
        disabled={!selected || pending}
        onClick={() => {
          if (selected) onSelect(selected);
        }}
      >
        {t("scheduling.saveTime")}
      </Button>
    </div>
  );
}
