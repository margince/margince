/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it } from "vitest";
import { ENTITY, ENTITY_KINDS } from "../../app/entity";
import { routeHash } from "../../app/router";
import {
  CITED_RECORD_KINDS,
  citationOpensRecord,
  openCitation,
} from "./citationroute";

afterEach(() => {
  window.location.hash = "";
});

describe("openCitation", () => {
  it.each(CITED_RECORD_KINDS)("opens a cited %s on its own screen", (kind) => {
    expect(citationOpensRecord(kind)).toBe(true);
    openCitation(kind, "r-1");
    expect(window.location.hash).toBe(routeHash(ENTITY[kind].route("r-1")));
  });

  const unrouted = [
    ...ENTITY_KINDS.filter((kind) => !citationOpensRecord(kind)),
    "activity",
    "fact",
    "profile_field",
    "toString",
  ];

  it.each(unrouted)("leaves the page where it is for a cited %s", (kind) => {
    expect(citationOpensRecord(kind)).toBe(false);
    openCitation(kind, "r-1");
    expect(window.location.hash).toBe("");
  });
});
