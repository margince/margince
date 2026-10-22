// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import {
  type AccountVendor,
  VENDOR_SERVICES,
} from "../format/connectorprovider";
import { DEFAULT_IMPORT_WINDOW } from "./backfill-run";
import type { ConnectAccountInput, ImportWindow } from "./connect-account";

// What the connect dialog has been told so far, and the one request it turns
// into. Kept apart from the drawing so the request has a single spelling.

type Provider = components["schemas"]["CaptureConnection"]["provider"];
export type Availability = components["schemas"]["CaptureProviderAvailability"];

/** The account a reader connects: one of the two vendors, or any other mail
 *  host over IMAP, which brings mail alone. */
export type FlowAccount = AccountVendor | "imap";
export const FLOW_ACCOUNTS: readonly FlowAccount[] = [
  "google",
  "microsoft",
  "imap",
];
export type Service = "mail" | "calendar";
export type ImportPick = ImportWindow | "none";
export type Step = 1 | 2 | 3;

export type Draft = Readonly<{
  step: Step;
  account: FlowAccount | "";
  services: readonly Service[];
  importPick: ImportPick;
}>;

export function providerFor(
  account: FlowAccount,
  service: Service,
): Provider | null {
  if (account === "imap") {
    return service === "mail" ? "imap" : null;
  }
  return VENDOR_SERVICES[account][service];
}

/** The services this account can still bring in: a calendar never over IMAP,
 *  and nothing the roster already holds. */
export function openServices(
  account: FlowAccount,
  connected: readonly Provider[],
): Service[] {
  const services: readonly Service[] = ["mail", "calendar"];
  return services.filter((service) => {
    const provider = providerFor(account, service);
    return provider !== null && !connected.includes(provider);
  });
}

/** Whether this installation could not connect the account right now. The
 *  roster names mail providers only, and one it leaves out is not a refusal:
 *  a connect that is refused says so on its own. */
export function accountUnwired(
  account: FlowAccount,
  availability: readonly Availability[],
): boolean {
  const mail = providerFor(account, "mail");
  return availability.some(
    (row) => row.provider === mail && row.reason !== "ready",
  );
}

/** Whether the flow has anything left to offer this roster. */
export function canAddAccount(connected: readonly Provider[]): boolean {
  return FLOW_ACCOUNTS.some(
    (account) => openServices(account, connected).length > 0,
  );
}

export function draftFor(
  account: FlowAccount | "",
  connected: readonly Provider[],
): Draft {
  return {
    step: account === "" ? 1 : 2,
    account,
    services: account === "" ? [] : openServices(account, connected),
    importPick: DEFAULT_IMPORT_WINDOW,
  };
}

/** One consent round trip, mail first: the server chains the calendar after
 *  the mail grant, so a calendar alone is the only time it leads. */
export function connectInputFor(
  vendor: AccountVendor,
  services: readonly Service[],
  importPick: ImportPick,
): ConnectAccountInput | null {
  const pair = VENDOR_SERVICES[vendor];
  if (services.includes("mail")) {
    return {
      provider: pair.mail,
      returnTo: "settings",
      alsoConnect: services.includes("calendar") ? pair.calendar : undefined,
      importWindow: importPick === "none" ? undefined : importPick,
    };
  }
  return services.includes("calendar")
    ? { provider: pair.calendar, returnTo: "settings" }
    : null;
}
