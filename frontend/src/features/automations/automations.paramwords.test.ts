// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { de } from "../../i18n/de";
import { en, type MessageKey } from "../../i18n/en";
import { vi } from "../../i18n/vi";
import { WEBSITE_INQUIRY_RECIPE } from "../../lib/websiteinquiries";
import { TEMPLATE_WORDS } from "./automations.catalogwords";
import { AUTOMATION_CATALOG } from "./automations.fixtures";
import { paramFields, paramsFromValues } from "./automations.params";
import {
  missingParam,
  OPTION_LABELS,
  PARAM_LABELS,
} from "./automations.paramwords";

const backend = join(
  dirname(fileURLToPath(import.meta.url)),
  "../../../../backend/internal",
);

function goSources(dir: string): string[] {
  return readdirSync(dir)
    .filter((file) => file.endsWith(".go") && !file.endsWith("_test.go"))
    .map((file) => readFileSync(join(dir, file), "utf8"));
}

// The server's catalog keys, read from its Go. Every site must resolve, so a
// key this reader cannot follow fails here instead of going unseen.
function serverCatalogKeys(): { keys: string[]; sites: number } {
  const automation = goSources(join(backend, "modules/automation"));
  const provenance = new Map(
    goSources(join(backend, "shared/kernel/provenance")).flatMap((source) =>
      [...source.matchAll(/\b(\w+)\s*=\s*"([a-z_]+)"/g)].map(
        ([, name, value]) => [name, value] as const,
      ),
    ),
  );
  const constants = new Map<string, string>();
  for (const source of automation) {
    for (const [, name, value, borrowed] of source.matchAll(
      /\b(\w+)\s*=\s*(?:"([a-z_]+)"|provenance\.(\w+))/g,
    )) {
      const resolved = value ?? provenance.get(borrowed ?? "");
      if (name && resolved) constants.set(name, resolved);
    }
  }
  // A catalog key is a literal or a constant named for its handler (`…Name`).
  const sites = automation.flatMap((source) =>
    [...source.matchAll(/\b(?:Key:\s*|entry\()("[a-z_]+"|\w+Name\b)/g)].map(
      ([, site]) => site ?? "",
    ),
  );
  const keys = sites.flatMap((site) =>
    site.startsWith('"') ? site.slice(1, -1) : (constants.get(site) ?? []),
  );
  return { keys, sites: sites.length };
}

const CATALOGS = { en, de, vi } as const;

function wordedInEveryLanguage(key: MessageKey) {
  for (const [lang, catalog] of Object.entries(CATALOGS)) {
    expect(catalog[key].trim(), `${lang} ${key}`).not.toBe("");
  }
}

describe("the automation catalog's words", () => {
  const server = serverCatalogKeys();

  it("reads every catalog key the server declares", () => {
    expect(server.keys).toHaveLength(server.sites);
    expect(server.keys).toEqual(
      expect.arrayContaining(["renewal_reminder", "list_membership_task"]),
    );
  });

  it("has a fixture entry for every server catalog key", () => {
    const fixture = AUTOMATION_CATALOG.map((entry) => entry.key);
    expect(fixture).toEqual(expect.arrayContaining(server.keys));
  });

  it.each(serverCatalogKeys().keys)(
    "names and describes %s in every language",
    (key) => {
      const words = TEMPLATE_WORDS[key];
      expect(words, key).toBeDefined();
      if (!words) return;
      wordedInEveryLanguage(words.name);
      wordedInEveryLanguage(words.description);
      expect(de[words.description]).not.toBe(en[words.description]);
      expect(vi[words.description]).not.toBe(en[words.description]);
    },
  );

  // The website inquiry recipe has its own dialog, which words its own fields.
  const drawn = AUTOMATION_CATALOG.filter(
    (entry) => entry.key !== WEBSITE_INQUIRY_RECIPE,
  ).flatMap((entry) =>
    paramFields(entry.params_schema).map((field) => ({
      entry: entry.key,
      field,
    })),
  );

  it("walks the parameters the form draws", () => {
    expect(drawn.length).toBeGreaterThan(20);
  });

  it.each(drawn)(
    "labels $entry's $field.key in every language",
    ({ field }) => {
      const label = PARAM_LABELS[field.key];
      expect(label, field.key).toBeDefined();
      if (label) wordedInEveryLanguage(label);
      for (const option of field.options ?? []) {
        const word = OPTION_LABELS[field.key]?.[option];
        expect(word, `${field.key}=${option}`).toBeDefined();
        if (word) wordedInEveryLanguage(word);
      }
    },
  );
});

describe("an optional parameter", () => {
  const routing = AUTOMATION_CATALOG.find(
    (entry) => entry.key === "assign_lead_owner",
  );
  const fields = paramFields(routing?.params_schema ?? {});
  const t = (key: MessageKey) => en[key];

  it("is optional when the schema does not require it", () => {
    expect(
      fields.find((field) => field.key === "cap_per_owner")?.required,
    ).toBe(false);
    expect(missingParam(fields, { cap_per_owner: "" }, t)).toBeUndefined();
  });

  it("is left out when empty, and sent when set", () => {
    expect(paramsFromValues(fields, { cap_per_owner: "" })).toEqual({});
    expect(paramsFromValues(fields, { cap_per_owner: "5" })).toEqual({
      cap_per_owner: 5,
    });
  });
});
