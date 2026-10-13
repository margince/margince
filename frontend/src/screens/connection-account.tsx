// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CalendarDays, Mail, Send } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, OverflowMenu } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { ErrorLine } from "../design-system/errorline";
import { ProviderMark } from "../design-system/provider-mark";
import { ProviderRow } from "../design-system/providerrow";
import { Chip } from "../design-system/readings";
import {
  ACCOUNT_NAME,
  type AccountVendor,
  type ConnectionAccount,
  isMailbox,
} from "../format/connectorprovider";
import { formatDateTime } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Translator, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, throwProblem } from "../lib/problem";
import { ImportPastEmailDialog } from "./backfill";
import { isLiveRun } from "./backfill-run";
import {
  type ConnectAccountInput,
  type ImportWindow,
  isOAuthConnectProvider,
  notConfiguredProvider,
  useConnectAccount,
} from "./connect-account";
import { connectInputFor, type Service } from "./connect-flow-draft";
import { type BringsInChange, BringsInDialog } from "./connection-bringsin";
import { AccountServices } from "./connection-service";
import { AccountSettings } from "./connection-settings";
import {
  accountHealth,
  healthBadge,
  missingSendGrant,
} from "./connector-status";

// One account on Connections: Google, Microsoft 365, an IMAP mailbox or the
// test mailbox. Its row says how the account is doing as a whole; the services
// under it (new mail, past email, the calendar) each say how they are doing and
// carry their own verb, and the overflow holds the verbs about the account.

type CaptureConnection = components["schemas"]["CaptureConnection"];
type Provider = CaptureConnection["provider"];

// Disconnecting deletes OUR stored credential and does not revoke the grant at
// the vendor, so the confirm names where a careful reader finishes that.
const REVOKE_NOTE: Record<AccountVendor, MessageKey> = {
  google: "connectors.disconnectBodyGoogleNote",
  microsoft: "connectors.disconnectBodyMicrosoftNote",
};

function vendorOfAccount(account: ConnectionAccount): AccountVendor | null {
  return account.key === "google" || account.key === "microsoft"
    ? account.key
    : null;
}

function servicesLabel(services: readonly Service[], t: Translator): string {
  if (services.length === 2) return t("connectors.servicesBoth");
  return services[0] === "calendar"
    ? t("connectors.bringsCalendar")
    : t("connectors.bringsMail");
}

// What one disconnect stops: the services, and what may follow it once they
// are gone (a service the same change ticked).
type PendingStop = Readonly<{
  services: readonly Service[];
  afterwards: ConnectAccountInput | null;
}>;

function servicesOf(account: ConnectionAccount): Service[] {
  const services: Service[] = [];
  if (account.mail) services.push("mail");
  if (account.calendar) services.push("calendar");
  return services;
}

/** The account's connected services, split into the mailbox the settings
 *  block is about and the services without mail beside it. */
function settingsSplit(account: ConnectionAccount) {
  const live = [account.mail, account.calendar].filter(
    (service): service is CaptureConnection =>
      service !== null && service.status === "connected",
  );
  return {
    mailbox: live.find((service) => isMailbox(service.provider)) ?? null,
    others: live.filter((service) => !isMailbox(service.provider)),
  };
}

export function ConnectionAccountRow({
  account,
  heldWindow,
  onImapReconnect,
}: Readonly<{
  account: ConnectionAccount;
  /** A past-email window asked for at connect and held for the reader. */
  heldWindow: ImportWindow | null;
  onImapReconnect: () => void;
}>) {
  const t = useT();
  const { mail, calendar } = account;
  const vendor = vendorOfAccount(account);
  const name = t(ACCOUNT_NAME[account.key]);
  const label = mail?.account_label ?? calendar?.account_label ?? undefined;
  const connect = useConnectAccount();
  const [bringsInOpen, setBringsInOpen] = useState(false);
  const [importOpen, setImportOpen] = useState(false);
  const [stop, setStop] = useState<PendingStop | null>(null);
  const present = servicesOf(account);

  const reconnect = (provider: Provider) => {
    if (isOAuthConnectProvider(provider)) {
      connect.mutate({ provider, returnTo: "settings" });
    } else if (provider === "imap") {
      onImapReconnect();
    }
  };
  // The account's own Reconnect asks again for everything it holds: the mail
  // consent with its calendar chained after, or the one service alone.
  const reconnectAccount = () => {
    const input = vendor ? connectInputFor(vendor, present, "none") : null;
    if (input) connect.mutate(input);
    else if (mail) reconnect(mail.provider);
  };
  const changeServices = (change: BringsInChange) => {
    const added =
      vendor && change.added.length > 0
        ? connectInputFor(vendor, change.added, "none")
        : null;
    if (change.removed.length > 0) {
      setBringsInOpen(false);
      setStop({ services: change.removed, afterwards: added });
    } else if (added) {
      connect.mutate(added);
    }
  };
  const health = healthBadge(accountHealth(account));
  const connectPendingFor = connect.isPending
    ? connect.variables?.provider
    : undefined;

  return (
    <ProviderRow
      mark={vendor ? <ProviderMark providerKey={vendor} /> : <Mail />}
      name={name}
      headingLevel="h3"
      status={{
        tone: health.tone,
        label: t(health.label),
        live: health.label === "connectors.health.syncing",
      }}
      account={label}
      facts={<AccountFacts account={account} />}
      menu={
        <AccountMenu
          account={account}
          name={name}
          onBringsIn={() => setBringsInOpen(true)}
          onImport={() => setImportOpen(true)}
          onReconnect={reconnectAccount}
          onDisconnect={() => setStop({ services: present, afterwards: null })}
        />
      }
    >
      <AccountServices
        account={account}
        vendor={vendor}
        label={t("connectors.svc.list", { account: name })}
        heldWindow={heldWindow}
        connectPendingFor={connectPendingFor}
        onReconnect={reconnect}
        onImport={() => setImportOpen(true)}
      />
      {!bringsInOpen && <ConnectRefusal connect={connect} name={name} />}
      <AccountSettings
        {...settingsSplit(account)}
        accountName={label ?? name}
      />
      {vendor && mail && (
        <ImportPastEmailDialog
          provider={mail.provider}
          initial={mail.backfill}
          open={importOpen}
          onClose={() => setImportOpen(false)}
        />
      )}
      {vendor && (
        <BringsInDialog
          open={bringsInOpen}
          onClose={() => {
            connect.reset();
            setBringsInOpen(false);
          }}
          accountName={label ?? name}
          vendorName={name}
          present={present}
          pending={connect.isPending}
          error={connect.error}
          onSave={changeServices}
        />
      )}
      <StopConfirm
        account={account}
        title={label ?? name}
        stop={stop}
        onClose={() => setStop(null)}
        onStopped={(afterwards) => {
          setStop(null);
          if (afterwards) connect.mutate(afterwards);
        }}
      />
    </ProviderRow>
  );
}

/** The verbs about the account as a whole, in words. */
function AccountMenu({
  account,
  name,
  onBringsIn,
  onImport,
  onReconnect,
  onDisconnect,
}: Readonly<{
  account: ConnectionAccount;
  name: string;
  onBringsIn: () => void;
  onImport: () => void;
  onReconnect: () => void;
  onDisconnect: () => void;
}>) {
  const t = useT();
  const vendor = vendorOfAccount(account);
  const mail = account.mail;
  return (
    <OverflowMenu label={t("connectors.account.menu", { account: name })}>
      {vendor && (
        <Button onClick={onBringsIn}>{t("connectors.menu.bringsIn")}</Button>
      )}
      {vendor && mail && !isLiveRun(mail.backfill?.state) && (
        <Button onClick={onImport}>{t("connectors.menu.importOlder")}</Button>
      )}
      {account.key !== "test_mailbox" && (
        <Button onClick={onReconnect}>{t("connectors.reconnect")}</Button>
      )}
      <Button variant="danger" onClick={onDisconnect}>
        {t("connectors.disconnect")}
      </Button>
    </OverflowMenu>
  );
}

/** What the account brings in, and since when. */
function AccountFacts({ account }: Readonly<{ account: ConnectionAccount }>) {
  const t = useT();
  const { locale } = useLocale();
  const { mail, calendar } = account;
  const createdAt = [mail?.created_at, calendar?.created_at]
    .filter((at): at is string => typeof at === "string")
    .sort()[0];
  return (
    <>
      {mail && (
        <Chip icon={Mail} dense>
          {t("connectors.bringsMail")}
        </Chip>
      )}
      {calendar && (
        <Chip icon={CalendarDays} dense>
          {t("connectors.bringsCalendar")}
        </Chip>
      )}
      {vendorOfAccount(account) && mail && !missingSendGrant(mail) && (
        <Chip icon={Send} dense>
          {t("connectors.bringsSend")}
        </Chip>
      )}
      {createdAt && (
        <span>
          {t("connectors.account.connectedOn", {
            date: formatDateTime(createdAt, locale, viewerZone()),
          })}
        </span>
      )}
    </>
  );
}

/** Why the last connect pressed on this account did not leave for the vendor. */
function ConnectRefusal({
  connect,
  name,
}: Readonly<{ connect: ReturnType<typeof useConnectAccount>; name: string }>) {
  const t = useT();
  if (notConfiguredProvider(connect.data)) {
    return (
      <ErrorLine>
        {t("connectors.providerNotConfigured", { provider: name })}
      </ErrorLine>
    );
  }
  return connect.isError ? (
    <ErrorLine>{problemMessageOf(connect.error, t)}</ErrorLine>
  ) : null;
}

/** One confirm for every disconnect: the whole account from the overflow, or
 *  the services "Change what it brings in" unticked. */
function StopConfirm({
  account,
  title,
  stop,
  onClose,
  onStopped,
}: Readonly<{
  account: ConnectionAccount;
  title: string;
  stop: PendingStop | null;
  onClose: () => void;
  onStopped: (afterwards: ConnectAccountInput | null) => void;
}>) {
  const t = useT();
  const disconnect = useDisconnectServices();
  const vendor = vendorOfAccount(account);
  const services = stop?.services ?? [];
  const whole = services.length === servicesOf(account).length;
  const providers = services
    .map((service) =>
      service === "mail" ? account.mail?.provider : account.calendar?.provider,
    )
    .filter((provider): provider is Provider => provider !== undefined);
  return (
    <ConfirmModal
      open={stop !== null}
      onClose={() => {
        disconnect.reset();
        onClose();
      }}
      title={
        whole
          ? t("connectors.disconnectAccountTitle", { account: title })
          : t("connectors.stopServicesTitle", {
              services: servicesLabel(services, t),
            })
      }
      confirmLabel={t("connectors.disconnect")}
      confirmVariant="danger"
      pending={disconnect.isPending}
      error={disconnect.isError ? problemMessageOf(disconnect.error, t) : null}
      onConfirm={() =>
        disconnect.mutate(providers, {
          onSuccess: () => onStopped(stop?.afterwards ?? null),
        })
      }
    >
      <p>
        {t("connectors.disconnectServicesBody", {
          services: servicesLabel(services, t),
        })}
      </p>
      {vendor && <p className="t-caption">{t(REVOKE_NOTE[vendor])}</p>}
    </ConfirmModal>
  );
}

/** Stops each named service in turn. One failure stops the rest, and the
 *  roster read afterwards says which are still connected. */
function useDisconnectServices() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (providers: readonly Provider[]) => {
      for (const provider of providers) {
        const { error } = await api.POST("/connectors/{provider}/disconnect", {
          params: { path: { provider } },
        });
        if (error) {
          throwProblem(error);
        }
      }
    },
    onSettled: () => qc.invalidateQueries({ queryKey: ["connectors"] }),
  });
}
