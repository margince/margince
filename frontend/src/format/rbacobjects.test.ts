// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { de } from "../i18n/de";
import { en } from "../i18n/en";
import { vi as vietnamese } from "../i18n/vi";
import {
  CORE_OBJECTS,
  coreObjectName,
  GRANT_AREA_NAME,
  objectLabel,
} from "./rbacobjects";

describe("coreObjectName", () => {
  it("names a core object and its area", () => {
    expect(coreObjectName("deal")).toEqual({
      area: "records",
      label: "users.access.object.deal",
    });
  });

  it("knows no object outside the contract, nor one from the prototype", () => {
    expect(coreObjectName("ext_notes_note")).toBeUndefined();
    expect(coreObjectName("constructor")).toBeUndefined();
  });

  it("gives every object a name of its own in each language", () => {
    for (const catalog of [en, de, vietnamese]) {
      const names = Object.values(CORE_OBJECTS).map(
        ({ label }) => catalog[label],
      );
      expect(new Set(names).size).toBe(names.length);
    }
  });

  it("names every area apart", () => {
    const names = Object.values(GRANT_AREA_NAME).map((key) => en[key]);
    expect(new Set(names).size).toBe(names.length);
  });
});

describe("objectLabel", () => {
  it("names an object by its key less the unit's prefix, in sentence case", () => {
    expect(objectLabel("openchannel", "ext_openchannel_endpoint")).toBe(
      "Endpoint",
    );
    expect(objectLabel("notes", "ext_notes_signing_key")).toBe("Signing key");
    expect(objectLabel("notes", "ext_other_thing")).toBe("Other thing");
    expect(objectLabel("notes", "ext_notes_")).toBe("Notes");
  });
});
