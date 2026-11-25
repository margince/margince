// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { translate } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { fieldChangeKeys, fieldTarget } from "./customfields.changes";

type AuditLogEntry = components["schemas"]["AuditLogEntry"];
type CustomField = components["schemas"]["CustomField"];

const t = (key: MessageKey, params?: Record<string, string>) =>
  translate("en", key, params);

const entry = (over: Partial<AuditLogEntry> = {}): AuditLogEntry => ({
  id: "a-1",
  actor_type: "human",
  actor_id: "u1",
  action: "update",
  entity_type: "custom_field",
  entity_id: "f-1",
  occurred_at: "2026-06-22T14:09:00Z",
  ...over,
});

const update = (before: object, after: object) =>
  entry({ before: { ...before }, after: { ...after } });

const field = (over: Partial<CustomField> = {}): CustomField => ({
  id: "f-1",
  object: "contact",
  label: "Budget",
  slug: "budget",
  type: "number",
  status: "active",
  column_name: "cf_budget",
  show_in_create: true,
  reportable: false,
  created_by: "u1",
  created_at: "2026-06-22T14:09:00Z",
  updated_at: "2026-06-22T14:09:00Z",
  ...over,
});

const words = (one: AuditLogEntry) => fieldChangeKeys(one).map((key) => t(key));

describe("fieldChangeKeys", () => {
  it("reads a create as created, whatever settings it wrote", () => {
    const created = entry({
      action: "create",
      after: { object: "deal", label: "Budget", show_in_create: true },
    });
    expect(words(created)).toEqual(["Created"]);
  });

  it.each([
    [{ label: "Budget" }, { label: "Spend" }, "Renamed"],
    [
      { label_translations: {} },
      { label_translations: { de: "Etat" } },
      "Translations changed",
    ],
    [
      { option_translations: {} },
      { option_translations: { Gold: { de: "Gold" } } },
      "Translations changed",
    ],
    [
      { show_in_create: true },
      { show_in_create: false },
      "Hidden from create forms",
    ],
    [
      { show_in_create: false },
      { show_in_create: true },
      "Shown in create forms",
    ],
    [{ reportable: false }, { reportable: true }, "Added to reports"],
    [{ reportable: true }, { reportable: false }, "Removed from reports"],
    [
      { activity_kinds: [] },
      { activity_kinds: ["call"] },
      "Activity types changed",
    ],
  ])("reads %j becoming %j as %s", (before, after, word) => {
    expect(words(update(before, after))).toEqual([word]);
  });

  // The archive write also clears the reports switch, which is not a second act.
  it("reads a field retired out of reports as archived alone", () => {
    const archived = update(
      { status: "active", reportable: true },
      { status: "retired", reportable: false },
    );
    expect(words(archived)).toEqual(["Archived"]);
  });

  // New choices carry their translations along in the same entry.
  it("reads new choices as choices, not as translations", () => {
    const choices = update(
      { options: ["Gold"], option_translations: {} },
      { options: ["Gold", "Silver"], option_translations: {} },
    );
    expect(words(choices)).toEqual(["Choices changed"]);
  });

  it("names each setting one write moved", () => {
    const both = update(
      { label: "Budget", show_in_create: true },
      { label: "Spend", show_in_create: false },
    );
    expect(words(both)).toEqual(["Renamed", "Hidden from create forms"]);
  });

  it("names nothing for a setting it does not know", () => {
    expect(
      fieldChangeKeys(update({ colour: "red" }, { colour: "blue" })),
    ).toEqual([]);
  });
});

describe("fieldTarget", () => {
  it("names the record type and the field's current label from the catalog", () => {
    const hidden = update({ show_in_create: true }, { show_in_create: false });
    expect(fieldTarget(hidden, field(), "Budget", t, "en")).toBe(
      "Contact · Budget",
    );
  });

  it("prefers the current label to the one a rename wrote", () => {
    const renamed = update({ label: "Budget" }, { label: "Spend" });
    expect(
      fieldTarget(renamed, field({ label: "Cost" }), "Spend", t, "en"),
    ).toBe("Contact · Cost");
  });

  it("marks a field that is retired now", () => {
    const retired = field({ object: "deal", status: "retired" });
    expect(fieldTarget(entry(), retired, "Budget", t, "en")).toBe(
      "Deal · Budget (retired)",
    );
  });

  it("reads the record type and label a create wrote when the catalog lacks the field", () => {
    const created = entry({
      action: "create",
      after: { object: "company", label: "Budget" },
    });
    expect(fieldTarget(created, undefined, "Budget", t, "en")).toBe(
      "Company · Budget",
    );
  });

  it("names nothing when neither the catalog nor the entry names the field", () => {
    const hidden = update({ show_in_create: true }, { show_in_create: false });
    expect(fieldTarget(hidden, undefined, undefined, t, "en")).toBeUndefined();
  });
});
