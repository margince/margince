// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import { Button } from "../design-system/atoms";
import { useClipboardCopy } from "../design-system/clipboardcopy";
import { DataTable } from "../design-system/datatable";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import "./oauth-app.css";

/**
 * The connector OAuth app a mailbox connection is made through: Google's, or
 * Microsoft's. One set of hooks and one redirect table for both vendors, so
 * how long a client SECRET lives in a mutation's `variables` has one answer.
 *
 * The secret is never read back. `GET` answers whether one is stored and the
 * client id, which is not a secret: it travels in every authorization redirect.
 */

export type Vendor = "google" | "microsoft";

// Each vendor's own wording, as literal message keys: the translator's key
// type is a closed union, so a key built from the vendor name would not type.
export const vendorCopy = {
  google: {
    title: "oauthApp.google.title",
    clientIdPlaceholder: "oauthApp.google.clientIdPlaceholder",
  },
  microsoft: {
    title: "oauthApp.microsoft.title",
    clientIdPlaceholder: "oauthApp.microsoft.clientIdPlaceholder",
  },
} as const;

/** Narrows a sign-in provider key to a vendor whose app this page reads. */
export function isVendor(key: string): key is Vendor {
  return key === "google" || key === "microsoft";
}

/** Where each vendor's client is created and its two values are read. */
export const VENDOR_CONSOLE: Readonly<Record<Vendor, string>> = {
  google: "https://console.cloud.google.com/apis/credentials",
  microsoft:
    "https://entra.microsoft.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade",
};

/** The narrow sign-in policy read, keyed apart from the installation
 * aggregate. Kept here because an app write is one of the things it answers. */
export const AUTHENTICATION_POLICY_KEY = ["authentication-policy"] as const;

// Keyed per vendor, so storing one app does not refetch the other's view.
function appQueryKey(provider: Vendor) {
  return ["installation-oauth-app", provider];
}

// Every read an app write changes: the app itself, the setup report that names
// it as outstanding, the connector roster whose availability it decides, and
// the sign-in policy. Store and remove share it, so neither can forget one.
async function refreshAfterAppWrite(queryClient: QueryClient, vendor: Vendor) {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: appQueryKey(vendor) }),
    queryClient.invalidateQueries({ queryKey: ["installation-setup"] }),
    queryClient.invalidateQueries({ queryKey: ["connectors"] }),
    queryClient.invalidateQueries({ queryKey: AUTHENTICATION_POLICY_KEY }),
  ]);
}

export function useOAuthApp(provider: Vendor) {
  return useQuery({
    queryKey: appQueryKey(provider),
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/installation/oauth-apps/{provider}",
        { params: { path: { provider } } },
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

export function useSetOAuthApp(provider: Vendor) {
  const queryClient = useQueryClient();
  return useMutation({
    // Collected the moment nothing observes it, because these `variables` hold
    // a credential. The caller resets it once settled for the same reason.
    gcTime: 0,
    mutationFn: async (vars: {
      clientId: string;
      clientSecret: string;
      tenant: string;
    }) => {
      const { error } = await api.PUT("/installation/oauth-apps/{provider}", {
        params: { path: { provider } },
        body: {
          client_id: vars.clientId,
          client_secret: vars.clientSecret,
          // Omitted rather than sent empty: the server refuses a tenant on a
          // vendor that has no directories, and an empty string is a value.
          ...(vars.tenant === "" ? {} : { tenant: vars.tenant }),
        },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => refreshAfterAppWrite(queryClient, provider),
  });
}

export function useRemoveOAuthApp(provider: Vendor) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      const { error } = await api.DELETE(
        "/installation/oauth-apps/{provider}",
        { params: { path: { provider } } },
      );
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => refreshAfterAppWrite(queryClient, provider),
  });
}

// A lookup rather than a computed key, so every purpose the contract declares
// is translated; one it adds later shows its raw enum rather than a raw key.
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

// One address and the control that copies it. A failed copy is said under the
// address the reader pressed for; whether it reads Copied is the table's call,
// because the clipboard holds one string and only one control may claim it.
function RedirectAddress({
  uri,
  copied,
  onCopied,
}: Readonly<{ uri: RedirectUri; copied: boolean; onCopied: () => void }>) {
  const t = useT();
  const copyLabel = t("oauthApp.redirectCopy", {
    purpose: purposeLabel(uri.purpose, t),
  });
  const copy = useClipboardCopy(
    uri.url,
    {
      copy: copyLabel,
      copied: t("oauthApp.redirectCopied"),
      remedy: t("oauthApp.redirectCopyFailed"),
    },
    onCopied,
  );
  return (
    <div className="oauthapp-address">
      <div className="oauthapp-address-line">
        <code>{uri.url}</code>
        <Button
          iconOnly
          aria-label={copied ? t("oauthApp.redirectCopied") : copyLabel}
          onClick={copy.copy}
        >
          {copied ? <Check aria-hidden /> : <Copy aria-hidden />}
        </Button>
      </div>
      {copy.notice}
    </div>
  );
}

// The callback addresses an operator registers on their OAuth client, one per
// flow this deployment serves. They come from the response and are never built
// here: their one job is to be byte-identical to what the vendor receives.
//
// Nothing to register renders nothing, not an empty table that reads as a list
// that failed to load.
export function RedirectUris({
  uris,
}: Readonly<{ uris: readonly RedirectUri[] | undefined }>) {
  const t = useT();
  // The string the clipboard holds, so one control at most reads Copied: a
  // row, or Copy all, never a stale earlier one.
  const [held, setHeld] = useState<string | null>(null);
  const rows = uris ?? [];
  const all = rows.map((uri) => uri.url).join("\n");
  const copyAll = useClipboardCopy(
    all,
    {
      copy: t("oauthApp.copyAll"),
      copied: t("oauthApp.redirectCopied"),
      remedy: t("oauthApp.redirectCopyFailed"),
    },
    () => setHeld(all),
  );
  if (rows.length === 0) {
    return null;
  }
  return (
    <div className="oauthapp-redirects">
      <div className="oauthapp-redirects-head">
        <Button onClick={copyAll.copy}>
          {held === all ? <Check aria-hidden /> : <Copy aria-hidden />}
          {held === all ? t("oauthApp.redirectCopied") : t("oauthApp.copyAll")}
        </Button>
      </div>
      {copyAll.notice}
      <DataTable
        label={t("oauthApp.redirectTable")}
        rows={[...rows]}
        rowKey={(uri) => uri.purpose}
        columns={[
          {
            key: "purpose",
            header: t("oauthApp.redirectCol.usedFor"),
            render: (uri) => purposeLabel(uri.purpose, t),
          },
          {
            key: "address",
            header: t("oauthApp.redirectCol.address"),
            grow: true,
            render: (uri) => (
              <RedirectAddress
                uri={uri}
                copied={held === uri.url}
                onCopied={() => setHeld(uri.url)}
              />
            ),
          },
        ]}
      />
    </div>
  );
}
