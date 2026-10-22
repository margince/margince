// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Where each class of notice reaches this reader. An admin never mutes a
// colleague's approvals, so there is no role gating: both endpoints read and
// write the calling seat's own rows. The classes are coarser than notice kinds.

import { useId, useState } from "react";
import { describedByIds } from "../../design-system/describedby";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { QueryGate } from "../../design-system/querystates";
import { useSaveDraft } from "../../design-system/savepage";
import { Select } from "../../design-system/select";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  type NotificationClass,
  type NotificationDelivery,
  type NotificationPreference,
  useNotificationPreferences,
  useSaveNotificationPreference,
} from "./notifications.queries";

/** The four deliveries, in the order the dropdown offers them. */
const CHOICES = ["off", "in_app", "email", "digest"] as const;

/**
 * What this class may be set to.
 *
 * Two classes narrow the four, and both mirror a refusal the server answers 422
 * for. Showing a choice that cannot be made is worse than omitting it: the
 * reader picks it, is refused, and learns nothing about why.
 *
 * `coach` may not be switched off — a colleague's words are not the product's
 * housekeeping to drop.
 *
 * Only `approval_pending` may be set to `email`. It is the one class with a
 * sending leg: its notice stages a message in the transaction that writes it,
 * and no other producer stages one at all. Offering the word elsewhere would
 * promise mail that was never going to arrive, which is worse than not offering
 * it — the reader stops watching the screen. `digest` reaches every class, so
 * mail is still on the table for all six.
 */
function choicesFor(
  notificationClass: NotificationClass,
): readonly NotificationDelivery[] {
  return CHOICES.filter(
    (choice) =>
      !(choice === "off" && notificationClass === "coach") &&
      !(choice === "email" && notificationClass !== "approval_pending"),
  );
}

/**
 * The label and the sentence under it, per class, in reading order: what waits
 * on the reader first, the product's housekeeping last. Keyed by the contract's
 * enum, so a new class fails this map rather than rendering a row with no words.
 */
const CLASS_COPY: Readonly<
  Record<NotificationClass, { label: MessageKey; help: MessageKey }>
> = {
  approval_pending: {
    label: "notifications.class.approval_pending.label",
    help: "notifications.class.approval_pending.help",
  },
  coach: {
    label: "notifications.class.coach.label",
    help: "notifications.class.coach.help",
  },
  lead_sla: {
    label: "notifications.class.lead_sla.label",
    help: "notifications.class.lead_sla.help",
  },
  automation: {
    label: "notifications.class.automation.label",
    help: "notifications.class.automation.help",
  },
  capture: {
    label: "notifications.class.capture.label",
    help: "notifications.class.capture.help",
  },
  system: {
    label: "notifications.class.system.label",
    help: "notifications.class.system.help",
  },
};

const CLASS_ORDER = Object.keys(CLASS_COPY);

function inReadingOrder(
  rows: readonly NotificationPreference[],
): NotificationPreference[] {
  return [...rows].sort(
    (a, b) => CLASS_ORDER.indexOf(a.class) - CLASS_ORDER.indexOf(b.class),
  );
}

/** What each delivery is called, spelled out for the same reason. */
const DELIVERY_COPY: Readonly<Record<NotificationDelivery, MessageKey>> = {
  off: "notifications.delivery.off",
  in_app: "notifications.delivery.in_app",
  email: "notifications.delivery.email",
  digest: "notifications.delivery.digest",
};

export function NotificationSettingsCard() {
  const t = useT();
  const preferences = useNotificationPreferences();

  return (
    <Panel title={t("notifications.title")}>
      <PanelBody>
        <PanelIntro>{t("notifications.sub")}</PanelIntro>
      </PanelBody>
      <QueryGate pendingLabel={t("notifications.title")} query={preferences}>
        {(list) => <DeliveryChoices rows={list.items} />}
      </QueryGate>
    </Panel>
  );
}

type Picks = Partial<Record<NotificationClass, NotificationDelivery>>;

function DeliveryChoices({
  rows,
}: Readonly<{ rows: readonly NotificationPreference[] }>) {
  const t = useT();
  const save = useSaveNotificationPreference();
  const [picks, setPicks] = useState<Picks>({});
  // The class the server refused: its row names the notice, and the bar still
  // carries the one summary error.
  const [refused, setRefused] = useState<{
    class: NotificationClass;
    error: unknown;
  } | null>(null);
  const refusalId = useId();
  const changed = rows.filter(
    (row) =>
      picks[row.class] !== undefined && picks[row.class] !== row.delivery,
  );
  useSaveDraft({
    dirty: changed.length > 0,
    // One class per write, in turn, because the endpoint takes one. A refusal
    // stops the rest, so the bar's error belongs to the row still drafted.
    save: async () => {
      setRefused(null);
      for (const row of changed) {
        const delivery = picks[row.class];
        if (delivery === undefined) continue;
        try {
          await save.mutateAsync({ class: row.class, delivery });
        } catch (error) {
          setRefused({ class: row.class, error });
          throw error;
        }
        setPicks((held) => {
          const rest = { ...held };
          delete rest[row.class];
          return rest;
        });
      }
    },
    discard: () => {
      save.reset();
      setRefused(null);
      setPicks({});
    },
  });
  return (
    <>
      <SettingList bleed="settings">
        {inReadingOrder(rows).map((row) => (
          <DeliveryRow
            key={row.class}
            row={row}
            delivery={picks[row.class] ?? row.delivery}
            onPick={(delivery) => {
              setRefused(null);
              setPicks((held) => ({ ...held, [row.class]: delivery }));
            }}
            refusedBy={refused?.class === row.class ? refusalId : undefined}
          />
        ))}
      </SettingList>
      {/* The title names the row the server refused: six rows share one
          notice under them. */}
      <PanelNotices
        refused={
          refused === undefined || refused === null
            ? undefined
            : {
                title: t("notifications.saveFailedFor", {
                  setting: t(CLASS_COPY[refused.class].label),
                }),
                error: refused.error,
                id: refusalId,
              }
        }
      />
    </>
  );
}

function DeliveryRow({
  row,
  delivery,
  onPick,
  refusedBy,
}: Readonly<{
  row: NotificationPreference;
  /** The drafted choice, or the stored one while the row is untouched. */
  delivery: NotificationDelivery;
  onPick: (delivery: NotificationDelivery) => void;
  refusedBy?: string;
}>) {
  const t = useT();
  const offered = choicesFor(row.class);
  return (
    <SettingRow
      label={t(CLASS_COPY[row.class].label)}
      description={t(CLASS_COPY[row.class].help)}
      testId={`notification-delivery-${row.class}`}
      control={(control) => (
        <Select
          {...control}
          aria-describedby={describedByIds(
            control["aria-describedby"],
            refusedBy,
          )}
          className="settingrow-measure"
          // A row the seat has never decided sits on the standing default the server
          // reports, not on a blank or an invented "Default" option. The control shows
          // what happens today, and saving a pick turns a default into a decision.
          value={delivery}
          onChange={(next) => {
            // Narrowed through the same list the options are built from, so nothing is
            // drafted that the control was never offering. The coach row can no more send
            // `off` than it can show it.
            const picked = offered.find((choice) => choice === next);
            if (picked) {
              onPick(picked);
            }
          }}
          options={offered.map((choice) => ({
            value: choice,
            label: t(DELIVERY_COPY[choice]),
          }))}
        />
      )}
    />
  );
}
