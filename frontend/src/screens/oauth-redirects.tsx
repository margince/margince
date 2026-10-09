// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { createPortal } from "react-dom";
import { Button } from "../design-system/atoms";
import { useClipboardCopy } from "../design-system/clipboardcopy";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { Heading } from "../design-system/heading";
import { PanelBody, PanelIntro } from "../design-system/panel";
import { useT } from "../i18n";
import "./oauth-redirects.css";

// The callback addresses a vendor OAuth app must carry, one per flow this
// installation serves: in a vendor's settings panel and on the first-run step.

// A switch, not a computed key: the catalog is a closed union of keys. An
// unknown purpose shows its raw enum, so every contract purpose needs an arm.
function purposeLabel(purpose: string, t: ReturnType<typeof useT>): string {
  switch (purpose) {
    case "sign_in":
      return t("oauthApp.redirect.sign_in");
    case "mailbox_connect":
      return t("oauthApp.redirect.mailbox_connect");
    case "calendar_connect":
      return t("oauthApp.redirect.calendar_connect");
    default:
      return purpose;
  }
}

type RedirectUri = Readonly<{ purpose: string; url: string }>;

// The failure notice goes to the table's slot: a cell has no room for a
// sentence. Copied is the table's call, since the clipboard holds one address.
function RedirectCopy({
  uri,
  purpose,
  copied,
  current,
  onPress,
  onCopied,
  noticeSlot,
}: Readonly<{
  uri: RedirectUri;
  purpose: string;
  copied: boolean;
  current: boolean;
  onPress: () => void;
  onCopied: () => void;
  noticeSlot: HTMLElement | null;
}>) {
  const t = useT();
  const copy = useClipboardCopy(
    uri.url,
    {
      copy: t("oauthApp.copy"),
      copied: t("oauthApp.redirectCopied"),
      remedy: t("oauthApp.redirectCopyFailed"),
    },
    onCopied,
  );
  return (
    <div className="cell-actions">
      <Button
        aria-label={
          copied ? undefined : t("oauthApp.redirectCopy", { purpose })
        }
        onClick={() => {
          onPress();
          copy.copy();
        }}
      >
        {copied ? t("oauthApp.redirectCopied") : t("oauthApp.copy")}
      </Button>
      {current && copy.notice && noticeSlot
        ? createPortal(copy.notice, noticeSlot)
        : null}
    </div>
  );
}

// The URLs come from the response and are never built here. A second spelling
// in the client is how they stop matching what the vendor receives.
export function RedirectUriTable({
  uris,
  bleed,
  noticeSlot,
}: Readonly<{
  uris: readonly RedirectUri[];
  bleed?: boolean;
  noticeSlot: HTMLElement | null;
}>) {
  const t = useT();
  const [copiedUrl, setCopiedUrl] = useState<string | null>(null);
  const [pressedUrl, setPressedUrl] = useState<string | null>(null);
  const columns: DataTableColumn<RedirectUri>[] = [
    {
      key: "purpose",
      header: t("oauthApp.redirectPurpose"),
      render: (uri) => purposeLabel(uri.purpose, t),
    },
    {
      key: "uri",
      header: t("oauthApp.redirectUri"),
      grow: true,
      render: (uri) => (
        <code className="oauthredirect-uri" title={uri.url}>
          {uri.url}
        </code>
      ),
    },
    {
      key: "copy",
      header: t("table.actions"),
      headerHidden: true,
      fold: "end",
      align: "end",
      render: (uri) => (
        <RedirectCopy
          uri={uri}
          purpose={purposeLabel(uri.purpose, t)}
          copied={copiedUrl === uri.url}
          current={pressedUrl === uri.url}
          onPress={() => setPressedUrl(uri.url)}
          onCopied={() => setCopiedUrl(uri.url)}
          noticeSlot={noticeSlot}
        />
      ),
    },
  ];
  return (
    <DataTable
      label={t("oauthApp.redirectTitle")}
      bleed={bleed}
      fold
      columns={columns}
      rows={[...uris]}
      rowKey={(uri) => uri.purpose}
    />
  );
}

// An empty list renders nothing: a bare heading reads as a failed load.
export function RedirectUris({
  uris,
  sub,
}: Readonly<{
  uris: readonly RedirectUri[] | undefined;
  sub: string;
}>) {
  const t = useT();
  const [noticeSlot, setNoticeSlot] = useState<HTMLElement | null>(null);
  // Absent and empty are the same answer here, and a body that lost the
  // contract-required field hands over `undefined` anyway.
  if (!uris || uris.length === 0) {
    return null;
  }
  return (
    <div>
      <p className="t-label">{t("oauthApp.redirectTitle")}</p>
      <p className="t-caption">{sub}</p>
      <div className="oauthredirect-notice" ref={setNoticeSlot} />
      <RedirectUriTable uris={uris} noticeSlot={noticeSlot} />
    </div>
  );
}

// The same list as a section of a vendor's panel: its heading, the caption, and
// the table edge to edge.
export function RedirectUriGroup({
  uris,
  sub,
}: Readonly<{ uris: readonly RedirectUri[] | undefined; sub: string }>) {
  const t = useT();
  const [noticeSlot, setNoticeSlot] = useState<HTMLElement | null>(null);
  if (!uris || uris.length === 0) {
    return null;
  }
  return (
    <>
      <PanelBody>
        <Heading size="xsmall" as="h3" className="oauthredirect-redirect-title">
          {t("oauthApp.redirectTitle")}
        </Heading>
        <div className="oauthredirect-notice" ref={setNoticeSlot} />
        <PanelIntro>{sub}</PanelIntro>
      </PanelBody>
      <RedirectUriTable uris={uris} bleed noticeSlot={noticeSlot} />
    </>
  );
}
