// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { CalendarDays, Info, LogIn, Mail } from "lucide-react";
import { useId, useState } from "react";
import type { components } from "../api/schema";
import { useCanWrite } from "../app/capability";
import { Badge, Button, OverflowMenu } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { ProviderMark } from "../design-system/provider-mark";
import {
  ProviderList,
  ProviderRow,
  type ProviderRowTone,
} from "../design-system/providerrow";
import { Chip } from "../design-system/readings";
import { useT } from "../i18n";
import { QueryGate } from "./common";
import { useCompanyPhrase } from "./company-phrase";
import { useOAuthApp, type Vendor, vendorCopy } from "./oauth-app";
import {
  OAuthAppDrawer,
  RedirectAddressesDialog,
  RemoveOAuthAppDialog,
} from "./oauth-app-drawer";
import "./oauth-app.css";

type AppSource = components["schemas"]["ConnectorApp"]["source"];

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

const SOURCE_STATUS: Readonly<
  Record<
    AppSource,
    { tone: ProviderRowTone; label: `oauthApp.status.${AppSource}` }
  >
> = {
  none: { tone: "default", label: "oauthApp.status.none" },
  stored: { tone: "success", label: "oauthApp.status.stored" },
  environment: { tone: "info", label: "oauthApp.status.environment" },
};

/**
 * The company's Google and Microsoft apps, one row each: whether it is there,
 * where it came from, what it unlocks for everyone, and the one verb that fits.
 *
 * A reader who may not write the apps still sees every verb, refused with one
 * sentence the card says once, so the page explains itself to them too.
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
  const neither = both?.every((app) => app.source === "none") ?? false;
  return (
    <Panel
      title={t("oauthApps.title")}
      titleAction={
        both &&
        (neither ? (
          <Badge tone="warning">{t("oauthApps.statusNeedsSetup")}</Badge>
        ) : (
          <Badge tone="success">{t("oauthApps.statusReady")}</Badge>
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
          <p className="t-body">{t("oauthApps.readyIntro", { company })}</p>
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
        <p className="t-caption oauthapp-foot">
          <Info size={14} aria-hidden="true" />
          {t("oauthApps.imapNote")}
        </p>
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
        {(status) => (
          <ProviderRow
            mark={<ProviderMark providerKey={provider} />}
            name={name}
            headingLevel="h3"
            status={{
              tone: SOURCE_STATUS[status.source].tone,
              label: t(SOURCE_STATUS[status.source].label),
            }}
            account={
              status.source === "none"
                ? t(copy.for)
                : t(CLIENT_LINE[status.source], { clientId: status.client_id })
            }
            facts={<AppFacts provider={provider} tenant={status.tenant} />}
            action={
              status.source === "stored" ? undefined : (
                <Button
                  id={actionId}
                  variant={status.source === "none" ? "primary" : undefined}
                  reasonId={refusedBy}
                  onClick={() => setOpened("setup")}
                >
                  {status.source === "none"
                    ? t(copy.setUp)
                    : t("oauthApp.useOwn")}
                </Button>
              )
            }
            menu={
              status.source === "stored" ? (
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
              ) : undefined
            }
          />
        )}
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
