// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Where each class of notice reaches this reader.
//
// It sits under the account's own settings beside the display language, not
// under anything an admin configures for the team: what the product may send
// somebody is the one part of a notification a seat is entitled to decide, and
// an admin muting a colleague's approvals is the shape this refuses. So there is
// no role gating here and no refusal copy — the endpoints read and write the
// calling seat's own rows.
//
// The classes are deliberately coarser than the notice kinds: a reader who
// muted "an automation fired" has not muted "somebody is waiting on your
// approval".

import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, QueryGate } from "./common";
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
 * `coach` is the one class that may not be switched off — the server answers
 * 422 — so the option is not offered. Showing a choice that cannot be made is
 * worse than omitting it: the reader picks it, is refused, and learns nothing
 * about why.
 */
function choicesFor(
  notificationClass: NotificationClass,
): readonly NotificationDelivery[] {
  return notificationClass === "coach"
    ? CHOICES.filter((choice) => choice !== "off")
    : CHOICES;
}

/**
 * The label and the sentence under it, per class.
 *
 * Spelled out rather than composed from the class at the call site, because `t`
 * takes a key from a closed union — a computed key does not compile. Keyed by
 * the contract's own enum, so the day a seventh class is added the regenerated
 * type fails this map rather than letting the page render a row it has no words
 * for.
 */
const CLASS_COPY: Readonly<
  Record<NotificationClass, { label: MessageKey; help: MessageKey }>
> = {
  approval_pending: {
    label: "notifications.class.approval_pending.label",
    help: "notifications.class.approval_pending.help",
  },
  automation: {
    label: "notifications.class.automation.label",
    help: "notifications.class.automation.help",
  },
  lead_sla: {
    label: "notifications.class.lead_sla.label",
    help: "notifications.class.lead_sla.help",
  },
  capture: {
    label: "notifications.class.capture.label",
    help: "notifications.class.capture.help",
  },
  system: {
    label: "notifications.class.system.label",
    help: "notifications.class.system.help",
  },
  coach: {
    label: "notifications.class.coach.label",
    help: "notifications.class.coach.help",
  },
};

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
      <PanelBody className="form-stack">
        <p className="settings-panel-sub">{t("notifications.sub")}</p>
        <QueryGate pendingLabel={t("notifications.title")} query={preferences}>
          {(list) => <DeliveryChoices rows={list.items} />}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

function DeliveryChoices({
  rows,
}: Readonly<{ rows: readonly NotificationPreference[] }>) {
  const t = useT();
  const save = useSaveNotificationPreference();
  return (
    <>
      <SettingList>
        {rows.map((row) => (
          <DeliveryRow
            key={row.class}
            row={row}
            onPick={(delivery) => save.mutate({ class: row.class, delivery })}
            saving={save.isPending}
          />
        ))}
      </SettingList>
      {/* THE CALLOUT NAMES THE ROW IT IS ABOUT. Six rows share one error
          surface, and it sits under all of them, so an unnamed failure leaves
          the reader checking each dropdown to find which one did not take.
          The class comes from the failed write's own variable rather than from
          a render — it is the same value that went on the wire, so the sentence
          cannot name a different row from the one the server refused. */}
      {save.isError && (
        <Callout
          tone="danger"
          kind="outcome"
          title={
            save.variables === undefined
              ? t("notifications.saveFailed")
              : t("notifications.saveFailedFor", {
                  setting: t(CLASS_COPY[save.variables.class].label),
                })
          }
        >
          {problemMessageOf(save.error, t)}
        </Callout>
      )}
    </>
  );
}

function DeliveryRow({
  row,
  onPick,
  saving,
}: Readonly<{
  row: NotificationPreference;
  onPick: (delivery: NotificationDelivery) => void;
  saving: boolean;
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
