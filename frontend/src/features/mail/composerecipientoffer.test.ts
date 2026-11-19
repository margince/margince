// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { withOffer, withoutOffer } from "./composerecipientoffer";

describe("the offered recipient", () => {
  it("fills an empty To", () => {
    expect(withOffer([], "dana@brandt.example", false)).toEqual([
      "dana@brandt.example",
    ]);
  });

  it("leaves a To the reader filled alone", () => {
    expect(
      withOffer(["cole@nordwand.example"], "dana@brandt.example", false),
    ).toEqual(["cole@nordwand.example"]);
  });

  it("is not added beside itself in other capitals", () => {
    expect(
      withOffer(["Dana@Brandt.example"], "dana@brandt.example", true),
    ).toEqual(["Dana@Brandt.example"]);
  });

  it("fills a vacated slot beside what the reader typed", () => {
    expect(
      withOffer(["cole@nordwand.example"], "dana@brandt.example", true),
    ).toEqual(["cole@nordwand.example", "dana@brandt.example"]);
  });

  it("leaves in whatever case it stands when the conversation changes", () => {
    expect(
      withoutOffer(
        ["DANA@brandt.example", "cole@nordwand.example"],
        "dana@brandt.example",
      ),
    ).toEqual(["cole@nordwand.example"]);
  });
});
