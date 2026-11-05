// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, it } from "vitest";
import { reportScope, writableScope } from "./analytics.context";

it("names a report's scope the way the picker's scope names it on the wire", () => {
  expect(reportScope({ kind: "workspace", label: "Whole company" })).toEqual({
    kind: "workspace",
    id: undefined,
  });
  expect(reportScope({ kind: "team", id: "t-1", label: "North" })).toEqual({
    kind: "team",
    id: "t-1",
  });
  expect(reportScope({ kind: "owner", id: "u-1", label: "Riley" })).toEqual({
    kind: "owner",
    id: "u-1",
  });
  expect(
    reportScope({ kind: "managed_teams", label: "My teams" }),
  ).toBeUndefined();
});

it("withholds writes about a scope the server marks as not assertable", () => {
  expect(
    writableScope({
      kind: "team",
      id: "t-1",
      label: "North",
      assertable: true,
    }),
  ).toEqual({ scope_kind: "team", scope_id: "t-1" });
  expect(
    writableScope({
      kind: "team",
      id: "t-2",
      label: "South",
      assertable: false,
    }),
  ).toBeNull();
  expect(
    writableScope({
      kind: "workspace",
      label: "Whole company",
      assertable: false,
    }),
  ).toBeNull();
  expect(writableScope({ kind: "workspace", label: "Whole company" })).toEqual({
    scope_kind: "workspace",
  });
});
