// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";
import { labelFor } from "../lib/labelfor";

type Provider = components["schemas"]["CaptureConnection"]["provider"];

// Keyed by the contract enum, so a provider added there without a name here
// fails the typecheck.
const PROVIDER_NAME: Readonly<Record<Provider, MessageKey>> = {
  gmail: "connectors.provGmail",
  gcal: "connectors.provGcal",
  gdocs: "connectors.provGdocs",
  graph: "connectors.provGraph",
  graphcal: "connectors.provGraphCal",
  imap: "connectors.provImap",
  test_mailbox: "connectors.provTestMailbox",
};

// Mirrors the server's `compose.MailProviders`, which refuses mail operations
// on a calendar; backend/gates/frontendmailproviders_test.go holds them equal.
export const MAIL_PROVIDERS: ReadonlySet<Provider> = new Set<Provider>([
  "gmail",
  "graph",
  "imap",
]);

export function isMailbox(provider: Provider): boolean {
  return MAIL_PROVIDERS.has(provider);
}

// A test mailbox is mail, though no mailbox setting applies to it.
export function isMailIcon(provider: Provider): boolean {
  return isMailbox(provider) || provider === "test_mailbox";
}

export function isCaptureProvider(id: string): id is Provider {
  return Object.hasOwn(PROVIDER_NAME, id);
}

/** The catalog key that names a capture provider. */
export function providerNameKey(provider: Provider): MessageKey {
  return labelFor(PROVIDER_NAME, provider);
}

// An id this build cannot name comes back as sent: a newer server's provider is
// still one the reader can quote.
export function providerName(id: string, t: (key: MessageKey) => string) {
  return isCaptureProvider(id) ? t(labelFor(PROVIDER_NAME, id)) : id;
}

/** The two vendors whose mail and calendar are separate connections of one account. */
export type AccountVendor = "google" | "microsoft";

/** Each vendor's mailbox and calendar, the same pairs the server chains with
 *  `also_connect` (gmail then gcal, graph then graphcal). `oneConsent` mirrors
 *  the server's `sharedConsentCalendars`: Google asks for both on one screen,
 *  Microsoft asks once for mail and once more for the calendar. */
export const VENDOR_SERVICES: Readonly<
  Record<
    AccountVendor,
    Readonly<{
      mail: "gmail" | "graph";
      calendar: "gcal" | "graphcal";
      oneConsent: boolean;
    }>
  >
> = {
  google: { mail: "gmail", calendar: "gcal", oneConsent: true },
  microsoft: { mail: "graph", calendar: "graphcal", oneConsent: false },
};

/** Google's meeting-notes connection: its own consent to read the Docs Meet
 *  writes, a third service of the Google account beside mail and calendar. */
export const MEETING_NOTES_PROVIDER = "gdocs";

/** The vendor a provider belongs to, or null for IMAP, the test mailbox and
 *  any string that is not a provider (a route segment, say). */
export function vendorOf(provider: string | undefined): AccountVendor | null {
  if (provider === MEETING_NOTES_PROVIDER) return "google";
  const vendors: readonly AccountVendor[] = ["google", "microsoft"];
  return (
    vendors.find(
      (vendor) =>
        provider === VENDOR_SERVICES[vendor].mail ||
        provider === VENDOR_SERVICES[vendor].calendar,
    ) ?? null
  );
}

/** The providers that connect by a consent redirect: every vendor's mail and
 *  calendar, and Google's meeting notes. IMAP posts credentials and gets a
 *  connection back, never a URL. */
export type OAuthConnectProvider =
  | (typeof VENDOR_SERVICES)[AccountVendor]["mail" | "calendar"]
  | typeof MEETING_NOTES_PROVIDER;

/** Narrows any string (a route segment, a stored value) to an OAuth provider
 *  by membership, never by assertion. */
export function asOAuthConnectProvider(
  value: string | undefined,
): OAuthConnectProvider | null {
  if (value === MEETING_NOTES_PROVIDER) return MEETING_NOTES_PROVIDER;
  const vendor = vendorOf(value);
  if (vendor === null) return null;
  const { mail, calendar } = VENDOR_SERVICES[vendor];
  return value === mail ? mail : calendar;
}

type CaptureConnection = components["schemas"]["CaptureConnection"];

/** A provider that pairs with no other: IMAP and the test mailbox carry no
 *  calendar, so each is an account of its own. */
type LoneProvider = Exclude<Provider, OAuthConnectProvider>;

/** One account as the member thinks of it: a mailbox, a calendar, or both,
 *  and for Google the meeting notes. */
export type ConnectionAccount = Readonly<{
  key: AccountVendor | LoneProvider;
  /** Names this account among others of one vendor: a member may connect
   *  several, so the vendor alone is not a key. */
  id: string;
  mail: CaptureConnection | null;
  calendar: CaptureConnection | null;
  notes: CaptureConnection | null;
}>;

/** The roster folded into accounts, Google first, then Microsoft 365, then the
 *  rest in roster order. One vendor's services belong to one account when they
 *  name the same address, so a second Google account is a second row. A
 *  calendar with no mailbox beside it stays an account.
 *
 *  Takes the rows the caller shows: a disconnected gcal left in the input
 *  would read as a connected calendar, so the caller filters those out first. */
export function accountsOf(
  connections: readonly CaptureConnection[],
): ConnectionAccount[] {
  const vendors: readonly AccountVendor[] = ["google", "microsoft"];
  const paired = vendors.flatMap((vendor) => {
    const services = VENDOR_SERVICES[vendor];
    const own = connections.filter(
      (c) =>
        c.provider === services.mail ||
        c.provider === services.calendar ||
        (vendor === "google" && c.provider === MEETING_NOTES_PROVIDER),
    );
    // Mail and calendar name the accounts. Meeting notes, and a row that names
    // no address, join the vendor's first account unless one names theirs.
    // That is where they sat while a member held one account per vendor.
    const named = own.filter((c) => c.provider !== MEETING_NOTES_PROVIDER);
    const addresses = addressesOf(named.length > 0 ? named : own);
    return addresses.map((address, i) => {
      const row = (provider: Provider) =>
        rowFor(own, provider, address) ??
        (i === 0
          ? (own.find(
              (c) =>
                c.provider === provider && !addresses.includes(addressOf(c)),
            ) ?? null)
          : null);
      return {
        key: vendor,
        id: `${vendor}:${address}`,
        mail: row(services.mail),
        calendar: row(services.calendar),
        notes: vendor === "google" ? row(MEETING_NOTES_PROVIDER) : null,
      };
    });
  });
  const lone = connections.flatMap((connection) => {
    const provider = connection.provider;
    return isLoneProvider(provider)
      ? [
          {
            key: provider,
            id: connection.id,
            mail: connection,
            calendar: null,
            notes: null,
          },
        ]
      : [];
  });
  return [...paired, ...lone].filter(
    (account) =>
      account.mail !== null ||
      account.calendar !== null ||
      account.notes !== null,
  );
}

/** The account address a connection names, compared without case as the
 *  server's key does. A connection that names none is the empty address. */
function addressOf(connection: CaptureConnection): string {
  return (connection.account_label ?? "").toLowerCase();
}

function addressesOf(connections: readonly CaptureConnection[]): string[] {
  const named = [...new Set(connections.map(addressOf))].filter(Boolean);
  return named.length > 0 ? named : [""];
}

function rowFor(
  connections: readonly CaptureConnection[],
  provider: Provider,
  address: string,
): CaptureConnection | null {
  return (
    connections.find(
      (c) => c.provider === provider && addressOf(c) === address,
    ) ?? null
  );
}

function isLoneProvider(provider: Provider): provider is LoneProvider {
  return vendorOf(provider) === null;
}

/** The name an account goes by: its vendor for Google and Microsoft 365, the
 *  kind of mailbox for the rest. */
export const ACCOUNT_NAME: Readonly<
  Record<ConnectionAccount["key"], MessageKey>
> = {
  google: "connectors.vendorGoogle",
  microsoft: "connectors.vendorMicrosoft",
  imap: "connectors.provImap",
  test_mailbox: "connectors.provTestMailbox",
};

/** The name of the account one connection belongs to, so gcal and gmail both
 *  read "Google". */
export function accountNameOf(provider: Provider): MessageKey {
  if (isLoneProvider(provider)) {
    return labelFor(ACCOUNT_NAME, provider);
  }
  return ACCOUNT_NAME[vendorOf(provider) === "google" ? "google" : "microsoft"];
}
