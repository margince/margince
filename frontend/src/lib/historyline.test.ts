// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { type Locale, translate } from "../i18n";
import { de } from "../i18n/de";
import { en, type MessageKey } from "../i18n/en";
import { vi } from "../i18n/vi";
import {
  type HistoryLineFacts,
  historyLine,
  historySubject,
} from "./historyline";
import { HISTORY_VERB } from "./historyverbs";

type AuditHistoryEntry = components["schemas"]["AuditHistoryEntry"];

const CATALOGS: Record<Locale, Record<MessageKey, string>> = { en, de, vi };
const LOCALES: readonly Locale[] = ["en", "de", "vi"];
const PASSPORT = "0199a000-0000-7000-8000-000000000001";

function fill(template: string, params: Record<string, string>): string {
  return Object.entries(params).reduce(
    (text, [name, value]) => text.replace(`{${name}}`, value),
    template,
  );
}

const base: AuditHistoryEntry = {
  id: "h1",
  actor_type: "human",
  actor_id: "human:u1",
  action: "update",
  occurred_at: "2026-07-14T10:00:00Z",
  summary: "the server's English sentence",
};

const SHAPES: ReadonlyArray<{
  shape: string;
  entry: Partial<AuditHistoryEntry>;
  subject:
    | {
        key: MessageKey;
        params?: Record<string, string>;
        // A name that is itself copy, read in the locale under test.
        nameKey?: MessageKey;
      }
    | string;
}> = [
  {
    shape: "a human edit",
    entry: { actor_name: "Uma Underwriter" },
    subject: "Uma Underwriter",
  },
  {
    shape: "a member whose seat no longer resolves",
    entry: { actor_name: null },
    subject: { key: "audit.unknownMember" },
  },
  {
    shape: "a human edit made with a passport",
    entry: { actor_name: "Uma Underwriter", passport_id: PASSPORT },
    subject: "Uma Underwriter",
  },
  {
    shape: "an agent on behalf of a member via a named client",
    entry: {
      actor_type: "agent",
      actor_id: "agent:p1",
      passport_id: PASSPORT,
      on_behalf_of_name: "Ada Authority",
      agent_client: "Claude",
    },
    subject: {
      key: "history.subject.viaClient",
      params: { name: "Ada Authority", client: "Claude" },
    },
  },
  {
    shape: "an agent on behalf of a member via an unnamed client",
    entry: {
      actor_type: "agent",
      actor_id: "agent:p1",
      passport_id: PASSPORT,
      on_behalf_of_name: "Ada Authority",
    },
    subject: {
      key: "history.subject.viaAgent",
      params: { name: "Ada Authority" },
    },
  },
  {
    shape: "an agent on behalf of a member whose seat no longer resolves",
    entry: {
      actor_type: "agent",
      actor_id: "agent:p1",
      passport_id: PASSPORT,
      on_behalf_of: "u-gone",
      on_behalf_of_name: null,
    },
    subject: {
      key: "history.subject.viaAgent",
      nameKey: "audit.unknownMember",
    },
  },
  {
    shape: "a connector on behalf of a member",
    entry: {
      actor_type: "connector",
      actor_id: "connector:gmail",
      on_behalf_of_name: "Ada Authority",
    },
    subject: {
      key: "history.subject.viaConnector",
      params: { name: "Ada Authority" },
    },
  },
  {
    shape: "a machine with a passport and no recorded authority",
    entry: {
      actor_type: "agent",
      actor_id: "agent:p1",
      passport_id: PASSPORT,
      on_behalf_of_name: "",
    },
    subject: { key: "history.subject.unrecordedAuthority" },
  },
  {
    shape: "an agent with a named client and no recorded authority",
    entry: {
      actor_type: "agent",
      actor_id: "agent:p1",
      passport_id: PASSPORT,
      agent_client: "Claude",
    },
    subject: { key: "history.subject.unrecordedAuthority" },
  },
  {
    shape: "the system with a passport",
    entry: { actor_type: "system", actor_id: "system", passport_id: PASSPORT },
    subject: { key: "history.subject.unrecordedAuthority" },
  },
  {
    shape: "the system",
    entry: { actor_type: "system", actor_id: "system" },
    subject: { key: "history.subject.system" },
  },
  {
    shape: "an agent with no passport",
    entry: { actor_type: "agent", actor_id: "agent:ext-job" },
    subject: { key: "history.subject.agent" },
  },
  {
    shape: "a connector with no granting member",
    entry: { actor_type: "connector", actor_id: "connector:finance" },
    subject: { key: "history.subject.connector" },
  },
  {
    shape: "a Deal Room buyer whose name no longer resolves",
    entry: { actor_type: "buyer", actor_id: "buyer:b1" },
    subject: { key: "history.subject.buyer" },
  },
  {
    shape: "a named Deal Room buyer",
    entry: {
      actor_type: "buyer",
      actor_id: "buyer:b1",
      actor_name: "Bea Buyer",
    },
    subject: {
      key: "history.subject.buyerNamed",
      params: { name: "Bea Buyer" },
    },
  },
];

const VERBS = [
  { action: "update", key: "history.verb.update" },
  { action: "archive", key: "history.verb.archive" },
] as const;

describe("historyLine", () => {
  for (const locale of LOCALES) {
    const t = (key: MessageKey, params?: Record<string, string>) =>
      translate(locale, key, params);
    for (const { shape, entry, subject } of SHAPES) {
      for (const verb of VERBS) {
        it(`phrases ${shape} that ran ${verb.action} in ${locale}`, () => {
          const row = { ...base, ...entry, action: verb.action };
          const expectedSubject =
            typeof subject === "string"
              ? subject
              : fill(CATALOGS[locale][subject.key], {
                  ...subject.params,
                  ...(subject.nameKey && {
                    name: CATALOGS[locale][subject.nameKey],
                  }),
                });
          const expected = fill(CATALOGS[locale][verb.key], {
            subject: expectedSubject,
          });

          expect(historySubject(row, t)).toBe(expectedSubject);
          expect(historyLine(row, t)).toBe(expected);
          expect(historyLine(row, t)).not.toBe(row.summary);
        });
      }
    }
  }

  it.each<[Locale, Partial<AuditHistoryEntry>, string]>([
    [
      "en",
      {
        actor_type: "agent",
        actor_id: "agent:p1",
        passport_id: PASSPORT,
        on_behalf_of: "u-ada",
        on_behalf_of_name: "Ada Authority",
        action: "update",
      },
      "Ada Authority, via an agent, updated the record",
    ],
    [
      "de",
      {
        actor_type: "agent",
        actor_id: "agent:p1",
        passport_id: PASSPORT,
        action: "archive",
      },
      "Ein automatisierter Vorgang (keine menschliche Autorisierung erfasst) hat den Datensatz archiviert",
    ],
    [
      "vi",
      {
        actor_type: "buyer",
        actor_id: "buyer:b1",
        actor_name: "Bea Buyer",
        action: "update",
      },
      "Bea Buyer (trong Deal Room) đã cập nhật hồ sơ",
    ],
  ])("writes the %s line out in full", (locale, entry, sentence) => {
    const row: AuditHistoryEntry = { ...base, ...entry };
    expect(
      historyLine(row, (key, params) => translate(locale, key, params)),
    ).toBe(sentence);
  });

  it("gives every verb exactly one subject and no other placeholder", () => {
    for (const locale of LOCALES) {
      for (const key of Object.values(HISTORY_VERB)) {
        const placeholders = CATALOGS[locale][key].match(/\{\w+\}/g);
        expect(placeholders, `${locale} ${key}`).toEqual(["{subject}"]);
      }
    }
  });

  it("keeps the server's sentence for a link change", () => {
    const row: AuditHistoryEntry = {
      ...base,
      action: "create",
      summary: "Ada Admin linked Employer GmbH as cto",
      edge: {
        kind: "employment",
        other_entity_type: "company",
        other_entity_id: "c1",
        other_label: "Employer GmbH",
      },
    };
    expect(
      historyLine(row, (key, params) => translate("de", key, params)),
    ).toBe(row.summary);
  });

  it("keeps the server's sentence for a verb or actor kind it does not know", () => {
    const t = (key: MessageKey, params?: Record<string, string>) =>
      translate("vi", key, params);
    const newerVerb: HistoryLineFacts = { ...base, action: "transmogrify" };
    const newerActor: HistoryLineFacts = { ...base, actor_type: "robot" };

    expect(historyLine(newerVerb, t)).toBe(base.summary);
    expect(historyLine(newerActor, t)).toBe(base.summary);
  });
});
