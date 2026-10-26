// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import { lockFor } from "../format/settinglocks";
import type { useT } from "../i18n";
import { throwProblem, unwrap } from "./problem";
import { AUTHENTICATION_POLICY_KEY } from "./sign-in-policy";

// The reads and writes of the installation's OAuth apps, with the lock rule
// that decides which app a card shows. The cards live with their screens.

export type Vendor = "google" | "microsoft";

// vendorCopy is each vendor's own wording, as literal message keys.
//
// A record of literals rather than a template built from the provider name.
// The translator's key type is a closed union, so a computed key types as a
// plain string. A missing translation would then reach a reader as a raw key
// instead of failing the build.
export const vendorCopy = {
  google: {
    title: "oauthApp.google.title",
    sub: "oauthApp.google.sub",
    absent: "oauthApp.google.absent",
    redirectSub: "oauthApp.google.redirectSub",
    clientIdPlaceholder: "oauthApp.google.clientIdPlaceholder",
    removeConfirmTitle: "oauthApp.google.removeConfirmTitle",
    removeConfirmBody: "oauthApp.google.removeConfirmBody",
  },
  microsoft: {
    title: "oauthApp.microsoft.title",
    sub: "oauthApp.microsoft.sub",
    absent: "oauthApp.microsoft.absent",
    redirectSub: "oauthApp.microsoft.redirectSub",
    clientIdPlaceholder: "oauthApp.microsoft.clientIdPlaceholder",
    removeConfirmTitle: "oauthApp.microsoft.removeConfirmTitle",
    removeConfirmBody: "oauthApp.microsoft.removeConfirmBody",
  },
} as const;

// The setting key a deployment's own app locks, per vendor.
export const appKey = {
  google: "capture.google_app",
  microsoft: "capture.microsoft_app",
} as const satisfies Record<Vendor, string>;

/** The lock-bearing slice of an app read, so a card and a test can build one. */
type LockedApp = Readonly<{
  configured: boolean;
  locks?: Parameters<typeof lockFor>[0];
}>;

/**
 * Where a core vendor's app comes from. An installation configuration that
 * fixes the app wins over a stored one and refuses every edit. So `lock`
 * carries the sentence naming where to change it. The deprecated `source`
 * field is not read.
 */
export function vendorAppState(
  app: LockedApp,
  provider: Vendor,
  t: ReturnType<typeof useT>,
) {
  const lock = lockFor(app.locks, appKey[provider], t);
  const origin = lock.disabled
    ? ("environment" as const)
    : app.configured
      ? ("stored" as const)
      : ("none" as const);
  return { origin, lock };
}

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

// The query key carries the vendor, so storing one app does not blank the
// other's card. One key for both would make every write invalidate a view it
// says nothing about.
function appQueryKey(provider: string) {
  return ["installation-oauth-app", provider];
}

export const APP_LIST_KEY = ["installation-oauth-apps"];

// Every read an app write changes. That is the app, the list of apps, the
// setup report naming it as outstanding, the connector roster it gates and the
// sign-in policy. Store and remove share it.
async function refreshAfterAppWrite(
  queryClient: QueryClient,
  provider: string,
) {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: appQueryKey(provider) }),
    queryClient.invalidateQueries({ queryKey: APP_LIST_KEY }),
    queryClient.invalidateQueries({ queryKey: ["installation-setup"] }),
    queryClient.invalidateQueries({ queryKey: ["connectors"] }),
    queryClient.invalidateQueries({ queryKey: AUTHENTICATION_POLICY_KEY }),
  ]);
}

export function useOAuthApp(provider: string) {
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

export function useSetOAuthApp(provider: string) {
  const queryClient = useQueryClient();
  return useMutation({
    // Collected the moment nothing observes it, because what this mutation's
    // `variables` hold is a credential rather than a form field. The caller
    // resets it once settled for the same reason.
    gcTime: 0,
    mutationFn: async (vars: {
      clientId: string;
      clientSecret: string;
      tenant: string;
    }) => {
      unwrap(
        await api.PUT("/installation/oauth-apps/{provider}", {
          params: { path: { provider } },
          body: {
            client_id: vars.clientId,
            client_secret: vars.clientSecret,
            // Omitted rather than sent empty: the server refuses a tenant on a
            // vendor that has no directories, and an empty string is a value.
            ...(vars.tenant === "" ? {} : { tenant: vars.tenant }),
          },
        }),
      );
    },
    onSuccess: () => refreshAfterAppWrite(queryClient, provider),
  });
}

export function useRemoveOAuthApp(provider: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      unwrap(
        await api.DELETE("/installation/oauth-apps/{provider}", {
          params: { path: { provider } },
        }),
      );
    },
    onSuccess: () => refreshAfterAppWrite(queryClient, provider),
  });
}
