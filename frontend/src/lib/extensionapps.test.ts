import type { ExtensionDescriptor } from "@composition/extensions";
import { describe, expect, it } from "vitest";
import { meFixture } from "../testing/mefixture";
import {
  appRoute,
  appsFor,
  declaredApps,
  findApp,
  mayOpenApp,
} from "./extensionapps";
import { parseHash, routeHash } from "./router";

function unit(name: string, app?: ExtensionDescriptor["app"]) {
  return {
    name,
    title: "",
    verbs: [],
    entries: [],
    panels: [],
    secretScope: "",
    app,
  } satisfies ExtensionDescriptor;
}

const NOTES_APP = {
  icon: "plug",
  label: "extUnits.user.title",
  group: "work",
  object: "ext_notes_note",
  export: "NotesApp",
} as const;

const REGISTRY = [unit("notes", NOTES_APP), unit("plain")];

describe("a unit's app registry", () => {
  it("lists only the units that declare an app, with the unit that declared it", () => {
    expect(declaredApps(REGISTRY)).toEqual([{ ...NOTES_APP, unit: "notes" }]);
    expect(declaredApps([unit("plain")])).toEqual([]);
  });

  it("finds an app by unit and answers null for a unit with none", () => {
    expect(findApp("notes", REGISTRY)?.export).toBe("NotesApp");
    expect(findApp("plain", REGISTRY)).toBeNull();
    expect(findApp("nonesuch", REGISTRY)).toBeNull();
    expect(findApp(undefined, REGISTRY)).toBeNull();
  });

  it("opens the app only to a reader who may read the unit's object", () => {
    const [app] = declaredApps(REGISTRY);
    if (!app) throw new Error("the registry lost its app");
    const reader = meFixture({ allow: { ext_notes_note: ["read"] } });
    const other = meFixture({ allow: { ext_other_thing: ["read"] } });
    const writeOnly = meFixture({ allow: { ext_notes_note: ["update"] } });
    expect(mayOpenApp(reader, app)).toBe(true);
    expect(mayOpenApp(other, app)).toBe(false);
    expect(mayOpenApp(writeOnly, app)).toBe(false);
    expect(mayOpenApp(undefined, app)).toBe(false);
    expect(appsFor(other, REGISTRY)).toEqual([]);
  });
});

describe("the app address", () => {
  it("round-trips through the hash and names the unit", () => {
    expect(routeHash(appRoute("notes"))).toBe("#/app/notes");
    expect(parseHash("#/app/notes")).toMatchObject({
      screen: "app",
      id: "notes",
    });
  });

  it("leaves the sub-setting address as it was", () => {
    expect(parseHash("#/ext/notes")).toMatchObject({
      screen: "ext",
      id: "notes",
    });
  });
});
