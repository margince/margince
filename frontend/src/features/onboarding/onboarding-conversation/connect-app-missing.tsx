// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Copy } from "lucide-react";
import { type ReactNode, useState } from "react";
import { OAuthAppDrawer } from "../../../components/oauth-app-drawer";
import { Button } from "../../../design-system/atoms";
import { useClipboardCopy } from "../../../design-system/clipboardcopy";
import {
  Panel,
  PanelBody,
  PanelIntro,
  PanelPlate,
} from "../../../design-system/panel";
import { providerBrandName } from "../../../design-system/provider-mark";
import type { AccountVendor } from "../../../format/connectorprovider";
import { useT } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { useCanWrite } from "../../../lib/capability";
import { useCompanyPhrase } from "../../../lib/company-phrase";
import type { AdminWait } from "../../../lib/setup-checklist";

// The picked account's company app is missing or will not open, so no connect
// can start. An administrator fixes that here, in place, because the onboarding
// gate bounces a link to Settings back to this step. Anyone else gets a request
// to hand on and the way to another mail provider.

const WAIT_COPY: Readonly<
  Record<
    AdminWait,
    Readonly<{
      title: MessageKey;
      body: MessageKey;
      holderBody: MessageKey;
      request: MessageKey;
      fix: MessageKey;
    }>
  >
> = {
  app_missing: {
    title: "ob.conv.connect.appMissingTitle",
    body: "ob.conv.connect.appMissingBody",
    holderBody: "ob.conv.connect.appMissingHolderBody",
    request: "ob.conv.connect.requestText",
    fix: "ob.conv.connect.registerApp",
  },
  // A stored app whose secret will not open needs that secret replaced, never a
  // second app, so every line here says so.
  app_unusable: {
    title: "ob.conv.connect.appUnusableTitle",
    body: "ob.conv.connect.appUnusableBody",
    holderBody: "ob.conv.connect.appUnusableHolderBody",
    request: "ob.conv.connect.requestTextUnusable",
    fix: "ob.conv.connect.replaceApp",
  },
};

export function AppWaitingPanel({
  vendor,
  wait,
  onUseImap,
}: Readonly<{
  vendor: AccountVendor;
  wait: AdminWait;
  /** Picks the IMAP account instead; null when this installation cannot
   *  connect one either. */
  onUseImap: (() => void) | null;
}>) {
  const t = useT();
  // The grant the app endpoints enforce, as Settings asks it.
  const holder = useCanWrite("capture_settings", "update");
  const company = useCompanyPhrase();
  const copy = WAIT_COPY[wait];
  const words = {
    vendor: providerBrandName(vendor) ?? vendor,
    company,
    page: t("settings.tab.authentication"),
  };
  const title = t(copy.title, words);
  const useImap = onUseImap && (
    <Button onClick={onUseImap}>{t("ob.conv.connect.useImap")}</Button>
  );
  return holder ? (
    <HolderFix
      vendor={vendor}
      title={title}
      body={t(copy.holderBody, words)}
      fix={t(copy.fix, words)}
      useImap={useImap}
    />
  ) : (
    <AdminRequest
      title={title}
      body={t(copy.body, words)}
      request={t(copy.request, words)}
      useImap={useImap}
    />
  );
}

/** The administrator's way: the app form, in a drawer over the step. Its save
 *  refreshes the roster, so the account turns connectable where it stands. */
function HolderFix({
  vendor,
  title,
  body,
  fix,
  useImap,
}: Readonly<{
  vendor: AccountVendor;
  title: string;
  body: string;
  fix: string;
  useImap: ReactNode;
}>) {
  const [open, setOpen] = useState(false);
  return (
    <Panel
      tone="warning"
      title={title}
      actions={
        <>
          {useImap}
          <Button variant="primary" onClick={() => setOpen(true)}>
            {fix}
          </Button>
        </>
      }
    >
      <PanelBody>
        <PanelIntro>{body}</PanelIntro>
      </PanelBody>
      <OAuthAppDrawer
        provider={vendor}
        open={open}
        onClose={() => setOpen(false)}
      />
    </Panel>
  );
}

/** Everyone else's way: the request, worded for the administrator, on the
 *  clipboard to send however this company talks. */
function AdminRequest({
  title,
  body,
  request,
  useImap,
}: Readonly<{
  title: string;
  body: string;
  request: string;
  useImap: ReactNode;
}>) {
  const t = useT();
  const clip = useClipboardCopy(request, {
    copy: t("ob.conv.connect.copyRequest"),
    copied: t("ob.conv.connect.requestCopied"),
    remedy: t("ob.conv.connect.requestCopyFailed", { request }),
  });
  return (
    <Panel
      tone="warning"
      title={title}
      actions={
        <>
          {useImap}
          <Button variant="primary" onClick={clip.copy}>
            <Copy aria-hidden />
            {clip.label}
          </Button>
        </>
      }
    >
      <PanelBody>
        <PanelIntro>{body}</PanelIntro>
        {/* Shown as well as copied, so the reader knows what they send. */}
        <PanelPlate>
          <p className="t-body">{request}</p>
        </PanelPlate>
        {clip.notice}
      </PanelBody>
    </Panel>
  );
}
