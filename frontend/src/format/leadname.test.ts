// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { leadIdentityName } from "./leadname";

// The same cases the server's own `TestLeadIdentityName` runs, because the two
// answer one question and a browser that disagreed would draw a blank name
// beside a promotion that names the lead by its address.
describe("what a lead is called", () => {
  it("is its own name when it has one", () => {
    expect(leadIdentityName({ full_name: "Vera Lindqvist" })).toBe(
      "Vera Lindqvist",
    );
  });

  it("is the email address of a lead with no name of its own", () => {
    expect(leadIdentityName({ email: "vera@nordwind.example" })).toBe(
      "vera@nordwind.example",
    );
  });

  it("prefers the name over the address", () => {
    expect(
      leadIdentityName({
        full_name: "Vera Lindqvist",
        email: "vera@nordwind.example",
      }),
    ).toBe("Vera Lindqvist");
  });

  // The defect this helper exists for: `??` fires on null alone, so a stored
  // empty string reads as a name and the lead renders blank.
  it("does not count a present-but-empty full_name as a name", () => {
    expect(
      leadIdentityName({ full_name: "", email: "vera@nordwind.example" }),
    ).toBe("vera@nordwind.example");
  });

  it("does not count padding as a name either", () => {
    expect(
      leadIdentityName({ full_name: "   ", email: "vera@nordwind.example" }),
    ).toBe("vera@nordwind.example");
  });

  it("carries no padding out of a name it does keep", () => {
    expect(leadIdentityName({ full_name: "  Vera Lindqvist  " })).toBe(
      "Vera Lindqvist",
    );
  });

  it("answers nothing for a lead with neither", () => {
    expect(leadIdentityName({})).toBe("");
    expect(leadIdentityName({ full_name: null, email: null })).toBe("");
  });

  it("answers nothing for an empty full_name with no address behind it", () => {
    expect(leadIdentityName({ full_name: "" })).toBe("");
  });

  it("carries no padding out of an address either", () => {
    expect(leadIdentityName({ email: "  vera@nordwind.example  " })).toBe(
      "vera@nordwind.example",
    );
  });

  // A padded name over a padded address names nobody. Answering the padding
  // would hand back a TRUTHY blank, and every caller's own last resort — the
  // id, the word for a lead — is reached with `||` and would never fire.
  it("answers nothing when both fields are nothing but padding", () => {
    expect(leadIdentityName({ full_name: " ", email: "   " })).toBe("");
  });
});
