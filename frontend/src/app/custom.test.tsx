import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { Users } from "lucide-react";
import { describe, expect, it } from "vitest";
import type { MessageKey } from "../i18n/en";
import {
  buildRegistry,
  CUSTOM_SCREEN,
  type CustomNavScreen,
  type CustomPaletteScreen,
  type CustomScreen,
  type CustomScreenRegistry,
  customNavItems,
  customPaletteScreens,
  customScreens,
  findCustomScreen,
  resolveCustomLabel,
} from "./custom";
import { isScreen, parseHash } from "./router";
import { navEntryHref } from "./subnav";

// The fork seam, from both ends.
//
// A seam that is empty upstream is a seam nothing exercises, and the failure
// mode of an unexercised one is silence: a fork drops a directory in, nothing
// appears, and there is no error to read. So the tests below split in two.
//
// The first group asserts what vanilla IS — empty, and empty in every reader,
// because a rail that grew a row for a fork nobody wrote would be worse than
// the seam not existing at all.
//
// The second drives the same readers over a registry a fork would produce. That
// is the half that would otherwise never run: upstream ships nothing, so every
// answer these functions give a fork is one this repo has never taken.

describe("upstream ships the seam empty", () => {
  it("declares no custom screen", () => {
    expect([...customScreens.keys()]).toEqual([]);
  });

  it("adds no row to any rail group", () => {
    expect(customNavItems("records")).toEqual([]);
    expect(customNavItems("work")).toEqual([]);
    expect(customNavItems("intelligence")).toEqual([]);
  });

  // The address is answerable even with nothing behind it, and that is the
  // point of routing it rather than leaving it to the not-found arm: a fork's
  // link keeps working across an upgrade because the SEGMENT is upstream's.
  it("routes #/x/<key> as its own screen, and resolves nothing", () => {
    expect(isScreen(CUSTOM_SCREEN)).toBe(true);
    expect(parseHash("#/x/warranty")).toEqual({
      screen: CUSTOM_SCREEN,
      id: "warranty",
    });
    expect(findCustomScreen("warranty")).toBeUndefined();
  });

  // The lookup is a Map, so this is already true — asserted because it was NOT
  // true of the extension registry, which is an object literal and answered
  // `extensionScreens["constructor"]` from the prototype chain with a function
  // React then tried to mount (App.tsx says so at its own lookup).
  it("resolves nothing for a key that names an Object member", () => {
    expect(findCustomScreen("constructor")).toBeUndefined();
    expect(findCustomScreen("toString")).toBeUndefined();
  });
});

// The reading half, driven over a registry a fork would produce.
//
// The registry itself cannot be faked: `import.meta.glob` is resolved by Vite at
// build time from a literal, so there is no way to point it at a fixture. What
// the readers take instead is the registry as an argument — the shape
// app/extensions.ts uses, and for its stated reason: upstream's is empty by
// construction, so a reader closed over the module binding could only ever be
// exercised on its miss path, and every answer it gave a fork would be one
// nothing here had ever run.
describe("a fork's screen reaches the rail, the palette and the router", () => {
  const warranty: CustomNavScreen & CustomPaletteScreen = {
    key: "warranty",
    component: () => null,
    // The fork's OWN words, which is the case the seam exists for: `warranty`
    // is not a noun this product has, so there is no key to reuse.
    nav: {
      group: "records",
      label: { en: "Warranty", de: "Garantie" },
      icon: Users,
    },
    palette: { label: { en: "Warranty" } },
  };
  // A second screen that wants NEITHER, which is what proves the two lists are
  // read from their own fields rather than from "is this a fork screen".
  const audit: CustomScreen = { key: "audit-trail", component: () => null };
  const forked: CustomScreenRegistry = new Map([
    [warranty.key, warranty],
    [audit.key, audit],
  ]);

  it("resolves by key, and a key nothing declares stays unresolved", () => {
    expect(findCustomScreen("warranty", forked)).toBe(warranty);
    expect(findCustomScreen("audit-trail", forked)).toBe(audit);
    expect(findCustomScreen("nothing-declared", forked)).toBeUndefined();
    expect(findCustomScreen(undefined, forked)).toBeUndefined();
  });

  it("joins the group it named, and no other", () => {
    expect(customNavItems("records", forked)).toEqual([warranty]);
    expect(customNavItems("work", forked)).toEqual([]);
    expect(customNavItems("intelligence", forked)).toEqual([]);
  });

  it("is in the palette only if it asked to be", () => {
    expect(customPaletteScreens(forked)).toEqual([warranty]);
  });

  // The half that makes the seam whole. A fork screen's label cannot be a
  // MessageKey — `warranty` is not a noun this product has — and minting one
  // would mean editing i18n/en.ts, de.ts and vi.ts: three upstream files, for
  // the one string that names a row. So the words ship WITH the screen.
  it("reads its own words, in the reader's language, falling back to English", () => {
    const t = (key: MessageKey) => `translated:${key}`;
    expect(resolveCustomLabel(warranty.nav.label, "de", t)).toBe("Garantie");
    expect(resolveCustomLabel(warranty.nav.label, "en", t)).toBe("Warranty");
    // Vietnamese is a locale this fork does not carry. English rather than a
    // blank or the key: there is always something to render, and a fork that
    // ships one language must not put a hole in a rail.
    expect(resolveCustomLabel(warranty.nav.label, "vi", t)).toBe("Warranty");
  });

  // And the other arm, which is why this is a union rather than a record: a
  // fork screen that IS one of the product's nouns seen differently reuses the
  // word the product already has, in every language it already has it.
  it("reuses an upstream key when the fork names one", () => {
    const t = (key: MessageKey) => `translated:${key}`;
    expect(resolveCustomLabel("nav.contacts", "de", t)).toBe(
      "translated:nav.contacts",
    );
  });

  // Two directories claiming one key is the failure this seam is worst at: a
  // Map keeps the last, and the loser is a screen with a component and a rail
  // row that no address reaches, with nothing to read and nothing looking wrong.
  it("refuses a key two screens claim", () => {
    expect(() =>
      buildRegistry({
        "./warranty/screen.tsx": { screen: warranty },
        "./legacy-warranty/screen.tsx": { screen: { ...warranty } },
      }),
    ).toThrow(/two custom screens claim the key "warranty"/);
    // And names the second one, so a fork knows which directory to change.
    expect(() =>
      buildRegistry({
        "./warranty/screen.tsx": { screen: warranty },
        "./legacy-warranty/screen.tsx": { screen: { ...warranty } },
      }),
    ).toThrow(/legacy-warranty/);
  });

  it("skips a module that exports no screen", () => {
    const registry = buildRegistry({
      "./warranty/screen.tsx": { screen: warranty },
      "./half-written/screen.tsx": {},
    });
    expect([...registry.keys()]).toEqual(["warranty"]);
  });

  it("is addressed at the segment its rail row builds", () => {
    // The round trip, not either half: the rail builds `path + prefix + id` and
    // the router parses the same string back, and a prefix and a parser that
    // disagreed would each look correct alone.
    const href = navEntryHref([], {
      id: warranty.key,
      prefix: [CUSTOM_SCREEN],
      label: warranty.nav.label,
      icon: warranty.nav.icon,
    });
    expect(href).toBe("#/x/warranty");
    expect(parseHash(href)).toEqual({
      screen: CUSTOM_SCREEN,
      id: warranty.key,
    });
  });
});

// The one thing the tests above cannot see.
//
// Upstream ships no fork screen, so a glob that matches the documented layout
// and a glob that matches NOTHING give the same answer here: an empty registry.
// Point the pattern at `*/notascreen.tsx` and every assertion above still
// passes — which means a rename of the directory, or of the file inside it,
// would land green and be discovered by a fork whose screen silently did not
// appear.
//
// There is no fixture that closes this without shipping one: a screen committed
// here is a screen upstream ships, rail row and all. What CAN be checked is
// that the pattern still points at something real, and that the README a fork
// actually follows says the same path. Two statements that must agree, and a
// one-sided edit is the realistic way this breaks.
describe("the glob and the documented layout are the same layout", () => {
  const source = readFileSync(
    fileURLToPath(new URL("./custom.ts", import.meta.url)),
    "utf8",
  );
  const pattern = /import\.meta\.glob<[^>]*>\(\s*"([^"]+)"/.exec(source)?.[1];

  it("globs the directory a fork is told to write in", () => {
    // The DIRECTORY, not just that some directory exists: a glob pointed at
    // `../screens/other/*/screen.tsx` would pass a file-name check, because the
    // README's example ends in `screen.tsx` too — and a fork following the
    // README would put its screens somewhere nothing looks.
    expect(pattern).toBeDefined();
    const readme = readFileSync(
      fileURLToPath(new URL("../screens/custom/README.md", import.meta.url)),
      "utf8",
    );
    const documented = /(src\/screens\/[a-z-]+)\/[a-z-]+\/[a-z-]+\.tsx/.exec(
      readme,
    )?.[1];
    if (documented === undefined || pattern === undefined) {
      throw new Error(
        "the README names no screen path, or custom.ts globs none",
      );
    }
    // Both resolved to an absolute path, because they are written from
    // different places: the README's is repo-relative and the glob's is
    // relative to this module.
    const globbed = fileURLToPath(
      new URL(pattern.slice(0, pattern.indexOf("*")), import.meta.url),
    );
    const told = fileURLToPath(
      new URL(`../../${documented}/`, import.meta.url),
    );
    expect(globbed).toBe(told);
  });

  it("globs a directory that exists", () => {
    expect(pattern).toBeDefined();
    if (pattern === undefined) {
      throw new Error("no import.meta.glob literal in custom.ts");
    }
    const directory = pattern.slice(0, pattern.indexOf("*"));
    expect(existsSync(fileURLToPath(new URL(directory, import.meta.url)))).toBe(
      true,
    );
  });

  it("globs the file name the README tells a fork to write", () => {
    const readme = readFileSync(
      fileURLToPath(new URL("../screens/custom/README.md", import.meta.url)),
      "utf8",
    );
    // The path in the README's own worked example, which is what a fork copies.
    const documented = /src\/screens\/custom\/[a-z-]+\/([a-z-]+\.tsx)/.exec(
      readme,
    )?.[1];
    if (documented === undefined || pattern === undefined) {
      throw new Error(
        "the README names no screen file, or custom.ts globs none",
      );
    }
    expect(pattern.endsWith(`/${documented}`)).toBe(true);
  });
});
