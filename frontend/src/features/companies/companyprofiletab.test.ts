import { describe, expect, it } from "vitest";
import { emptyReason, repeatsDescription } from "./companyprofiletab";

describe("repeatsDescription", () => {
  const offer = [{ field: "offer_summary", value: "Bearings sold online. " }];

  it("hides the offer row that repeats the description", () => {
    expect(
      repeatsDescription("offer_summary", offer, "Bearings sold online."),
    ).toBe(true);
  });

  it("keeps an offer the description does not repeat, and every other field", () => {
    expect(
      repeatsDescription("offer_summary", offer, "A family firm in Ulm."),
    ).toBe(false);
    expect(repeatsDescription("offer_summary", offer, null)).toBe(false);
    expect(
      repeatsDescription(
        "icp",
        [{ field: "icp", value: "Bearings sold online." }],
        "Bearings sold online.",
      ),
    ).toBe(false);
  });
});

describe("emptyReason", () => {
  it("says the website does not state it once a read finished", () => {
    for (const status of ["ready", "partial", "confirmed"]) {
      expect(emptyReason(status)).toBe("co.narrative.notOnSite");
    }
  });

  it("says the website was not read yet before a read finished", () => {
    for (const status of [
      null,
      undefined,
      "queued",
      "reading",
      "failed",
      "abandoned",
    ]) {
      expect(emptyReason(status)).toBe("co.narrative.notReadYet");
    }
  });
});
