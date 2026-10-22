// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Mail } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import type { components } from "../../../api/schema";
import { Badge, Button } from "../../../design-system/atoms";
import { Callout } from "../../../design-system/callout";
import { StageActions } from "../../../design-system/onboarding-stage";
import { Panel, PanelBody } from "../../../design-system/panel";
import { ProviderMark } from "../../../design-system/provider-mark";
import { ProviderRow } from "../../../design-system/providerrow";
import { ServiceList, ServiceRow } from "../../../design-system/servicerow";
import { statusTone } from "../../../design-system/statusbadge";
import {
  ACCOUNT_NAME,
  type AccountVendor,
  type ConnectionAccount,
  vendorOf,
} from "../../../format/connectorprovider";
import { useT } from "../../../i18n";
import {
  notConfiguredProvider,
  useConnectAccount,
} from "../../../lib/connect-account";
import type { Service } from "../../../lib/connect-flow-draft";
import { SERVICE_ICON } from "../../../lib/connect-services";
import {
  accountHealth,
  healthBadge,
  needsReconnect,
  serviceBadge,
  serviceReason,
} from "../../../lib/connector-status";
import { problemMessageOf } from "../../../lib/problem";
import { IMPORT_WINDOW_LABELS } from "../../../mail-history/window-picker";
import { ConnectPostureStep } from "../connect-posture";
import {
  type BackreadChoice,
  BackreadPanel,
  offersImport,
  useOnboardingBackread,
} from "../onboarding-backread";
import { markOAuthAttempt } from "../onboarding-connect-panels";
import { connectInput } from "./connect-vendor";

// The connect step once a mailbox is live: what is connected, who may read it,
// how far back to import, and the way on. The footer owns the import's start;
// the account's own primary is a Reconnect, only while a grant is stale.

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

/**
 * The account as one row, the way Connections draws it. It shows its health,
 * why it needs someone, and one primary Reconnect when a grant must be asked
 * again. Its services hang under it. A vendor account without its calendar
 * offers that calendar alone, landing back here.
 */
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
  const name = t(ACCOUNT_NAME[account.key]);
  const health = healthBadge(accountHealth(account));
  const live = liveServices(account);
  const reasonKey =
    live.map(([, row]) => serviceReason(row)).find((key) => key !== null) ??
    null;
  const stale = live
    .filter(([, row]) => needsReconnect(row))
    .map(([service]) => service);
  useEffect(() => {
    onBusyChange(connect.isPending);
  }, [connect.isPending, onBusyChange]);

  return (
    <Panel title={t("ob.conv.connect.connectedTitle", { vendor: name })}>
      <PanelBody>
        <ProviderRow
          mark={vendor ? <ProviderMark providerKey={vendor} /> : <Mail />}
          name={name}
          headingLevel="h3"
          status={{
            tone: health.tone,
            label: t(health.label),
            live: health.live,
          }}
          account={
            account.mail?.account_label ??
            account.calendar?.account_label ??
            undefined
          }
          reason={reasonKey ? t(reasonKey) : undefined}
          action={
            vendor !== null && stale.length > 0 ? (
              <Button
                variant="primary"
                pending={connect.isPending}
                onClick={() => connect.mutate(connectInput(vendor, stale))}
              >
                {t("connectors.reconnect")}
              </Button>
            ) : undefined
          }
        >
          <AccountServiceRows
            account={account}
            vendor={vendor}
            label={t("connectors.svc.list", { account: name })}
            pending={connect.isPending}
            // The calendar leg alone: the mailbox is already live, and the
            // import stays the footer's choice.
            onConnectCalendar={(v) =>
              connect.mutate(connectInput(v, ["calendar"]))
            }
          />
        </ProviderRow>
        <CalendarAskOutcome connect={connect} name={name} />
      </PanelBody>
    </Panel>
  );
}

/** What a calendar ask came back with, when it did not leave the page. */
function CalendarAskOutcome({
  connect,
  name,
}: Readonly<{ connect: ReturnType<typeof useConnectAccount>; name: string }>) {
  const t = useT();
  return (
    <>
      {connect.isError && (
        <Callout
          tone="danger"
          kind="outcome"
          title={t("ob.conv.connect.calendarFailedTitle")}
        >
          {problemMessageOf(connect.error, t)}
        </Callout>
      )}
      {notConfiguredProvider(connect.data) && (
        <Callout
          tone="warning"
          kind="outcome"
          title={t("connectors.providerNotConfigured", { provider: name })}
        />
      )}
    </>
  );
}

/** The account's services that are not disconnected, by what they are. */
function liveServices(
  account: ConnectionAccount,
): (readonly [Service, CaptureConnection])[] {
  const rows: (readonly [Service, CaptureConnection])[] = [];
  if (account.mail && account.mail.status !== "disconnected") {
    rows.push(["mail", account.mail]);
  }
  if (account.calendar && account.calendar.status !== "disconnected") {
    rows.push(["calendar", account.calendar]);
  }
  return rows;
}

/**
 * What the account brings in, one line each. Past email is left out: onboarding
 * asks for it in its own import panel below, where Connections lists it here.
 */
function AccountServiceRows({
  account,
  vendor,
  label,
  pending,
  onConnectCalendar,
}: Readonly<{
  account: ConnectionAccount;
  vendor: AccountVendor | null;
  label: string;
  pending: boolean;
  onConnectCalendar: (vendor: AccountVendor) => void;
}>) {
  const t = useT();
  const { mail, calendar } = account;
  const calendarLive = calendar !== null && calendar.status !== "disconnected";
  return (
    <ServiceList label={label}>
      {mail && (
        <ConnectedService
          row={mail}
          service="mail"
          caption={t("connectors.svc.newMailDesc")}
        />
      )}
      {vendor !== null && calendarLive && (
        <ConnectedService
          row={calendar}
          service="calendar"
          caption={t("connectors.svc.calendarDesc")}
        />
      )}
      {vendor !== null && !calendarLive && (
        <ServiceRow
          icon={SERVICE_ICON.calendar}
          name={t("connectors.svc.calendar")}
          caption={t("ob.conv.connect.calendarNotConnected")}
          status={{
            tone: statusTone("neutral"),
            label: t("connectors.health.notConnected"),
          }}
          action={
            <Button pending={pending} onClick={() => onConnectCalendar(vendor)}>
              {t("ob.conv.connect.connectCalendar")}
            </Button>
          }
        />
      )}
    </ServiceList>
  );
}

function ConnectedService({
  row,
  service,
  caption,
}: Readonly<{ row: CaptureConnection; service: Service; caption: string }>) {
  const t = useT();
  const badge = serviceBadge(row);
  return (
    <ServiceRow
      icon={SERVICE_ICON[service]}
      name={t(
        service === "mail"
          ? "connectors.svc.newMail"
          : "connectors.svc.calendar",
      )}
      caption={caption}
      status={{ tone: badge.tone, label: t(badge.label), live: badge.live }}
    />
  );
}

/**
 * The way on with a mailbox live. While the window pick is on screen the one
 * primary is the import, which starts the run and then leaves. Declining it is
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
