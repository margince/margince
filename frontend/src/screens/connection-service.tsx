// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  ArrowRight,
  CalendarDays,
  History,
  type LucideIcon,
  Mail,
} from "lucide-react";
import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { routeHash } from "../app/router";
import { Badge, Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { ImportWindow } from "./connect-account";
import { usePastEmail } from "./connection-past-email";
import {
  errorClassKey,
  healthBadge,
  missingCalendarWriteGrant,
  missingSendGrant,
  serviceHealth,
} from "./connector-status";
import {
  type AccountVendor,
  type ConnectionAccount,
  VENDOR_SERVICES,
} from "./connectorproviders";
import { settingsHref } from "./settingsrouting";

// One thing an account brings in (new mail, past email, the calendar) as a
// line under the account's row: what it is, how it is doing, and its verb.

type CaptureConnection = components["schemas"]["CaptureConnection"];
type Provider = CaptureConnection["provider"];

/**
 * The services an account brings in, one line each: new mail, past email and
 * the calendar for a Google or Microsoft 365 account, new mail alone for any
 * other. A vendor account names the service it is missing, with its connect.
 */
export function AccountServices({
  account,
  vendor,
  label,
  heldWindow,
  connectPendingFor,
  onReconnect,
  onImport,
}: Readonly<{
  account: ConnectionAccount;
  vendor: AccountVendor | null;
  /** The list's accessible name. */
  label: string;
  heldWindow: ImportWindow | null;
  /** The provider whose consent is on its way, if any. */
  connectPendingFor: Provider | undefined;
  /** Asks for one provider's consent again, or for the first time. */
  onReconnect: (provider: Provider) => void;
  onImport: () => void;
}>) {
  const { mail, calendar } = account;
  return (
    <ServiceList label={label}>
      {(mail || vendor) && (
        <NewMailService
          mail={mail}
          pending={mail !== null && connectPendingFor === mail.provider}
          onReconnect={onReconnect}
          onConnect={() => vendor && onReconnect(VENDOR_SERVICES[vendor].mail)}
        />
      )}
      {vendor && mail && (
        <PastEmailItem
          mail={mail}
          heldWindow={heldWindow}
          onImport={onImport}
        />
      )}
      {vendor && (
        <CalendarService
          calendar={calendar}
          pending={connectPendingFor === VENDOR_SERVICES[vendor].calendar}
          onConnect={() => onReconnect(VENDOR_SERVICES[vendor].calendar)}
        />
      )}
    </ServiceList>
  );
}

function PastEmailItem({
  mail,
  heldWindow,
  onImport,
}: Readonly<{
  mail: CaptureConnection;
  heldWindow: ImportWindow | null;
  onImport: () => void;
}>) {
  const t = useT();
  const { name, side, body } = usePastEmail({ mail, heldWindow, onImport });
  return (
    <ServiceItem
      testId={`connector-${mail.provider}-past-email`}
      icon={History}
      name={name}
      description={t("connectors.svc.pastEmailDesc")}
      side={side}
    >
      {body}
    </ServiceItem>
  );
}

function ServiceList({
  label,
  children,
}: Readonly<{ label: string; children: ReactNode }>) {
  return (
    <ul className="connection-services" aria-label={label}>
      {children}
    </ul>
  );
}

function ServiceItem({
  testId,
  icon: Icon,
  name,
  description,
  side,
  children,
}: Readonly<{
  testId: string;
  icon: LucideIcon;
  name: string;
  description: ReactNode;
  /** The service's status badges and its own verbs. */
  side?: ReactNode;
  /** Whatever the service reports beyond its sentence: progress, an error. */
  children?: ReactNode;
}>) {
  return (
    <li className="connection-service" data-testid={testId}>
      <Icon className="connection-service-icon" aria-hidden="true" />
      <div className="connection-service-main">
        <span className="connection-service-name">{name}</span>
        <span className="connection-service-desc">{description}</span>
        {children}
      </div>
      {side !== undefined && (
        <div className="connection-service-side">{side}</div>
      )}
    </li>
  );
}

function LastSynced({ conn }: Readonly<{ conn: CaptureConnection }>) {
  const t = useT();
  const { locale } = useLocale();
  return conn.last_synced_at
    ? t("connectors.lastSynced", {
        at: formatDateTime(conn.last_synced_at, locale, viewerZone()),
      })
    : t("connectors.neverSynced");
}

/** A service's own word, and the sentence that says what went wrong. */
function ServiceHealth({ conn }: Readonly<{ conn: CaptureConnection }>) {
  const t = useT();
  const badge = healthBadge(serviceHealth(conn));
  return <Badge tone={badge.tone}>{t(badge.label)}</Badge>;
}

function ServiceError({ conn }: Readonly<{ conn: CaptureConnection }>) {
  const t = useT();
  const health = serviceHealth(conn);
  if (health !== "needs_reconnect" && health !== "needs_attention") {
    return null;
  }
  return (
    <ErrorLine inline standing>
      {t(errorClassKey(conn.last_sync_error_class))}
    </ErrorLine>
  );
}

function NotConnected({
  label,
  pending,
  onConnect,
}: Readonly<{ label: string; pending: boolean; onConnect: () => void }>) {
  const t = useT();
  return (
    <>
      <Badge>{t("connectors.health.notConnected")}</Badge>
      <Button pending={pending} onClick={onConnect}>
        {label}
      </Button>
    </>
  );
}

function NewMailService({
  mail,
  pending,
  onReconnect,
  onConnect,
}: Readonly<{
  mail: CaptureConnection | null;
  pending: boolean;
  onReconnect: (provider: Provider) => void;
  onConnect: () => void;
}>) {
  const t = useT();
  const description = t("connectors.svc.newMailDesc");
  if (!mail) {
    return (
      <ServiceItem
        testId="connector-mail-missing"
        icon={Mail}
        name={t("connectors.svc.newMail")}
        description={description}
        side={
          <NotConnected
            label={t("connectors.connectMail")}
            pending={pending}
            onConnect={onConnect}
          />
        }
      />
    );
  }
  const cannotSend = missingSendGrant(mail);
  const needsReconnect = mail.status === "reauth_required" || cannotSend;
  return (
    <ServiceItem
      testId={`connector-${mail.provider}`}
      icon={Mail}
      name={t("connectors.svc.newMail")}
      description={
        <>
          {description} <LastSynced conn={mail} />
        </>
      }
      side={
        <>
          <ServiceHealth conn={mail} />
          {cannotSend && (
            <Badge tone="warning">{t("connectors.cannotSend")}</Badge>
          )}
          {needsReconnect && (
            <Button
              pending={pending}
              onClick={() => onReconnect(mail.provider)}
            >
              {t("connectors.reconnect")}
            </Button>
          )}
        </>
      }
    >
      <ServiceError conn={mail} />
      {cannotSend && (
        <span className="t-caption">{t("connectors.reconnectToSend")}</span>
      )}
    </ServiceItem>
  );
}

function CalendarService({
  calendar,
  pending,
  onConnect,
}: Readonly<{
  calendar: CaptureConnection | null;
  pending: boolean;
  onConnect: () => void;
}>) {
  const t = useT();
  const description = t("connectors.svc.calendarDesc");
  if (!calendar) {
    return (
      <ServiceItem
        testId="connector-calendar-missing"
        icon={CalendarDays}
        name={t("connectors.svc.calendar")}
        description={description}
        side={
          <NotConnected
            label={t("connectors.connectCalendar")}
            pending={pending}
            onConnect={onConnect}
          />
        }
      />
    );
  }
  const readOnly = missingCalendarWriteGrant(calendar);
  const needsReconnect = calendar.status === "reauth_required" || readOnly;
  return (
    <ServiceItem
      testId={`connector-${calendar.provider}`}
      icon={CalendarDays}
      name={t("connectors.svc.calendar")}
      description={
        <>
          {description} <LastSynced conn={calendar} />
        </>
      }
      side={
        <>
          <ServiceHealth conn={calendar} />
          {readOnly && (
            <Badge tone="warning">{t("scheduling.readOnlyBadge")}</Badge>
          )}
          {needsReconnect && (
            <Button pending={pending} onClick={onConnect}>
              {t("connectors.reconnect")}
            </Button>
          )}
        </>
      }
    >
      <ServiceError conn={calendar} />
      {readOnly && (
        <span className="t-caption">{t("scheduling.readOnlyCalendar")}</span>
      )}
      {calendar.status === "connected" && !readOnly && (
        <a
          className="connection-service-link"
          href={routeHash(settingsHref("meetings"))}
        >
          {t("connectors.svc.bookingLink")}
          <ArrowRight aria-hidden="true" />
        </a>
      )}
    </ServiceItem>
  );
}
