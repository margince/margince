// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { type CarriageViolation, carriageViolations } from "./carriage";
import type { ChosenFile } from "./composeattachments";

type Carriage = Parameters<typeof carriageViolations>[0];

// A transport that carries generously: every bound off except the aggregate,
// which is always positive. Each test tightens the one bound it is about, so a
// violation it asserts can only be the bound it set.
const GENEROUS: Carriage = {
  carries: true,
  max_files: 0,
  max_bytes_per_file: 0,
  max_total_bytes: 1_000_000_000,
  max_body_with_files: 0,
};

const file = (over: Partial<ChosenFile> = {}): ChosenFile => ({
  id: "att-1",
  filename: "offer.pdf",
  byteSize: 1000,
  ...over,
});

const kinds = (violations: readonly CarriageViolation[]) =>
  violations.map((violation) => violation.kind);

describe("carriageViolations", () => {
  it("finds nothing when no file is attached, whatever the bounds", () => {
    // A text-only message is not the carriage gate's business — the caption
    // bound only bites once a file rides with it.
    const carriage: Carriage = {
      ...GENEROUS,
      carries: false,
      max_body_with_files: 1,
    };
    expect(
      carriageViolations(carriage, [], "a body far past the caption bound"),
    ).toEqual([]);
  });

  it("reports carries and nothing else when the transport takes no files", () => {
    // The wall supersedes every fixable bound: offering "send fewer" for a
    // transport that sends none is a fix for a problem the rep cannot have.
    const carriage: Carriage = {
      ...GENEROUS,
      carries: false,
      max_files: 1,
      max_total_bytes: 1,
    };
    const found = carriageViolations(
      carriage,
      [file(), file({ id: "att-2" })],
      "hi",
    );
    expect(found).toEqual([{ kind: "carries", count: 2 }]);
  });

  it("reports too many files against the transport's own cap", () => {
    const carriage: Carriage = { ...GENEROUS, max_files: 1 };
    const found = carriageViolations(
      carriage,
      [file(), file({ id: "att-2" })],
      "hi",
    );
    expect(found).toEqual([{ kind: "count", limit: 1, named: 2 }]);
  });

  it("reports each file over the per-file cap by name", () => {
    const carriage: Carriage = { ...GENEROUS, max_bytes_per_file: 500 };
    const found = carriageViolations(
      carriage,
      [
        file({ filename: "big-a.pdf", byteSize: 900 }),
        file({ id: "att-2", filename: "ok.pdf", byteSize: 100 }),
        file({ id: "att-3", filename: "big-b.pdf", byteSize: 700 }),
      ],
      "hi",
    );
    expect(found).toEqual([
      { kind: "perFile", filename: "big-a.pdf", limit: 500 },
      { kind: "perFile", filename: "big-b.pdf", limit: 500 },
    ]);
  });

  it("reports the aggregate when files each under the per-file cap still total too much", () => {
    // The bound the other two cannot see between them: three files each under
    // the per-file cap that together pass the aggregate.
    const carriage: Carriage = {
      ...GENEROUS,
      max_bytes_per_file: 400,
      max_total_bytes: 1000,
    };
    const found = carriageViolations(
      carriage,
      [
        file({ byteSize: 400 }),
        file({ id: "att-2", byteSize: 400 }),
        file({ id: "att-3", byteSize: 400 }),
      ],
      "hi",
    );
    expect(found).toEqual([
      { kind: "aggregate", total: 1200, limit: 1000, count: 3 },
    ]);
  });

  it("counts the caption in runes, so an emoji is one character", () => {
    // Four code points, not the eight UTF-16 units the astral pairs would
    // count, matching the server's rune count.
    const carriage: Carriage = { ...GENEROUS, max_body_with_files: 3 };
    const found = carriageViolations(carriage, [file()], "🙂🙂🙂🙂");
    expect(found).toEqual([{ kind: "caption", limit: 3, length: 4 }]);
  });

  it("leaves the caption alone at exactly the bound", () => {
    const carriage: Carriage = { ...GENEROUS, max_body_with_files: 4 };
    expect(carriageViolations(carriage, [file()], "🙂🙂🙂🙂")).toEqual([]);
  });

  it("does not count an unmeasured file against a byte bound", () => {
    // The server refuses an empty file at staging; an unknown SIZE is not that,
    // and the client must not block a send the server would resolve and take.
    const carriage: Carriage = {
      ...GENEROUS,
      max_bytes_per_file: 500,
      max_total_bytes: 500,
    };
    expect(
      carriageViolations(carriage, [file({ byteSize: null })], "hi"),
    ).toEqual([]);
  });

  it("reports every broken bound at once, so the rep fixes them in one pass", () => {
    const carriage: Carriage = {
      carries: true,
      max_files: 1,
      max_bytes_per_file: 500,
      max_total_bytes: 800,
      max_body_with_files: 2,
    };
    const found = carriageViolations(
      carriage,
      [
        file({ filename: "big.pdf", byteSize: 600 }),
        file({ id: "att-2", byteSize: 400 }),
      ],
      "too long",
    );
    expect(kinds(found)).toEqual(["count", "perFile", "aggregate", "caption"]);
  });
});
