// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarDays, Mail } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import type { components } from "../../api/schema";
import { Badge, Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { StageActions } from "../../design-system/onboarding-stage";
import { Panel, PanelBody } from "../../design-system/panel";
import {
  ProviderList,
  ProviderRow,
  type ProviderRowTone,
} from "../../design-system/providerrow";
import { useT } from "../../i18n";
import { IMPORT_WINDOW_LABELS } from "../../mail-history/window-picker";
import { problemMessageOf } from "../common";
import { notConfiguredProvider, useConnectAccount } from "../connect-account";
import { ConnectPostureStep } from "../connect-posture";
import {
  healthBadge,
  missingCalendarWriteGrant,
  serviceHealth,
} from "../connector-status";
import {
  ACCOUNT_NAME,
  type ConnectionAccount,
  VENDOR_SERVICES,
  vendorOf,
} from "../connectorproviders";
import {
  type BackreadChoice,
  BackreadPanel,
  offersImport,
  useOnboardingBackread,
} from "../onboarding-backread";
import { markOAuthAttempt } from "../onboarding-connect-panels";

// The connect step once a mailbox is live: what is connected, who may read it,
// how far back to import, and the way on. The footer owns the import's start,
// so the page carries one primary in every state.

type CaptureConnection = components["schemas"]["CaptureConnection"];

export function ConnectedStep({
  account,
  mailbox,
  also,
  finishing,
  onFinish,
}: Readonly<{
  account: ConnectionAccount;
  /** The account's live mailbox: the row the posture and the import are for. */
  mailbox: CaptureConnection;
  /** The optional extras, drawn between the import and the footer. */
  also: ReactNode;
  finishing: boolean;
  /** Leaves the step with a mailbox connected. */
  onFinish: () => void;
}>) {
  const t = useT();
  // Raised while the posture write is in flight: an import or a finish that
  // begins first runs under whichever of the two answers wins.
  const [postureSaving, setPostureSaving] = useState(false);
  const [calendarBusy, setCalendarBusy] = useState(false);
  const way: WayProps = {
    caption:
      account.calendar?.status === "connected"
        ? t("ob.conv.connect.connectedCaption", {
            page: t("settings.tab.connections"),
          })
        : t("ob.conv.connect.connectedCaptionMailOnly"),
    held: postureSaving || calendarBusy,
    finishing,
    onFinish,
  };

  return (
    <>
      <AccountPanel account={account} onBusyChange={setCalendarBusy} />
      <Panel title={t("connectors.mailPosture.label")}>
        <PanelBody>
          <p className="t-sub">{t("ob.conv.connect.postureIntro")}</p>
          <ConnectPostureStep
            provider={mailbox.provider}
            posture={mailbox.mail_posture ?? undefined}
            onPendingChange={setPostureSaving}
          />
        </PanelBody>
      </Panel>
      {/* Only Google and Microsoft mailboxes can be read backward from a
          date; any other host fills from its standing sync alone. */}
      {vendorOf(mailbox.provider) === null ? (
        <>
          {also}
          <ConnectedWayOnward choice={null} {...way} />
        </>
      ) : (
        <ImportThenWayOn mailbox={mailbox} also={also} way={way} />
      )}
    </>
  );
}

type WayProps = Readonly<{
  caption: string;
  /** A decision about this account is still being written or redirected. */
  held: boolean;
  finishing: boolean;
  onFinish: () => void;
}>;

/** The import panel and the footer that starts it, sharing one choice. */
function ImportThenWayOn({
  mailbox,
  also,
  way,
}: Readonly<{ mailbox: CaptureConnection; also: ReactNode; way: WayProps }>) {
  const t = useT();
  const choice = useOnboardingBackread({
    provider: mailbox.provider,
    initial: mailbox.backfill,
  });
  return (
    <>
      <Panel
        title={t("ob.conv.connect.importTitle")}
        titleAction={<Badge>{t("ob.deck.optional")}</Badge>}
      >
        <PanelBody>
          <p className="t-sub">{t("ob.conv.connect.importIntro")}</p>
          <BackreadPanel choice={choice} />
        </PanelBody>
      </Panel>
      {also}
      <ConnectedWayOnward choice={choice} {...way} />
    </>
  );
}

/** The account and one row per service it brings in. A Google or Microsoft
 *  account without its calendar offers that calendar alone, landing back here. */
function AccountPanel({
  account,
  onBusyChange,
}: Readonly<{
  account: ConnectionAccount;
  onBusyChange: (busy: boolean) => void;
}>) {
  const t = useT();
  const vendor = vendorOf(account.mail?.provider);
  const connect = useConnectAccount({
    onRedirect: (input) => markOAuthAttempt(input.provider),
  });
  const unwired = notConfiguredProvider(connect.data);
  const name = t(ACCOUNT_NAME[account.key]);
  const calendar = account.calendar;
  const calendarHealth =
    calendar === null ? "not_connected" : serviceHealth(calendar);
  // A read-only grant cannot add bookings: Connections and Meetings ask for a
  // reconnect there too, so onboarding never shows it as plain Connected.
  const readOnly = calendar !== null && missingCalendarWriteGrant(calendar);
  const asksCalendar =
    readOnly ||
    calendarHealth === "not_connected" ||
    calendarHealth === "needs_reconnect";
  const calendarStatus: Readonly<{ tone: ProviderRowTone; label: string }> =
    readOnly
      ? { tone: "warning", label: t("scheduling.readOnlyBadge") }
      : {
          tone: healthBadge(calendarHealth).tone,
          label: t(healthBadge(calendarHealth).label),
        };
  let calendarFacts: string | undefined;
  if (readOnly) calendarFacts = t("scheduling.readOnlyCalendar");
  else if (calendarHealth === "not_connected")
    calendarFacts = t("ob.conv.connect.calendarNotConnected");
  useEffect(() => {
    onBusyChange(connect.isPending);
  }, [connect.isPending, onBusyChange]);

  return (
    <Panel
      tone="success"
      title={t("ob.conv.connect.connectedTitle", { vendor: name })}
    >
      <PanelBody>
        <ProviderList>
          {account.mail !== null && (
            <ServiceRow
              icon={<Mail />}
              name={t("ob.conv.connect.serviceMail")}
              row={account.mail}
              facts={t("ob.s4.connectLive")}
            />
          )}
          {vendor !== null &&
            (calendar !== null && !asksCalendar ? (
              <ServiceRow
                icon={<CalendarDays />}
                name={t("ob.conv.connect.serviceCalendar")}
                row={calendar}
                facts={t("ob.conv.connect.calendarBrings")}
              />
            ) : (
              <ProviderRow
                mark={<CalendarDays />}
                name={t("ob.conv.connect.serviceCalendar")}
                status={calendarStatus}
                facts={calendarFacts}
                action={
                  <Button
                    pending={connect.isPending}
                    // The calendar leg alone: the mailbox is already live, and
                    // the import stays the footer's choice.
                    onClick={() =>
                      connect.mutate({
                        provider: VENDOR_SERVICES[vendor].calendar,
                        returnTo: "onboarding",
                      })
                    }
                  >
                    {t(
                      calendarHealth === "not_connected"
                        ? "ob.conv.connect.connectCalendar"
                        : "connectors.reconnect",
                    )}
                  </Button>
                }
              />
            ))}
        </ProviderList>
        {connect.isError && (
          <Callout
            tone="warning"
            kind="outcome"
            title={t("ob.conv.connect.calendarFailedTitle")}
          >
            {problemMessageOf(connect.error, t)}
          </Callout>
        )}
        {unwired && (
          <Callout
            tone="warning"
            kind="outcome"
            title={t("connectors.providerNotConfigured", { provider: name })}
          />
        )}
      </PanelBody>
    </Panel>
  );
}

function ServiceRow({
  icon,
  name,
  row,
  facts,
}: Readonly<{
  icon: ReactNode;
  name: string;
  row: CaptureConnection;
  facts: string;
}>) {
  const t = useT();
  const badge = healthBadge(serviceHealth(row));
  return (
    <ProviderRow
      mark={icon}
      name={name}
      status={{ tone: badge.tone, label: t(badge.label) }}
      account={row.account_label ?? undefined}
      facts={facts}
    />
  );
}

/**
 * The way on with a mailbox live. While the window pick is on screen the one
 * primary is the import, which starts the run and then leaves; declining it is
 * the quiet way. A run already started, or no import to offer, leaves Continue.
 */
export function ConnectedWayOnward({
  choice,
  caption,
  held,
  finishing,
  onFinish,
}: WayProps & Readonly<{ choice: BackreadChoice | null }>) {
  const t = useT();
  if (choice === null || !offersImport(choice)) {
    return (
      <StageActions>
        <span className="t-caption">{caption}</span>
        <Button
          variant="primary"
          pending={finishing}
          disabled={held}
          onClick={onFinish}
        >
          {t("ob.conv.connect.continue")}
        </Button>
      </StageActions>
    );
  }
  return (
    <StageActions>
      <span className="t-caption">{caption}</span>
      <Button
        variant="ghost"
        disabled={held || choice.start.isPending || finishing}
        onClick={onFinish}
      >
        {t("ob.conv.connect.continueNoImport")}
      </Button>
      <Button
        variant="primary"
        pending={choice.start.isPending || finishing}
        disabled={held}
        onClick={() => choice.begin(choice.window, onFinish)}
      >
        {t("ob.conv.connect.importAndContinue", {
          period: t(IMPORT_WINDOW_LABELS[choice.window]),
        })}
      </Button>
    </StageActions>
  );
}
