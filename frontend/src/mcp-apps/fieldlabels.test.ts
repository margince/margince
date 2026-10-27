import { describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import { FIELD_LABELS, fieldLabel, isID } from "./fieldlabels";

const PREFIX = "history.field.";

const catalog = Object.fromEntries(
  Object.entries(en)
    .filter(([key]) => key.startsWith(PREFIX))
    .map(([key, label]) => [key.slice(PREFIX.length), label]),
);

describe("a view names a field the way the history panel does", () => {
  it("holds every history.field label, word for word", () => {
    expect(Object.keys(catalog).length).toBeGreaterThan(0);
    expect(FIELD_LABELS).toEqual(catalog);
  });

  it("names a field the catalog holds by its label", () => {
    expect(fieldLabel("title")).toBe("Job title");
    expect(fieldLabel("owner_id")).toBe("Owner");
  });

  it("names any other field in words rather than as its column", () => {
    expect(fieldLabel("cf_renewal_quarter")).toBe("Renewal quarter");
    expect(fieldLabel("champion_contact_id")).toBe("Champion contact");
  });

  it("tells a record id from text a reader can use", () => {
    expect(isID("0195c3a0-0000-7000-8000-000000000003")).toBe(true);
    expect(isID("Head of Sales")).toBe(false);
    expect(isID(42)).toBe(false);
  });
});
