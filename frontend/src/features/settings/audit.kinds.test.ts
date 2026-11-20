// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { de } from "../../i18n/de";
import { en, type MessageKey } from "../../i18n/en";
import { vi } from "../../i18n/vi";
import { auditKindKey } from "./audit";

// No single owner lists the record types the audit log names: the contract
// types `entity_type` as free text, and each storekit.Audit* call site picks
// its own. So the corpus is read from those call sites in the Go source.

const root = join(
  dirname(fileURLToPath(import.meta.url)),
  "../../../../backend",
);
const MODULE = "github.com/margince/margince/backend/";
const WRITERS =
  /\bstorekit\.(?:Audit|AuditWithEvidence|AuditEvent|AuditEventWithEvidence|AuditEach|AuditWithTrail)\(/g;

function goFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory())
      return name === "testdata" ? [] : goFiles(path);
    return name.endsWith(".go") && !name.endsWith("_test.go") ? [path] : [];
  });
}

// The fourth argument of a call, read by bracket depth so a nested call or a
// map literal before it does not shift it.
function fourthArgument(source: string, from: number): string {
  const args: string[] = [];
  let depth = 1;
  let current = "";
  let quoted = false;
  for (let i = from; i < source.length && depth > 0 && args.length < 4; i++) {
    const char = source.charAt(i);
    if (quoted) {
      quoted = !(char === '"' && source.charAt(i - 1) !== "\\");
    } else if (char === '"') {
      quoted = true;
    } else if ("([{".includes(char)) {
      depth++;
    } else if (")]}".includes(char)) {
      depth--;
    }
    if (depth === 1 && !quoted && char === ",") {
      args.push(current.trim());
      current = "";
    } else {
      current += char;
    }
  }
  if (args.length < 4) args.push(current.trim());
  return (args[3] ?? "").replace(/^string\((.*)\)$/, "$1");
}

function auditedEntityTypes(): { types: Set<string>; dynamic: string[] } {
  const files = goFiles(join(root, "internal"));
  const sources = new Map(
    files.map((file) => [file, readFileSync(file, "utf8")]),
  );
  const declared = new Map<string, Map<string, string>>();
  for (const [file, source] of sources) {
    const dir = dirname(file);
    const names = declared.get(dir) ?? new Map<string, string>();
    declared.set(dir, names);
    for (const [, name, value] of source.matchAll(
      /^\s*(?:const\s+)?(\w+)(?:\s+[\w.]+)?\s*=\s*("[a-z_]+"|[\w.]+)\s*(?:\/\/.*)?$/gm,
    )) {
      if (name && value) names.set(name, value);
    }
  }
  const resolve = (
    dir: string,
    imports: Map<string, string>,
    raw: string,
    hops = 0,
  ): string | null => {
    if (/^"[a-z_]+"$/.test(raw)) return raw.slice(1, -1);
    if (hops > 4) return null;
    const [first, second] = raw.split(".");
    if (second === undefined) {
      const value = declared.get(dir)?.get(raw);
      return value ? resolve(dir, imports, value, hops + 1) : null;
    }
    const target = imports.get(first ?? "");
    const value = target ? declared.get(target)?.get(second) : undefined;
    return target && value ? resolve(target, imports, value, hops + 1) : null;
  };
  const types = new Set<string>();
  const dynamic: string[] = [];
  for (const [file, source] of sources) {
    const imports = new Map(
      [...source.matchAll(/^\s*(?:(\w+)\s+)?"([^"]+)"\s*$/gm)].flatMap(
        ([, alias, path = ""]) =>
          path.startsWith(MODULE)
            ? [
                [
                  alias ?? path.split("/").at(-1) ?? path,
                  join(root, path.slice(MODULE.length)),
                ] as const,
              ]
            : [],
      ),
    );
    for (const call of source.matchAll(WRITERS)) {
      const raw = fourthArgument(source, (call.index ?? 0) + call[0].length);
      const type = resolve(dirname(file), imports, raw);
      if (type) types.add(type);
      else dynamic.push(`${relative(root, file)}: ${raw}`);
    }
  }
  return { types, dynamic };
}

const RUNTIME_TYPED_WRITERS: Record<string, number> = {
  "internal/compose/extledger.go": 2,
  "internal/modules/capture/lifecycleaudit.go": 1,
  "internal/modules/consent/authorizebasis.go": 1,
  "internal/modules/consent/bouncesuppress.go": 1,
  "internal/modules/consent/lawfulbasiswarning.go": 1,
  "internal/modules/consent/lift.go": 1,
  "internal/modules/consent/overridecarry.go": 1,
  "internal/modules/consent/qualifyingevent.go": 2,
  "internal/modules/consent/recordadmitted.go": 1,
  "internal/modules/consent/restrictionlift.go": 1,
  "internal/modules/consent/satellitecarry.go": 1,
  "internal/modules/consent/stopcarry.go": 1,
  "internal/modules/consent/suppress.go": 1,
  "internal/modules/consent/withdrawalpress.go": 1,
  "internal/modules/contacts/company_evidence_write.go": 3,
  "internal/modules/contacts/mergekindid.go": 1,
  "internal/modules/finance/linkbackfill.go": 1,
  "internal/modules/identity/grantmerge.go": 3,
  "internal/modules/identity/grants.go": 2,
  "internal/modules/identity/ownercarry.go": 1,
  "internal/modules/privacy/erasure_collateral.go": 1,
  "internal/modules/privacy/erasure_extension.go": 1,
  "internal/modules/privacy/retentionactions.go": 1,
  "internal/modules/recordlinks/write.go": 1,
  "internal/modules/reporting/persistence.go": 1,
  "internal/platform/settings/delete.go": 1,
  "internal/platform/settings/store.go": 1,
};

const CATALOGS = { en, de, vi } as const;

describe("the audit log's record types", () => {
  const { types, dynamic } = auditedEntityTypes();

  it("reads the types the writers name", () => {
    expect(types).toContain("data_subject_request");
    expect(types).toContain("privacy_request_proof");
  });

  // A call site whose type is a runtime value hides that type from this test.
  // So a new one fails here until someone checks the types it can write.
  it("knows every call site whose type it cannot read", () => {
    const counts: Record<string, number> = {};
    for (const site of dynamic) {
      const file = site.slice(0, site.indexOf(":"));
      counts[file] = (counts[file] ?? 0) + 1;
    }
    expect(counts).toEqual(RUNTIME_TYPED_WRITERS);
  });

  it.each([...types].sort())("words %s in every language", (type) => {
    const key: MessageKey | undefined = auditKindKey(type);
    expect(key, type).toBeDefined();
    if (!key) return;
    for (const [lang, catalog] of Object.entries(CATALOGS)) {
      expect(catalog[key].trim(), `${lang} ${type}`).not.toBe("");
    }
  });
});
