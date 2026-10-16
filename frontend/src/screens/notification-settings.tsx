// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Where each class of notice reaches this reader. An admin never mutes a
// colleague's approvals, so there is no role gating: both endpoints read and
// write the calling seat's own rows. The classes are coarser than notice kinds.

import { useId } from "react";
import { describedByIds } from "../design-system/describedby";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { PanelNotices } from "../design-system/panelnotices";
import { QueryGate } from "../design-system/querystates";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
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

function DeliveryChoices({
  rows,
}: Readonly<{ rows: readonly NotificationPreference[] }>) {
  const t = useT();
  const save = useSaveNotificationPreference();
  const refusalId = useId();
  return (
    <>
      <SettingList bleed="settings">
        {inReadingOrder(rows).map((row) => (
          <DeliveryRow
            key={row.class}
            row={row}
            onPick={(delivery) => save.mutate({ class: row.class, delivery })}
            // THIS row's write, not any row's. The mutation is shared so one
            // error surface can name the class that failed, and a bare
            // `isPending` spread that wait across all six — freezing five
            // dropdowns nobody touched, and hiding the queued second change the
            // hook's own scope exists to serialise.
            saving={save.isPending && save.variables?.class === row.class}
            refusedBy={
              save.isError && save.variables?.class === row.class
                ? refusalId
                : undefined
            }
          />
        ))}
      </SettingList>
      {/* The title names the row the server refused: six rows share one
          notice under them. */}
      <PanelNotices
        refused={
          save.isError
            ? {
                title:
                  save.variables === undefined
                    ? t("notifications.saveFailed")
                    : t("notifications.saveFailedFor", {
                        setting: t(CLASS_COPY[save.variables.class].label),
                      }),
                error: save.error,
                id: refusalId,
              }
            : undefined
        }
      />
    </>
  );
}

function DeliveryRow({
  row,
  onPick,
  saving,
  refusedBy,
}: Readonly<{
  row: NotificationPreference;
  onPick: (delivery: NotificationDelivery) => void;
  saving: boolean;
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
          // A row the seat has never decided sits on the STANDING DEFAULT the
          // server reports, not on a blank or an invented "Default" option:
          // what the control shows is what happens today, and touching it is
          // what turns a default into a decision.
          value={row.delivery}
          disabled={saving}
          onChange={(next) => {
            // Narrowed through the same list the options are built from, so
            // nothing is sent that the control was never offering — the coach
            // row can no more send `off` than it can show it.
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
