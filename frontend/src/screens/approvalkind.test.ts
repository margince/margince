import { describe, expect, it } from "vitest";
import { LOCALES, translate } from "../i18n";
import {
  approvalKindLabel,
  EDITABLE_FIELDS,
  humanizeKind,
  KIND_LABEL,
} from "./approvalkind";

// A proposal's kind is a wire enum. A reader deciding whether to accept
// twenty-five of something must be told what that something is.

const t = (
  key: Parameters<typeof translate>[1],
  params?: Record<string, string>,
) => translate("en", key, params);

describe("what a staged proposal is called", () => {
  // Over LOCALES, not a chosen pair: a locale whose label leaked the raw
  // identifier would pass a two-locale sweep, and the byte-equality guard in
  // i18n.test.ts only flags values EQUAL to English — an identifier-shaped
  // translation differs from English, so nothing else in the suite sees it.
  // WHICH kinds must be here is the server's to say, and
  // backend/frontendapprovalkinds_test.go holds that both ways against the
  // grant maps themselves. What this covers is the half a Go test cannot
  // read: that every label actually says something, in every locale we ship.
  it("gives every labelled kind real words, in every shipped locale", () => {
    expect(Object.keys(KIND_LABEL).length).toBeGreaterThan(0);
    for (const kind of Object.keys(KIND_LABEL)) {
      for (const locale of LOCALES) {
        const label = translate(locale, KIND_LABEL[kind]);
        expect(label.trim(), `${kind} in ${locale}`).not.toBe("");
        // The identifier itself is the thing this map exists to stop showing.
        expect(label, `${kind} in ${locale}`).not.toContain("_");
      }
    }
  });

  it("names a known kind in words, never its identifier", () => {
    expect(approvalKindLabel("site_lead", t)).toBe(
      "Add a person found on the site",
    );
    expect(approvalKindLabel("fx_rate_proposal", t)).toBe(
      "Refresh exchange rates",
    );
  });

  it("degrades an unmapped kind to its own words, not snake_case", () => {
    expect(approvalKindLabel("some_future_kind", t)).toBe("some future kind");
    expect(humanizeKind("a_b_c")).toBe("a b c");
  });
});

// What a reader may change before accepting.
//
// The inline editor's default offers every string field as free text. For a
// proposal built out of identifiers and enums that default hands a reader two
// ways to type themselves into a server refusal: re-aiming the proposal at
// another record (assertSameEntityRefs), or naming a stage that does not
// exist. A declared policy is what keeps those off the screen.
describe("what a reader may change before accepting", () => {
  it("offers only the stage on a lifecycle proposal, and only real stages", () => {
    const fields = EDITABLE_FIELDS.lifecycle_change;
    expect(fields.map((entry) => entry.field)).toEqual(["proposed_lifecycle"]);
    const [stage] = fields;
    expect(stage.as).toBe("choice");
    if (stage.as !== "choice") {
      throw new Error(
        "the stage field must be a fixed choice, never free text",
      );
    }
    expect(stage.options).toContain("former_customer");
    expect(stage.options).toContain("customer");
  });

  it("never offers an identifier, on any kind that declares a policy", () => {
    for (const [kind, fields] of Object.entries(EDITABLE_FIELDS)) {
      for (const entry of fields) {
        expect(
          entry.field.endsWith("_id"),
          `${kind} offers ${entry.field} for editing — changing an identifier ` +
            "re-aims the proposal at another record, which the server refuses",
        ).toBe(false);
      }
    }
  });

  it("declares a policy only for a kind this surface labels", () => {
    const unknown = Object.keys(EDITABLE_FIELDS).filter(
      (kind) => !(kind in KIND_LABEL),
    );
    expect(
      unknown,
      "a policy for a kind nobody stages asserts nothing",
    ).toEqual([]);
  });
});
