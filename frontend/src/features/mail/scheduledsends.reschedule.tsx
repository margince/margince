// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { isoOrBlank } from "../../design-system/dateinput";
import { Popover } from "../../design-system/popover";
import { clockOrBlank } from "../../design-system/timeinput";
import { localDateTimeValue } from "../../format/calendarday";
import { formatDateTime } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useViewerZone } from "../../lib/viewerzone";
import { scheduleFields, schedulePresets } from "./composeschedule";
import { MomentFields, messageZone, useMomentChoice } from "./scheduledmoment";
import type { ScheduledSend } from "./scheduledqueue";

/** The write: the row's identity and version travel with the new moment. */
export type Move = Readonly<{
  id: string;
  version: number;
  at: string;
  /** The zone to store; the reader's own when absent. */
  tz?: string;
  intent?: components["schemas"]["IntentTime"];
}>;

/**
 * Rescheduling in one press: the composer's own three moments, and a picker.
 * The presets come from the composer's function, so "Tomorrow morning" is the
 * same hour in both places.
 *
 * Only the time can change, because the approval is bound to the content.
 * The picker reads back through `scheduleFields`, as the composer does.
 */
export function RescheduleMenu({
  send,
  now,
  pending,
  onMove,
}: Readonly<{
  send: ScheduledSend;
  now: Date;
  pending: boolean;
  onMove: (move: Move) => void;
}>) {
  const readerZone = useViewerZone();
  const t = useT();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const choiceZone = messageZone(send, readerZone);
  // The button refuses half a moment or a past one, which the server would 422.
  const choice = useMomentChoice(choiceZone, now);

  const move = (chosen: ReturnType<typeof scheduleFields>) => {
    if (chosen.scheduled_at === undefined) return;
    onMove({
      id: send.id,
      version: send.version,
      at: chosen.scheduled_at,
      tz: chosen.scheduled_tz,
      intent: chosen.scheduled_intent,
    });
    setOpen(false);
  };

  return (
    <Popover
      dialog
      variant="ghost"
      disabled={pending}
      open={open}
      onOpenChange={(next) => {
        if (next) {
          const [shownDay, shownTime] = (
            send.scheduled_intent?.wall_clock ??
            localDateTimeValue(send.scheduled_at, choiceZone)
          ).split("T");
          choice.setDay(isoOrBlank(shownDay));
          choice.setTime(clockOrBlank(shownTime));
        }
        setOpen(next);
      }}
      panelClassName="scheduledsends-menu"
      label={
        <>
          <span aria-hidden>{t("sched.move")}</span>
          <span className="sr-only">
            {t("sched.moveFor", { subject: send.subject })}
          </span>
        </>
      }
    >
      <div className="scheduledsends-presets">
        {schedulePresets(now, choiceZone).map((preset) => (
          <Button
            key={preset.key}
            variant="ghost"
            onClick={() =>
              move(
                scheduleFields(
                  localDateTimeValue(preset.at.toISOString(), choiceZone),
                  undefined,
                  choiceZone,
                ),
              )
            }
          >
            <span>{t(preset.key)}</span>
            <span className="t-caption">
              {formatDateTime(preset.at.toISOString(), locale, readerZone)}
            </span>
          </Button>
        ))}
      </div>
      <div className="scheduledsends-custom">
        <MomentFields
          choice={choice}
          dayLabel={t("sched.moveToDay", { subject: send.subject })}
          timeLabel={t("sched.moveToTime", { subject: send.subject })}
        />
        <Button
          variant="primary"
          disabled={!choice.ready}
          reason={choice.past ? t("compose.schedulePast") : undefined}
          pending={pending}
          onClick={() => {
            if (choice.ready) move(choice.fields);
          }}
        >
          {t("sched.moveSave")}
        </Button>
      </div>
    </Popover>
  );
}
