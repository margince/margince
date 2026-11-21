import { describe, expect, it } from "vitest";
import { stable } from "../format/collate";
import { en } from "../i18n/en";
import { fieldsARecordUpdateWrites } from "../testing/updatebodies";
import {
  historyFieldLabel,
  historyFieldLabelKey,
  historyFieldLabelled,
  syntheticAuditFieldLabelled,
} from "./historyfieldlabels";

// The coverage census: every wire name a history entry can show has a word,
// and every word answers a field that exists.
//
// A missing key fails nowhere else — the label function falls back to the
// column with its underscores spaced out, which renders "amount minor" at a
// reader and looks deliberate. So the obligation is DERIVED rather than
// listed. Two derivations meet here, both out of `schema.d.ts`, which is
// generated from the contract: the record types this projection serves are the
// `entity_type` enum on `FieldHistoryEntry`, and the fields an update writes
// are the properties of each type's `Update<Type>Request` body. Add a field
// upstream, forget the label, and this fails by name.
//
// It reads the generated types rather than the six Go stores for the same
// reason the client does: the contract is the seam between the two trees.

describe("history field labels", () => {
  // Under-recognition is the one way this gate could break in silence: a
  // narrowed walk reads a smaller contract, finds nothing to check and passes.
  // So the corpus is proved to be a real one before it is used.
  it("finds every record type's update body in the contract", () => {
    const byType = fieldsARecordUpdateWrites();
    expect([...byType.keys()].sort(stable)).toEqual([
      "activity",
      "company",
      "contact",
      "deal",
      "lead",
      "project",
    ]);
    expect(byType.get("deal")).toContain("amount_minor");
    for (const [recordType, fields] of byType) {
      expect(fields.length, recordType).toBeGreaterThan(0);
    }
  });

  it("has a word for every field a record update writes", () => {
    const unnamed = [...fieldsARecordUpdateWrites().values()]
      .flat()
      .filter((field) => historyFieldLabelKey(field) === undefined)
      .sort(stable);
    expect(unnamed, `fields with no label key: ${unnamed.join(", ")}`).toEqual(
      [],
    );
  });

  // The other direction. A word for a field no update writes any more is a
  // string nobody can reach and nobody will delete, and it is also how a
  // renamed column keeps looking covered while the new spelling renders raw.
  it("has no word for a field no record update writes", () => {
    const written = new Set([...fieldsARecordUpdateWrites().values()].flat());
    const orphans = historyFieldLabelled()
      .filter((field) => !written.has(field))
      .sort(stable);
    expect(
      orphans,
      `labels for unwritten fields: ${orphans.join(", ")}`,
    ).toEqual([]);
  });

  it("names the columns a deal's probability is stored under", () => {
    expect(historyFieldLabel("win_probability_override", (k) => en[k])).toBe(
      en["history.field.win_probability_override"],
    );
    expect(historyFieldLabel("win_probability_set_by", (k) => en[k])).toBe(
      en["history.field.win_probability_set_by"],
    );
  });

  it("names each stored address column with the Address dialog's word", () => {
    expect(historyFieldLabel("address_line1", (k) => en[k])).toBe(
      "Street and number",
    );
    expect(historyFieldLabel("address_postal_code", (k) => en[k])).toBe(
      "Postal code",
    );
  });

  it("names a key the catalogs actually carry", () => {
    const missing = historyFieldLabelled()
      .map((field) => historyFieldLabelKey(field))
      .filter((key) => key !== undefined && !(key in en));
    expect(missing).toEqual([]);
  });

  // A workspace's own column has no label in any catalog this build ships —
  // its name lives in the custom-field catalog — so the fallback is what a
  // reader gets, and it must still read as words.
  it("spaces out a column it has no word for", () => {
    expect(historyFieldLabel("cf_renewal_owner", (key) => key)).toBe(
      "cf renewal owner",
    );
  });
});

// Some AuditEvent writers name a fact with no updatable contract column at
// all, so the contract-derived census above never sees a gap in their own
// vocabulary — the field would otherwise render raw on a screen a compliance
// reviewer reads to answer "what happened to this record, and when".
describe("synthetic AuditEvent field labels", () => {
  // Never in the contract-derived set, in EITHER direction: not claimed as an
  // orphan (it will never be written by an Update<Type>Request), and not
  // required of one either (the same reason it needed its own map).
  it("stays out of the contract-derived census", () => {
    const labelled = historyFieldLabelled();
    for (const field of syntheticAuditFieldLabelled()) {
      expect(labelled).not.toContain(field);
    }
  });

  it("resolves every key to a word that exists in the catalog, not the raw payload key", () => {
    for (const field of syntheticAuditFieldLabelled()) {
      const key = historyFieldLabel(field, (k) => k);
      expect(key, field).not.toBe(field);
      expect(key in en, `${field} -> ${key}`).toBe(true);
    }
  });

  // The literal reproduction: a suppression write's own two payload keys,
  // rendered the way the History tab actually calls historyFieldLabel.
  it("names a suppression's own payload keys in plain English", () => {
    expect(historyFieldLabel("suppression_kind", (k) => en[k])).toBe(
      "Suppression kind",
    );
    expect(historyFieldLabel("decided_by_level", (k) => en[k])).toBe(
      "Decided by",
    );
  });

  // "kind" is deliberately NOT a synthetic key: an activity's own audited
  // create names its kind that way too, on the same projected entity type,
  // and this lookup carries no entity context to tell the two apart.
  it("leaves the bare kind field to whichever writer actually owns it", () => {
    expect(historyFieldLabel("kind", (k) => k)).toBe("kind");
  });

  it("names a lifted suppression's own payload keys in plain English", () => {
    expect(historyFieldLabel("lifted_by", (k) => en[k])).toBe("Lifted by");
    expect(historyFieldLabel("lifted_by_level", (k) => en[k])).toBe(
      "Lifted at level",
    );
  });
});

describe("company profile field labels", () => {
  it("names every profile field the company history can show as the profile tab does", () => {
    expect(historyFieldLabel("register_vat", (k) => en[k])).toBe(
      "Register / VAT ID",
    );
    expect(historyFieldLabel("registered_address", (k) => en[k])).toBe(
      "Registered address",
    );
    expect(historyFieldLabel("icp", (k) => en[k])).toBe(
      "Ideal customer profile",
    );
  });
});
