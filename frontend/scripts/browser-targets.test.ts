// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import browserslist from "browserslist";
import { describe, expect, it } from "vitest";
import spaConfig from "../vite.config";
import {
  policyTargets,
  supportedBrowsers,
  viteTargets,
} from "./browser-targets";

describe("viteTargets", () => {
  it("takes the oldest version listed for each engine, compared as numbers", () => {
    expect(
      viteTargets([
        "chrome 157",
        "and_chr 151",
        "chrome 146",
        "ios_saf 26.0",
        "ios_saf 18.5-18.7",
        "safari 26.0",
        "safari 9.1",
      ]),
    ).toEqual(["chrome146", "ios18.5", "safari9.1"]);
  });

  it("lowers for the Chromium that a Chromium build runs", () => {
    const chromium = new Map([
      ["samsunginternet_android", [{ version: [30, 0], chromium: 143 }]],
      ["webview_android", [{ version: [157], chromium: 157 }]],
    ]);
    expect(
      viteTargets(["samsung 30", "android 157", "chrome 150"], chromium),
    ).toEqual(["chrome143"]);
  });

  it("gives a build newer than the mapping's data the newest Chromium it knows", () => {
    const chromium = new Map([
      [
        "samsunginternet_android",
        [
          { version: [29, 0], chromium: 136 },
          { version: [30, 0], chromium: 143 },
        ],
      ],
    ]);
    expect(viteTargets(["samsung 31", "chrome 150"], chromium)).toEqual([
      "chrome143",
    ]);
  });

  it("refuses a Chromium build whose Chromium the mapping does not know", () => {
    const chromium = new Map([
      ["samsunginternet_android", [{ version: [30, 0], chromium: 143 }]],
    ]);
    expect(() => viteTargets(["samsung 28"], chromium)).toThrow(/samsung 28/);
    expect(() => viteTargets(["op_mob 80"], chromium)).toThrow(/op_mob 80/);
  });

  it("refuses a browser the build cannot lower for", () => {
    expect(() => viteTargets(["kaios 3.0-3.1"])).toThrow(/kaios 3\.0-3\.1/);
    expect(() => viteTargets(["and_uc 15.5"])).toThrow(/and_uc 15\.5/);
    expect(() => viteTargets(["safari TP"])).toThrow(/safari TP/);
  });
});

describe("the browser policy", () => {
  const browsers = supportedBrowsers();
  const families = new Set(browsers.map((browser) => browser.split(" ")[0]));

  it("covers every primary and secondary browser family", () => {
    for (const family of [
      "chrome",
      "and_chr",
      "edge",
      "samsung",
      "safari",
      "ios_saf",
      "firefox",
    ]) {
      expect(families).toContain(family);
    }
  });

  it("supports every Firefox extended support release", () => {
    for (const release of browserslist("Firefox ESR")) {
      expect(browsers).toContain(release);
    }
  });

  it("names no browser it shuts out", () => {
    for (const family of [
      "ie",
      "ie_mob",
      "op_mini",
      "kaios",
      "and_ff",
      "and_uc",
      "and_qq",
    ]) {
      expect(families).not.toContain(family);
    }
    const dead = new Set(browserslist("dead"));
    expect(browsers.filter((browser) => dead.has(browser))).toEqual([]);
  });

  it("is the floor the production build lowers to", () => {
    expect(spaConfig.build?.target).toEqual(policyTargets());
    expect(
      policyTargets().map((target) => target.replace(/[\d.]+$/, "")),
    ).toEqual(
      expect.arrayContaining(["chrome", "edge", "firefox", "ios", "safari"]),
    );
  });
});
