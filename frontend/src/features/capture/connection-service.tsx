// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ArrowRight, History } from "lucide-react";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { ServiceList, ServiceRow } from "../../design-system/servicerow";
import { statusTone } from "../../design-system/statusbadge";
import {
  type AccountVendor,
  type ConnectionAccount,
  VENDOR_SERVICES,
} from "../../format/connectorprovider";
import { formatDateTime } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import type { ImportWindow } from "../../lib/connect-account";
import { SERVICE_ICON, SERVICE_WORDS } from "../../lib/connect-services";
import {
  missingCalendarWriteGrant,
  serviceBadge,
} from "../../lib/connector-status";
import { navigate } from "../../lib/router";
import { settingsHref } from "../../lib/settingsrouting";
import { usePastEmail } from "./connection-past-email";

// One thing an account brings in (new mail, past email, the calendar) as a line
// under the account's row. It says what it is and how it is doing. A broken
// service says so with its badge. The why, and the Reconnect, are the
// account's.

type CaptureConnection = components["schemas"]["CaptureConnection"];
type Provider = CaptureConnection["provider"];

/**
 * The services an account brings in, one line each. A Google or Microsoft 365
 * account has new mail, past email and the calendar. Any other has new mail
 * alone. A vendor account names the service it is missing, with its connect.
 */
export function AccountServices({
  account,
  vendor,
  label,
  heldWindow,
  connectPendingFor,
  onConnect,
  onImport,
}: Readonly<{
  account: ConnectionAccount;
  vendor: AccountVendor | null;
  /** The list's accessible name. */
  label: string;
  heldWindow: ImportWindow | null;
  /** The provider whose consent is on its way, if any. */
  connectPendingFor: Provider | undefined;
  /** Asks for a missing service's consent for the first time. */
  onConnect: (provider: Provider) => void;
  onImport: () => void;
}>) {
  const t = useT();
  const { mail, calendar } = account;
  const mailProvider = vendor ? VENDOR_SERVICES[vendor].mail : null;
  const calendarProvider = vendor ? VENDOR_SERVICES[vendor].calendar : null;
  return (
    <ServiceList label={label}>
      {mail && <ConnectedService conn={mail} kind="mail" />}
      {!mail && mailProvider && (
        <MissingService
          kind="mail"
          connectLabel={t("connectors.connectMail")}
          pending={connectPendingFor === mailProvider}
          onConnect={() => onConnect(mailProvider)}
        />
      )}
      {vendor && mail?.can_backfill && (
        <PastEmailService
          mail={mail}
          heldWindow={heldWindow}
          onImport={onImport}
        />
      )}
      {calendar && vendor && (
        <ConnectedService conn={calendar} kind="calendar" />
      )}
      {!calendar && calendarProvider && (
        <MissingService
          kind="calendar"
          connectLabel={t("connectors.connectCalendar")}
          pending={connectPendingFor === calendarProvider}
          onConnect={() => onConnect(calendarProvider)}
        />
      )}
    </ServiceList>
  );
}

type ServiceKind = "mail" | "calendar";

function useServiceWords(kind: ServiceKind) {
  const t = useT();
  return {
    name: t(SERVICE_WORDS[kind].name),
    desc: t(SERVICE_WORDS[kind].desc),
  };
}

function PastEmailService({
  mail,
  heldWindow,
  onImport,
}: Readonly<{
  mail: CaptureConnection;
  heldWindow: ImportWindow | null;
  onImport: () => void;
}>) {
  const t = useT();
  const { name, status, action, body } = usePastEmail({
    mail,
    heldWindow,
    onImport,
  });
  return (
    <ServiceRow
      testId={`connector-${mail.provider}-past-email`}
      icon={History}
      name={name}
      caption={t("connectors.svc.pastEmailDesc")}
      status={status}
      action={action}
    >
      {body}
    </ServiceRow>
  );
}

function LastSynced({ conn }: Readonly<{ conn: CaptureConnection }>) {
  const t = useT();
  const { locale } = useLocale();
  if (conn.last_success_at === undefined) {
    return t("connectors.syncTimeUnknown");
  }
  return conn.last_success_at
    ? t("connectors.lastSynced", {
        at: formatDateTime(conn.last_success_at, locale, viewerZone()),
      })
    : t("connectors.neverSynced");
}

function MissingService({
  kind,
  connectLabel,
  pending,
  onConnect,
}: Readonly<{
  kind: ServiceKind;
  connectLabel: string;
  pending: boolean;
  onConnect: () => void;
}>) {
  const t = useT();
  const words = useServiceWords(kind);
  return (
    <ServiceRow
      testId={`connector-${kind}-missing`}
      icon={SERVICE_ICON[kind]}
      name={words.name}
      caption={words.desc}
      status={{
        tone: statusTone("neutral"),
        label: t("connectors.health.notConnected"),
      }}
      action={
        <Button pending={pending} onClick={onConnect}>
          {connectLabel}
        </Button>
      }
    />
  );
}

// A connected service and its one badge. A calendar that can book links
// onward to Meetings.
function ConnectedService({
  conn,
  kind,
}: Readonly<{ conn: CaptureConnection; kind: ServiceKind }>) {
  const t = useT();
  const words = useServiceWords(kind);
  const badge = serviceBadge(conn);
  const books =
    kind === "calendar" &&
    conn.status === "connected" &&
    !missingCalendarWriteGrant(conn);
  return (
    <ServiceRow
      testId={`connector-${conn.provider}`}
      icon={SERVICE_ICON[kind]}
      name={words.name}
      caption={
        <>
          {words.desc} <LastSynced conn={conn} />
        </>
      }
      status={{ tone: badge.tone, label: t(badge.label), live: badge.live }}
    >
      {books && (
        <Button
          variant="link"
          onClick={() => navigate(settingsHref("meetings"))}
        >
          {t("connectors.svc.bookingLink")}
          <ArrowRight aria-hidden="true" />
        </Button>
      )}
    </ServiceRow>
  );
}
