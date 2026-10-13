// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

type Provider = components["schemas"]["CaptureConnection"]["provider"];

/** The providers that connect a MAILBOX.
 *
 *  The other two — `gcal` and `graphcal` — connect a calendar, and on this
 *  screen the difference decides what a row may say about itself. A calendar
 *  has no mail history to import backward from a date, no posture to take
 *  towards mail it does not carry, and no signature block to read a contact
 *  out of. Every one of those rows was drawn against a calendar anyway, under
 *  an envelope icon and beside the member's own email address, and the import
 *  card answered "this mailbox type can't be backfilled" — which a member read
 *  as the product refusing to import their mail.
 *
 *  A DELIBERATE MIRROR of the server's `compose.MailProviders`, which refuses
 *  those same operations for a calendar, held equal by
 *  backend/gates/frontendmailproviders_test.go. Two answers to "is this a
 *  mailbox" is how a screen offers what a server refuses. */
export const MAIL_PROVIDERS: ReadonlySet<Provider> = new Set<Provider>([
  "gmail",
  "graph",
  "imap",
]);

/** Whether this connection is a mailbox rather than a calendar. */
export function isMailbox(provider: Provider): boolean {
  return MAIL_PROVIDERS.has(provider);
}

/** Whether this connection's row should draw the mail icon.
 *
 *  test_mailbox is deliberately outside MAIL_PROVIDERS/isMailbox — it has no
 *  "Add a connection" button, no signature enrichment, and no backfill (there
 *  is no history to import), so the rows those gate stay off for it exactly
 *  as they do for a calendar. But it is still a MAILBOX, not a calendar, and
 *  drawing it under CalendarDays reads as a second, wrong-shaped connection
 *  type nobody chose. */
export function isMailIcon(provider: Provider): boolean {
  return isMailbox(provider) || provider === "test_mailbox";
}

/** The two vendors whose mail and calendar are separate grants of one account. */
export type AccountVendor = "google" | "microsoft";

/** Each vendor's mailbox and calendar, the same pairs the server chains with
 *  `also_connect` (gmail then gcal, graph then graphcal). */
export const VENDOR_SERVICES: Readonly<
  Record<
    AccountVendor,
    Readonly<{ mail: "gmail" | "graph"; calendar: "gcal" | "graphcal" }>
  >
> = {
  google: { mail: "gmail", calendar: "gcal" },
  microsoft: { mail: "graph", calendar: "graphcal" },
};

/** The vendor a provider belongs to, or null for IMAP, the test mailbox and
 *  any string that is not a provider (a route segment, say). */
export function vendorOf(provider: string | undefined): AccountVendor | null {
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
 *  calendar. IMAP posts credentials and gets a connection back, never a URL. */
export type OAuthConnectProvider = (typeof VENDOR_SERVICES)[AccountVendor][
  | "mail"
  | "calendar"];

/** Narrows any string (a route segment, a stored value) to an OAuth provider
 *  by membership, never by assertion. */
export function asOAuthConnectProvider(
  value: string | undefined,
): OAuthConnectProvider | null {
  const vendor = vendorOf(value);
  if (vendor === null) return null;
  const { mail, calendar } = VENDOR_SERVICES[vendor];
  return value === mail ? mail : calendar;
}

type CaptureConnection = components["schemas"]["CaptureConnection"];

/** A provider that pairs with no other: IMAP and the test mailbox carry no
 *  calendar, so each is an account of its own. */
type LoneProvider = Exclude<Provider, OAuthConnectProvider>;

/** One account as the member thinks of it: a mailbox, a calendar, or both. */
export type ConnectionAccount = Readonly<{
  key: AccountVendor | LoneProvider;
  mail: CaptureConnection | null;
  calendar: CaptureConnection | null;
}>;

/** The roster folded into accounts, Google first, then Microsoft 365, then the
 *  rest in roster order. A calendar with no mailbox beside it stays an account.
 *
 *  Takes the rows the caller shows: a disconnected gcal left in the input
 *  would read as a connected calendar, so the caller filters those out first. */
export function accountsOf(
  connections: readonly CaptureConnection[],
): ConnectionAccount[] {
  const vendors: readonly AccountVendor[] = ["google", "microsoft"];
  const paired = vendors.map((vendor) => ({
    key: vendor,
    mail: rowFor(connections, VENDOR_SERVICES[vendor].mail),
    calendar: rowFor(connections, VENDOR_SERVICES[vendor].calendar),
  }));
  const lone = connections.flatMap((connection) => {
    const provider = connection.provider;
    return isLoneProvider(provider)
      ? [{ key: provider, mail: connection, calendar: null }]
      : [];
  });
  return [...paired, ...lone].filter(
    (account) => account.mail !== null || account.calendar !== null,
  );
}

function rowFor(
  connections: readonly CaptureConnection[],
  provider: Provider,
): CaptureConnection | null {
  return connections.find((c) => c.provider === provider) ?? null;
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
    return ACCOUNT_NAME[provider];
  }
  const { mail, calendar } = VENDOR_SERVICES.google;
  const isGoogle = provider === mail || provider === calendar;
  return ACCOUNT_NAME[isGoogle ? "google" : "microsoft"];
}
