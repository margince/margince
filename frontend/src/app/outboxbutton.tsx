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

// The way back to a message a rep scheduled, from anywhere in the app. The
// queue is off the rail on purpose — a queue of one contact's own unsent mail is
// not a destination of its own — so this sits beside the bell instead: a thing
// that is waiting FOR the reader, shown only as loud as it is.
//
// It draws nothing until the queue has answered and nothing when the read fails:
// a clock that cannot say what it holds would only be a second place to be wrong.

export function OutboxButton() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const queue = useScheduledSends();
  // A chrome control must never take the shell down: an answer that is not a
  // list (a client and server on other versions) draws nothing.
  if (!Array.isArray(queue.data)) return null;
  const { waiting, held } = queueCounts(queue.data);
  const total = waiting + held;

  // The counts join the NAME, as the bell's do: `aria-label` replaces the
  // contents for name computation, so a figure left to the badge alone would
  // reach a screen reader from nowhere.
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
            {/* A held message is the one that will not send itself, so it is the
                one that changes the colour. */}
            <Badge variant="primary" tone={held > 0 ? "warning" : "accent"}>
              {formatNumber(total, locale)}
            </Badge>
          </span>
        )}
      </Button>
    </div>
  );
}
