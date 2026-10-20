// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Clock } from "lucide-react";
import { Badge, Button } from "../design-system/atoms";
import {
  queueCounts,
  SCHEDULED_SCREEN,
  useScheduledSends,
} from "../features/mail/scheduledqueue";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { navigate } from "../lib/router";
import "./outboxbutton.css";

// The way back to a scheduled message from any page. The queue has no sidebar
// row, because one contact's unsent mail is not a destination of its own.
// So it sits beside the bell, as something waiting for the reader.
//
// It draws nothing until the queue answers, and nothing when the read fails.
// A clock that cannot say what it holds would be a second place to be wrong.

export function OutboxButton() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const queue = useScheduledSends();
  // A chrome control must not take the shell down. An answer that is not a
  // list, from a client and server on other versions, draws nothing.
  if (!Array.isArray(queue.data)) return null;
  const { waiting, held } = queueCounts(queue.data);
  const total = waiting + held;

  // The counts join the name, as the bell's do. `aria-label` replaces the
  // contents, so a figure left to the badge would never reach a screen reader.
  const name =
    held > 0
      ? t("outbox.nameHeld", {
          waiting: formatNumber(waiting, locale),
          held: formatNumber(held, locale),
        })
      : waiting > 0
        ? plural("outbox.nameWaiting", waiting, {
            count: formatNumber(waiting, locale),
          })
        : t("outbox.name");

  return (
    <div className="outbox">
      <Button
        variant="bare"
        iconOnly
        aria-label={name}
        title={t("outbox.name")}
        onClick={() => navigate({ screen: SCHEDULED_SCREEN })}
      >
        <Clock aria-hidden />
        {total > 0 && (
          <span className="outbox-count">
            {/* A held message will not send itself, so it changes the colour. */}
            <Badge variant="primary" tone={held > 0 ? "warning" : "accent"}>
              {formatNumber(total, locale)}
            </Badge>
          </span>
        )}
      </Button>
    </div>
  );
}
