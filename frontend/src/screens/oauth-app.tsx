// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  type QueryClient,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../api/client";
import { throwProblem, unwrap } from "../lib/problem";
import "./oauth-app.css";

/**
 * The connector OAuth app a mailbox connection is made through: Google's, or
 * Microsoft's. One set of hooks for both vendors, so
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

export function useRemoveOAuthApp(provider: Vendor) {
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
