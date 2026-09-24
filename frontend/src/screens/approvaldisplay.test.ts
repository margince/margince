import { describe, expect, it } from "vitest";
import { LOCALES, translate } from "../i18n";
import { DISPLAY_FIELDS, resolveDisplay } from "./approvaldisplay";
import { KIND_LABEL } from "./approvalkind";

const t = (
  key: Parameters<typeof translate>[1],
  params?: Record<string, string>,
) => translate("en", key, params);

//
// The defect these pin: a card that printed its payload's own JSON keys handed
// a contact `deal_id`, `flags: ["unrealistic_stale"]` and `target_version` and
// asked them to make a business decision from it.
describe("what a staged proposal shows", () => {
  const day = (value: string) => value.split("-").reverse().join(".");

  const closeDate = (over: Record<string, unknown> = {}) =>
    resolveDisplay(
      "close_date_correction",
      {
        deal_id: "01a03781-9083-7565-8d65-5939ec0f3e70",
        basis: "deal has gone quiet; confirm it is still alive",
        expected_close_date: "2026-10-01",
        previous_close_date: "2026-10-01",
        flags: ["unrealistic_stale"],
        ...over,
      },
      t,
      day,
    );

  const shown = (fields: ReturnType<typeof resolveDisplay>) =>
    fields.filter((entry) => entry.value !== null && !entry.lead);

  it("names the fields it shows and leaves the identifiers out", () => {
    const fields = closeDate();
    expect(fields.map((entry) => entry.field)).not.toContain("deal_id");
    expect(shown(fields).map((entry) => entry.label)).toEqual([
      "Proposed date",
      "Issues",
    ]);
  });

  // The reason the server wrote leads, so the card can print it as a sentence
  // rather than as one more captioned row.
  it("marks the reason as the lead", () => {
    const lead = closeDate().find((entry) => entry.lead);
    expect(lead?.value).toBe("deal has gone quiet; confirm it is still alive");
  });

  // A wire enum on screen is a token wearing a caption.
  it("puts an enum into words", () => {
    const flags = closeDate().find((entry) => entry.field === "flags");
    expect(flags?.value).toBe("no recent progress");
  });

  // A code the catalogue has no word for still reaches the reader: a finding
  // the server raised and the card silently dropped tells them less than an
  // unpolished word does.
  it("degrades an unmapped enum to its own words rather than dropping it", () => {
    const flags = closeDate({ flags: ["some_new_finding"] }).find(
      (entry) => entry.field === "flags",
    );
    expect(flags?.value).toBe("some new finding");
  });

  // The sweep keeps the date on a stale deal and asks a human instead of
  // guessing a new one, so both dates are the same value. Two captions over one
  // value is not a comparison — it reads as a fault in the card.
  it("states an unchanged date once, as the proposal", () => {
    expect(shown(closeDate()).map((entry) => entry.field)).toEqual([
      "expected_close_date",
      "flags",
    ]);
  });

  // ...and when it genuinely moves, both sides are the whole point.
  it("keeps both dates when the proposal actually moves one", () => {
    const fields = closeDate({ expected_close_date: "2026-12-01" });
    expect(shown(fields).map((entry) => entry.field)).toEqual([
      "previous_close_date",
      "expected_close_date",
      "flags",
    ]);
  });

  // The general rule ("drop any field whose value another field already
  // showed") was briefly the implementation here, and it was wrong in a way no
  // close-date fixture could show: it deleted BOTH sides of a LinkedIn match
  // whose connection and contact share a name, which is a match at its most
  // obvious. Two fields agreeing is ordinarily information.
  it("keeps both sides of a match whose two names agree", () => {
    const fields = resolveDisplay(
      "linkedin_match",
      {
        connection_name: "Ada B",
        connection_company: "Helvetia",
        contact_name: "Ada B",
      },
      t,
      day,
    );
    expect(shown(fields).map((entry) => entry.field)).toEqual([
      "connection_name",
      "connection_company",
      "contact_name",
    ]);
  });

  // Same shape, and the coincidence is the whole story: an agent that has used
  // exactly its limit is one that just hit the ceiling.
  it("keeps a usage figure that equals its own limit", () => {
    const fields = resolveDisplay(
      "volume_release",
      { tool: "send_email", observed: 40, limit: 40, allowance: 100 },
      t,
      day,
    );
    expect(shown(fields).map((entry) => entry.value)).toEqual([
      "send_email",
      "40",
      "40",
      "100",
    ]);
  });

  // Absent is not blank: `previous_close_date` is genuinely missing on a deal
  // that never had one, and an empty row under a caption reads as a fact
  // nobody wrote.
  it("resolves a field the payload does not carry to null", () => {
    const fields = resolveDisplay(
      "close_date_correction",
      { basis: "b", expected_close_date: "2026-10-01" },
      t,
      day,
    );
    expect(
      fields.find((entry) => entry.field === "previous_close_date")?.value,
    ).toBeNull();
  });

  // A capture collision fills the lead's EMPTY fields and leaves the rest
  // alone, so the card has to say which is which. A pair is a conflict the
  // accept will drop; a captured value standing alone is one that lands.
  it("draws a captured value beside what the lead already holds", () => {
    const fields = resolveDisplay(
      "merge_records",
      {
        FullName: "J. Petersen",
        Email: "jonas@nordwind.test",
        CompanyName: "Nordwind Logistik",
        Title: "Operations Lead",
        current_full_name: "Jonas Petersen",
        current_company_name: "",
        current_title: "Head of Operations",
        Unowned: false,
      },
      t,
      day,
    );
    expect(shown(fields).map((entry) => [entry.label, entry.value])).toEqual([
      ["Current name", "Jonas Petersen"],
      ["Name in the message", "J. Petersen"],
      // No "Current company": the lead has none, so the captured company is
      // the one value here that accepting actually writes.
      ["Company in the message", "Nordwind Logistik"],
      ["Current job title", "Head of Operations"],
      ["Job title in the message", "Operations Lead"],
      ["Email", "jonas@nordwind.test"],
    ]);
  });

  // The identifiers stay off the card, here as everywhere: `Unowned` is how the
  // capture decides who a lead belongs to, not something a reader answers.
  it("leaves a collision card's bookkeeping out", () => {
    const fields = resolveDisplay(
      "merge_records",
      { FullName: "J. Petersen", Unowned: true },
      t,
      day,
    );
    expect(fields.map((entry) => entry.field)).not.toContain("Unowned");
  });

  // The honest fallback: a kind carrying an agent's tool arguments has no typed
  // payload to describe, so it declares nothing and keeps the generic reading.
  it("declares nothing for a kind whose payload is raw tool arguments", () => {
    expect(resolveDisplay("update_record", { stage_id: "x" }, t, day)).toEqual(
      [],
    );
  });

  // Every declared field must be a kind the server actually stages, and every
  // label must exist in the catalogue — the same two obligations EDITABLE_FIELDS
  // carries above.
  it("declares display only for kinds we stage", () => {
    const unknown = Object.keys(DISPLAY_FIELDS).filter(
      (kind) => !(kind in KIND_LABEL),
    );
    expect(
      unknown,
      "a display policy for a kind nobody stages asserts nothing",
    ).toEqual([]);
  });

  it("names every display label in every locale", () => {
    for (const locale of LOCALES) {
      for (const fields of Object.values(DISPLAY_FIELDS)) {
        for (const entry of fields) {
          const label = translate(locale, entry.label);
          expect(label, `${locale}: ${entry.label}`).not.toBe("");
          expect(label, `${locale}: ${entry.label}`).not.toBe(entry.label);
          for (const key of Object.values(entry.optionLabels ?? {})) {
            const option = translate(locale, key);
            expect(option, `${locale}: ${key}`).not.toBe(key);
          }
        }
      }
    }
  });
});
