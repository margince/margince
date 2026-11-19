// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { identifierNumber } from "../../format/format";
import type { Locale } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { looksLikeEmail } from "../../lib/emailshape";
import { inRange, PORT_RANGE } from "../../lib/imapconnect";
import {
  type FieldLabel,
  type FieldLabels,
  fieldCheckSentence,
} from "../../lib/problem";

type ConnectRequest = components["schemas"]["ConnectSharedMailboxRequest"];
export type Provider = ConnectRequest["provider"];

// The connect form's fields, each by the wire path a refusal names it with.
export const FIELD_LABEL = {
  address: "sharedMailboxes.address",
  reader_team_id: "sharedMailboxes.readerTeam",
  "imap.host": "connectors.imapHost",
  "imap.port": "connectors.imapPort",
  "imap.username": "connectors.imapUsername",
  "imap.secret": "connectors.imapSecret",
  "graph.tenant_id": "sharedMailboxes.graphTenant",
  "graph.client_id": "sharedMailboxes.graphClient",
  "graph.client_secret": "sharedMailboxes.graphSecret",
  "gmail.service_account_key": "sharedMailboxes.gmailKey",
} as const satisfies Record<string, MessageKey>;

export type FieldKey = keyof typeof FIELD_LABEL;
export type Values = Readonly<Partial<Record<FieldKey, string>>>;

export const PROVIDER_FIELDS: Record<Provider, readonly FieldKey[]> = {
  imap: ["imap.host", "imap.port", "imap.username", "imap.secret"],
  graph: ["graph.tenant_id", "graph.client_id", "graph.client_secret"],
  gmail: ["gmail.service_account_key"],
};

export const SECRETS: ReadonlySet<FieldKey> = new Set([
  "imap.secret",
  "graph.client_secret",
]);

export function fieldLabels(t: (key: MessageKey) => string): FieldLabels {
  const labels: Record<string, FieldLabel> = {};
  for (const [key, label] of Object.entries(FIELD_LABEL)) {
    labels[key] = t(label);
  }
  labels["imap.port"] = {
    label: t(FIELD_LABEL["imap.port"]),
    limit: (wire) => identifierNumber(Number(wire)),
  };
  return labels;
}

// The wire code the server would refuse a value with, or null for one it takes.
function valueFault(
  key: FieldKey,
  raw: string,
): Readonly<{ code: string; params?: Record<string, string> }> | null {
  const value = raw.trim();
  if (value === "") return { code: "required" };
  if (key === "address" && !looksLikeEmail(value)) {
    return { code: "email_malformed" };
  }
  if (key !== "imap.port") return null;
  const port = Number(value);
  if (!Number.isInteger(port)) return { code: "not_integer" };
  return inRange(port, PORT_RANGE)
    ? null
    : {
        code: "out_of_range",
        params: { min: String(PORT_RANGE.min), max: String(PORT_RANGE.max) },
      };
}

export function checkedFields(
  provider: Provider,
  values: Values,
  labels: FieldLabels,
  locale: Locale,
): ReadonlyMap<FieldKey, string> {
  const keys: readonly FieldKey[] = [
    "address",
    "reader_team_id",
    ...PROVIDER_FIELDS[provider],
  ];
  const faults = new Map<FieldKey, string>();
  for (const key of keys) {
    const fault = valueFault(key, values[key] ?? "");
    if (fault) {
      faults.set(
        key,
        fieldCheckSentence(fault.code, locale, labels[key], fault.params),
      );
    }
  }
  return faults;
}

export function credentialsOf(
  provider: Provider,
  values: Values,
): Partial<ConnectRequest> {
  const value = (key: FieldKey) => values[key] ?? "";
  switch (provider) {
    case "imap":
      return {
        imap: {
          host: value("imap.host").trim(),
          port: Number(value("imap.port").trim()),
          username: value("imap.username").trim(),
          secret: value("imap.secret"),
          mailbox: "INBOX",
          max_messages: 50,
        },
      };
    case "graph":
      return {
        graph: {
          tenant_id: value("graph.tenant_id").trim(),
          client_id: value("graph.client_id").trim(),
          client_secret: value("graph.client_secret"),
        },
      };
    case "gmail":
      return {
        gmail: { service_account_key: value("gmail.service_account_key") },
      };
    default:
      return {};
  }
}
