// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it } from "vitest";
import {
  forgetSeat,
  readStored,
  STORAGE_KEYS,
  writeStored,
} from "../../lib/storage";
import { forecastFit } from "./analytics.forecast.fit";
import {
  chooseContext,
  dialsOf,
  namesContext,
  noteContext,
  outboundDials,
  permittedDials,
  rememberedContext,
  rememberedView,
  rememberView,
  withContext,
} from "./analytics.overview.context";

const ANNA = "00000000-0000-4000-8000-0000000000a1";
const BEN = "00000000-0000-4000-8000-0000000000b2";

afterEach(() => {
  localStorage.clear();
  sessionStorage.clear();
});

const params = (entries: Record<string, string>) =>
  new Map(Object.entries(entries));

describe("one view's question in the address", () => {
  it("reads only the dials the view asks with", () => {
    const address = params({
      overview: "sales",
      scope: "team:t-1",
      pipeline: "all",
      period: "this_month",
      close: "fiscal_quarter",
    });
    expect(dialsOf(address, "sales")).toEqual({
      scope: "team:t-1",
      pipeline: "all",
      period: "this_month",
      close: "fiscal_quarter",
    });
    // My prospecting is the reader's own work: no scope, no pipeline.
    expect(dialsOf(address, "prospecting")).toEqual({ period: "this_month" });
    expect(dialsOf(address, "team")).toEqual({
      scope: "team:t-1",
      period: "this_month",
    });
  });

  it("lets an address that states a question win over what was remembered", () => {
    expect(namesContext(params({ overview: "sales" }), "sales")).toBe(false);
    expect(namesContext(params({ pipeline: "all" }), "sales")).toBe(true);
    expect(namesContext(params({ pipeline: "all" }), "prospecting")).toBe(
      false,
    );
  });

  it("opens a view on its own question and drops the one it left", () => {
    const left = params({
      overview: "sales",
      pipeline: "all",
      close: "fiscal_quarter",
      ask: "1",
    });
    expect(
      Object.fromEntries(withContext(left, "team", { period: "last_week" })),
    ).toEqual({ overview: "team", period: "last_week", ask: "1" });
  });
});

describe("what a remembered question may still ask", () => {
  it("drops a scope the reader lost and a period the view does not read", () => {
    expect(
      permittedDials(
        { scope: "team:gone", period: "this_quarter", pipeline: "renewals" },
        "team",
        ["team:t-1"],
      ),
    ).toEqual({});
    expect(
      permittedDials(
        { scope: "team:t-1", period: "custom", close: "sometime" },
        "sales",
        ["team:t-1"],
      ),
    ).toEqual({ scope: "team:t-1", period: "custom" });
  });

  it("keeps a pipeline, which Sales review checks itself", () => {
    expect(permittedDials({ pipeline: "renewals" }, "sales", [])).toEqual({
      pipeline: "renewals",
    });
  });
});

describe("where a question is remembered", () => {
  it("keeps what a tab showed apart from what the reader chose", () => {
    noteContext(ANNA, "sales", { pipeline: "renewals" });
    expect(rememberedContext(ANNA, "sales")).toEqual({ pipeline: "renewals" });
    // A new tab has no tab memory and no choice: a link opened is no choice.
    sessionStorage.clear();
    expect(rememberedContext(ANNA, "sales")).toBeUndefined();

    chooseContext(ANNA, "sales", { pipeline: "all" });
    sessionStorage.clear();
    expect(rememberedContext(ANNA, "sales")).toEqual({ pipeline: "all" });
  });

  it("prefers this tab's last question over the account's choice", () => {
    chooseContext(ANNA, "sales", { pipeline: "all" });
    noteContext(ANNA, "sales", { pipeline: "renewals" });
    expect(rememberedContext(ANNA, "sales")).toEqual({ pipeline: "renewals" });
  });

  it("keeps an omitted pipeline apart from all pipelines", () => {
    chooseContext(ANNA, "sales", {});
    chooseContext(ANNA, "team", { period: "last_week" });
    expect(rememberedContext(ANNA, "sales")).toEqual({});
    expect(rememberedContext(ANNA, "team")).toEqual({ period: "last_week" });
  });

  it("keeps each account's choices to that account", () => {
    chooseContext(ANNA, "sales", { pipeline: "all" });
    rememberView(ANNA, "team");
    expect(rememberedContext(BEN, "sales")).toBeUndefined();
    expect(rememberedView(BEN)).toBeNull();
    expect(rememberedView(ANNA)).toBe("team");
  });

  it("honours a view kept before choices were per account, then replaces it", () => {
    writeStored(STORAGE_KEYS.analyticsOverviewView, "prospecting");
    expect(rememberedView(ANNA)).toBe("prospecting");
    rememberView(ANNA, "sales");
    expect(readStored(STORAGE_KEYS.analyticsOverviewView)).toBeNull();
    expect(rememberedView(ANNA)).toBe("sales");
  });

  it("reads a value in another shape as nothing remembered", () => {
    writeStored(
      { family: STORAGE_KEYS.analyticsOverview, member: ANNA },
      "{not json",
    );
    expect(rememberedView(ANNA)).toBeNull();
    writeStored(
      { family: STORAGE_KEYS.analyticsOverview, member: ANNA },
      JSON.stringify({ view: "board", contexts: { sales: { pipeline: 7 } } }),
    );
    expect(rememberedView(ANNA)).toBeNull();
    expect(rememberedContext(ANNA, "sales")).toEqual({});
  });

  it("forgets every choice at sign-out", () => {
    chooseContext(ANNA, "sales", { pipeline: "all" });
    rememberView(ANNA, "team");
    forgetSeat();
    expect(rememberedContext(ANNA, "sales")).toBeUndefined();
    expect(rememberedView(ANNA)).toBeNull();
  });
});

describe("the question a door out of a view carries", () => {
  it("spells every default out, the scope included", () => {
    // A copied link opened by a reader with another default must still name
    // this population.
    expect(
      Object.fromEntries(
        outboundDials("sales", {
          scope: "team:t-1",
          pipeline: "sales",
          period: "this_quarter",
        }),
      ),
    ).toEqual({
      overview: "sales",
      scope: "team:t-1",
      pipeline: "sales",
      period: "this_quarter",
    });
  });

  it("keeps My prospecting on the reader's own records", () => {
    expect(
      outboundDials("prospecting", { scope: `owner:${ANNA}` }).get("scope"),
    ).toBe(`owner:${ANNA}`);
  });
});

describe("what the forecast can answer of a carried question", () => {
  const two = [{ id: "sales" }, { id: "renewals" }];

  it("reads the current week, month or quarter, and the quarter by default", () => {
    expect(forecastFit("this_week", null, undefined)).toEqual({
      kind: "fits",
      period: "week",
    });
    expect(forecastFit("this_month", "all", undefined)).toEqual({
      kind: "fits",
      period: "month",
    });
    expect(forecastFit(undefined, undefined, undefined)).toEqual({
      kind: "fits",
      period: "quarter",
    });
  });

  it("says so for a past or custom period instead of reading the current one", () => {
    expect(forecastFit("last_month", null, two).kind).toBe("period");
    expect(forecastFit("custom", null, two).kind).toBe("period");
  });

  it("says so for one pipeline of several, and for one it cannot check", () => {
    expect(forecastFit("this_quarter", "sales", two).kind).toBe("pipeline");
    expect(forecastFit("this_quarter", "sales", null).kind).toBe("pipeline");
    expect(forecastFit("this_quarter", "sales", undefined).kind).toBe(
      "pending",
    );
  });

  it("answers for the only pipeline there is", () => {
    expect(forecastFit("this_quarter", "sales", [{ id: "sales" }])).toEqual({
      kind: "fits",
      period: "quarter",
    });
  });
});
