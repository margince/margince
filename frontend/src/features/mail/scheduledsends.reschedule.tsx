// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { Button, TextInput } from "../../design-system/atoms";
import { Popover } from "../../design-system/popover";
import { localDateTimeValue } from "../../format/calendarday";
import { formatDateTime } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { schedulePresets } from "./composeschedule";
import { scheduleFields } from "./composesend";
import type { ScheduledSend } from "./scheduledqueue";

/** The write: the row's identity and version travel with the new moment. */
export type Move = Readonly<{ id: string; version: number; at: string }>;

/**
 * Rescheduling in one press: the composer's own three moments, from the same
 * function so "Tomorrow morning" means the same hour in both places, and a
 * picker for anything else.
 *
 * Time only, which is the contract's own rule. The approval is bound to the
 * content, so changing it is cancel-and-recompose. The picker is seeded from
 * the send's moment in the reader's zone and read back through
 * `scheduleFields`, as the composer's send-later control does, so the two agree
 * on what a picked moment means.
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
  const t = useT();
  const { locale } = useLocale();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState("");
  // Empty or unparseable yields neither field, and the wire requires both. The
  // button refuses rather than sending half a schedule the server would 422.
  const fields = scheduleFields(draft);

  const move = (at: string) => {
    onMove({ id: send.id, version: send.version, at });
    setOpen(false);
  };

  return (
    <Popover
      dialog
      variant="ghost"
      disabled={pending}
      open={open}
      onOpenChange={(next) => {
        if (next) setDraft(localDateTimeValue(send.scheduled_at));
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
        {schedulePresets(now).map((preset) => (
          <Button
            key={preset.key}
            variant="ghost"
            onClick={() => move(preset.at.toISOString())}
          >
            <span>{t(preset.key)}</span>
            <span className="t-caption">
              {formatDateTime(preset.at.toISOString(), locale, viewerZone())}
            </span>
          </Button>
        ))}
      </div>
      <div className="scheduledsends-custom">
        <TextInput
          type="datetime-local"
          aria-label={t("sched.moveTo", { subject: send.subject })}
          value={draft}
          onChange={(event) => setDraft(event.target.value)}
        />
        <Button
          variant="primary"
          disabled={fields.scheduled_at === undefined}
          pending={pending}
          onClick={() => {
            if (fields.scheduled_at !== undefined) move(fields.scheduled_at);
          }}
        >
          {t("sched.moveSave")}
        </Button>
      </div>
    </Popover>
  );
}
