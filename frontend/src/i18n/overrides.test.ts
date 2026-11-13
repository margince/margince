import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  overrideProblems,
  shippedOverrideProblems,
} from "../testing/overrides.testkit";
import { de } from "./de";

vi.mock("@composition/copy", () => ({
  extensionCopy: { en: { "extAcme.title": "Acme" } },
  extensionOverrides: {
    en: { "nav.contacts": "Customers", "worklist.relationships.optional": "{q}" },
  },
}));

const { translate } = await import("./index");

describe("translate() with overrides", () => {
  it("reads the override before the core catalogue", () => {
    expect(translate("en", "nav.contacts")).toBe("Customers");
  });

  it("falls back to core for a locale the override does not cover", () => {
    expect(translate("de", "nav.contacts")).toBe(de["nav.contacts"]);
  });

  it("still fills placeholders in an override", () => {
    expect(translate("en", "worklist.relationships.optional", { q: "x" })).toBe(
      "x",
    );
  });

  it("still reads a unit's own copy for a key core does not have", () => {
    expect(translate("en", "extAcme.title")).toBe("Acme");
  });
});

describe("overrideProblems", () => {
  it("refuses an override that drops a placeholder core has", () => {
    expect(overrideProblems("en", { "users.editTitle": "Edit" })).toEqual([
      "en users.editTitle: drops {name}",
    ]);
  });

  it("refuses an override that omits a plural form core has", () => {
    expect(
      overrideProblems("en", {
        "homeRepair.normalizeSummary_one": "{count} task",
      }),
    ).toEqual([
      "en homeRepair.normalizeSummary_one: omits plural form homeRepair.normalizeSummary_other",
    ]);
  });

  it("accepts a complete override", () => {
    expect(
      overrideProblems("en", {
        "homeRepair.normalizeSummary_one": "{count} task",
        "homeRepair.normalizeSummary_other": "{count} tasks",
      }),
    ).toEqual([]);
  });
});

describe("the override files extensions ship", () => {
  const roots: string[] = [];
  afterEach(() => {
    for (const root of roots.splice(0)) rmSync(root, { recursive: true });
  });

  // An extensions tree with one unit's override file, laid out as on disk.
  const treeWith = (file: string, body: object): string => {
    const root = mkdtempSync(join(tmpdir(), "overrides-"));
    roots.push(root);
    const layer = join(root, "acme", "frontend", "i18n-overrides");
    mkdirSync(layer, { recursive: true });
    writeFileSync(join(layer, file), JSON.stringify(body));
    return root;
  };

  it("fails on a file that drops a placeholder", () => {
    const root = treeWith("de.json", { "users.editTitle": "Bearbeiten" });
    expect(shippedOverrideProblems(root)).toEqual([
      "de users.editTitle: drops {name}",
    ]);
  });

  it("fails on a file that omits a plural form", () => {
    const root = treeWith("en.json", {
      "homeRepair.normalizeSummary_other": "{count} tasks",
    });
    expect(shippedOverrideProblems(root)).toHaveLength(1);
  });

  it("passes a file that keeps both", () => {
    const root = treeWith("en.json", { "users.editTitle": "Edit {name}" });
    expect(shippedOverrideProblems(root)).toEqual([]);
  });

  it("holds for every extension in this tree", () => {
    expect(
      shippedOverrideProblems(
        resolve(import.meta.dirname, "../../../extensions"),
      ),
    ).toEqual([]);
  });
});
