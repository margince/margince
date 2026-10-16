import { describe, expect, it } from "vitest";
import { oauthReturnOf } from "./oauthreturn";

describe("the consent outcome a unit screen reads", () => {
  it("reads the outcome and the provider the callback sent", () => {
    expect(oauthReturnOf("#/ext/ledger?oauth=ok&provider=books")).toEqual({
      outcome: "ok",
      provider: "books",
    });
  });

  // A screen that believed an invented outcome would tell an admin the
  // connection worked when the server never said so.
  it("reads an outcome outside the server's set, or a half address, as none", () => {
    expect(oauthReturnOf("#/ext/ledger?oauth=connected&provider=books")).toBe(
      null,
    );
    expect(oauthReturnOf("#/ext/ledger?oauth=ok")).toBe(null);
    expect(oauthReturnOf("#/ext/ledger")).toBe(null);
  });
});
