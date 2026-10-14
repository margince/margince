// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { rowsState } from "./surfacestate";

describe("rowsState", () => {
  it("is partial with the rows left out when the server matched more than it sent", () => {
    expect(rowsState(1000, 1250)).toEqual({ state: "partial", remaining: 250 });
  });

  it("is partial when a capped answer sent no rows but matched some", () => {
    expect(rowsState(0, 3)).toEqual({ state: "partial", remaining: 3 });
  });

  it("is ready when every matched row is in hand", () => {
    expect(rowsState(4, 4)).toEqual({ state: "ready", remaining: 0 });
  });

  it("is empty only when the server matched none", () => {
    expect(rowsState(0, 0)).toEqual({ state: "empty", remaining: 0 });
  });

  it("leaves nothing remaining when the server sends no total", () => {
    expect(rowsState(5, undefined)).toEqual({ state: "ready", remaining: 0 });
    expect(rowsState(0, null)).toEqual({ state: "empty", remaining: 0 });
  });

  it("never counts a negative remainder when the total is below the rows in hand", () => {
    expect(rowsState(5, 2)).toEqual({ state: "ready", remaining: 0 });
  });
});
