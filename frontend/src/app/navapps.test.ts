import { describe, expect, it } from "vitest";
import type { UnitApp } from "../lib/extensionapps";
import { railTrail } from "./nav";

const NOTES: UnitApp = {
  unit: "notes",
  icon: "plug",
  label: "extUnits.user.title",
  group: "work",
  object: "ext_notes_note",
  export: "NotesApp",
};

describe("the drawer rows", () => {
  it("adds nothing when no app is offered", () => {
    const [primary] = railTrail({ screen: "home" });
    const ids = primary?.groups.flatMap((g) => g.items.map((row) => row.id));
    expect(ids).not.toContain("notes");
  });

  it("files the app under its heading, after the product's own rows, addressed at the app route", () => {
    const [primary] = railTrail({ screen: "home" }, undefined, [], false, [
      NOTES,
    ]);
    const work = primary?.groups.find(
      (group) => group.headingKey === "nav.group.work",
    );
    expect(work?.items.map((row) => row.id)).toEqual([
      "projects",
      "filters",
      "notes",
    ]);
    expect(work?.items.at(-1)).toMatchObject({
      id: "notes",
      prefix: ["app"],
      label: { en: "Other accounts" },
    });
  });

  it("marks the app's row current on its own address", () => {
    const [primary] = railTrail(
      { screen: "app", id: "notes" },
      undefined,
      [],
      false,
      [NOTES],
    );
    expect(primary?.activeId).toBe("notes");
  });
});
