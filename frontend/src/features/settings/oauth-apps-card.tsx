// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarDays, LogIn, Mail } from "lucide-react";
import { useId, useState } from "react";
import {
  OAuthAppDrawer,
  RedirectAddressesDialog,
  RemoveOAuthAppDialog,
} from "../../components/oauth-app-drawer";
import { Button, OverflowMenu } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { ProviderMark } from "../../design-system/provider-mark";
import { ProviderList, ProviderRow } from "../../design-system/providerrow";
import { QueryGate } from "../../design-system/querystates";
import { Chip } from "../../design-system/readings";
import {
  StatusBadge,
  type StatusKind,
  statusTone,
} from "../../design-system/statusbadge";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { useCompanyPhrase } from "../../lib/company-phrase";
import {
  useOAuthApp,
  type Vendor,
  vendorAppState,
  vendorCopy,
} from "../../lib/oauth-app";
import "../../components/oauth-app.css";

type AppSource = ReturnType<typeof vendorAppState>["origin"];

// The row's own words per vendor, as literal keys for the reason vendorCopy
// spells its own that way.
const rowCopy = {
  google: {
    for: "oauthApp.google.for",
    setUp: "oauthApp.google.setUp",
    unlockMail: "oauthApp.unlock.gmail",
  },
  microsoft: {
    for: "oauthApp.microsoft.for",
    setUp: "oauthApp.microsoft.setUp",
    unlockMail: "oauthApp.unlock.outlook",
  },
} as const;

// An app the installation supplies works as well as a stored one; the label,
// not the colour, says where it came from.
const SOURCE_STATUS: Readonly<
  Record<AppSource, { kind: StatusKind; label: `oauthApp.status.${AppSource}` }>
> = {
  none: { kind: "neutral", label: "oauthApp.status.none" },
  stored: { kind: "done", label: "oauthApp.status.stored" },
  environment: { kind: "done", label: "oauthApp.status.environment" },
};

/**
 * The company's Google and Microsoft apps, one row each. A row shows whether
 * the app is there, where it came from, what it unlocks for everyone, and the
 * one verb that fits.
 *
 * A reader who may not write the apps still sees every verb, refused with one
 * sentence the card says once. The page explains itself to them too.
 */
export function OAuthAppsCard() {
  const t = useT();
  const canManage = useCanWrite("capture_settings", "update");
  const reasonId = useId();
  const company = useCompanyPhrase();
  const google = useOAuthApp("google").data;
  const microsoft = useOAuthApp("microsoft").data;
  // Judged only once both answers are in: one vendor alone cannot say whether
  // the company has an app at all.
  const both = google && microsoft ? [google, microsoft] : null;
  const neither = both?.every((app) => !app.configured) ?? false;
  return (
    <Panel
      title={t("oauthApps.title")}
      titleAction={
        both &&
        (neither ? (
          <StatusBadge
            kind="attention"
            label={t("oauthApps.statusNeedsSetup")}
          />
        ) : (
          <StatusBadge kind="done" label={t("oauthApps.statusReady")} />
        ))
      }
    >
      <PanelBody>
        {neither && (
          <Callout
            tone="warning"
            kind="standing"
            title={t("oauthApps.noneTitle")}
          >
            {t("oauthApps.noneBody")}
          </Callout>
        )}
        {both && !neither && (
          <PanelIntro>{t("oauthApps.readyIntro", { company })}</PanelIntro>
        )}
        {!canManage && (
          <p className="t-caption" id={reasonId}>
            {t("oauthApp.noWriteReason")}
          </p>
        )}
        <ProviderList>
          <AppRow
            provider="google"
            refusedBy={canManage ? undefined : reasonId}
          />
          <AppRow
            provider="microsoft"
            refusedBy={canManage ? undefined : reasonId}
          />
        </ProviderList>
        <p className="t-caption">{t("oauthApps.imapNote")}</p>
      </PanelBody>
    </Panel>
  );
}

// Where the client id came from, said with it.
const CLIENT_LINE = {
  stored: "oauthApp.clientLine",
  environment: "oauthApp.clientLineEnv",
} as const;

// The directory an app is limited to, then what the app unlocks for everyone.
function AppFacts({
  provider,
  tenant,
}: Readonly<{ provider: Vendor; tenant: string | undefined }>) {
  const t = useT();
  return (
    <>
      {tenant && <span>{t("oauthApp.directoryLine", { tenant })}</span>}
      <span>{t("oauthApp.unlocks")}</span>
      <Chip icon={Mail} dense>
        {t(rowCopy[provider].unlockMail)}
      </Chip>
      <Chip icon={CalendarDays} dense>
        {t("oauthApp.unlock.calendar")}
      </Chip>
      <Chip icon={LogIn} dense>
        {t("oauthApp.unlock.signIn")}
      </Chip>
    </>
  );
}

type Opened = "setup" | "redirects" | "remove" | null;

function AppRow({
  provider,
  refusedBy,
}: Readonly<{ provider: Vendor; refusedBy: string | undefined }>) {
  const t = useT();
  const copy = rowCopy[provider];
  const name = t(vendorCopy[provider].title);
  const app = useOAuthApp(provider);
  const [opened, setOpened] = useState<Opened>(null);
  // Whichever primary verb the row shows wears this id, so focus has somewhere
  // to land once a removal has taken the overflow menu away.
  const actionId = useId();
  const close = () => setOpened(null);
  return (
    <>
      <QueryGate query={app} pendingLabel={name}>
        {(status) => {
          const { origin, lock } = vendorAppState(status, provider, t);
          return (
            <ProviderRow
              mark={<ProviderMark providerKey={provider} />}
              name={name}
              headingLevel="h3"
              status={{
                tone: statusTone(SOURCE_STATUS[origin].kind),
                label: t(SOURCE_STATUS[origin].label),
              }}
              account={
                origin === "none"
                  ? t(copy.for)
                  : t(CLIENT_LINE[origin], { clientId: status.client_id })
              }
              facts={<AppFacts provider={provider} tenant={status.tenant} />}
              // The installation's own app refuses every edit, so the row
              // names where to change it instead of offering a verb.
              reason={lock.disabled ? lock.reason : undefined}
              // Google and Microsoft are equal choices: neither verb is primary.
              action={
                origin === "none" ? (
                  <Button
                    id={actionId}
                    reasonId={refusedBy}
                    onClick={() => setOpened("setup")}
                  >
                    {t(copy.setUp)}
                  </Button>
                ) : undefined
              }
              menu={
                origin === "stored" ? (
                  <OverflowMenu label={t("oauthApp.menu", { app: name })}>
                    <Button onClick={() => setOpened("redirects")}>
                      {t("oauthApp.menu.showRedirects")}
                    </Button>
                    <Button
                      reasonId={refusedBy}
                      onClick={() => setOpened("setup")}
                    >
                      {t("oauthApp.menu.replace")}
                    </Button>
                    <Button
                      variant="danger"
                      reasonId={refusedBy}
                      onClick={() => setOpened("remove")}
                    >
                      {t("oauthApp.remove")}
                    </Button>
                  </OverflowMenu>
                ) : lock.disabled ? (
                  <OverflowMenu label={t("oauthApp.menu", { app: name })}>
                    <Button onClick={() => setOpened("redirects")}>
                      {t("oauthApp.menu.showRedirects")}
                    </Button>
                  </OverflowMenu>
                ) : undefined
              }
            />
          );
        }}
      </QueryGate>
      <OAuthAppDrawer
        provider={provider}
        open={opened === "setup"}
        onClose={close}
      />
      <RedirectAddressesDialog
        provider={provider}
        open={opened === "redirects"}
        onClose={close}
      />
      <RemoveOAuthAppDialog
        provider={provider}
        open={opened === "remove"}
        onClose={close}
        returnFocusTo={() => document.getElementById(actionId)}
      />
    </>
  );
}
