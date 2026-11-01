// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {
  SummaryFields,
  type SummaryParts,
  summaryFieldsLine,
  summaryPartsWithout,
  summarySentence,
} from "./approvalsummary";

afterEach(cleanup);

const words = {
  field: (name: string) => (name === "name" ? "Name" : name),
  more: (count: number) => `${count} more not shown`,
};

const parts: SummaryParts = {
  sentence: "Update record deal",
  fields: [
    { field: "name", value: "Acme, Inc. renewal" },
    { field: "owner_id", value: "null" },
  ],
  more_fields: 3,
};

describe("an approval summary taken apart", () => {
  it("heads with the sentence, and an old row reads as its whole summary", () => {
    expect(
      summarySentence({
        summary: "Update record deal: name=Acme, Inc. renewal",
        summary_parts: parts,
      }),
    ).toBe("Update record deal");
    expect(summarySentence({ summary: "Archive the tag" })).toBe(
      "Archive the tag",
    );
    expect(summarySentence({})).toBeNull();
    expect(
      summarySentence({
        summary: "Archive the tag",
        summary_parts: { ...parts, sentence: "" },
      }),
    ).toBe("Archive the tag");
  });

  it("lists each value whole under the label the caller gives it", () => {
    const { container } = render(<SummaryFields parts={parts} words={words} />);
    const rows = [...container.querySelectorAll(".factlist-row")].map((row) =>
      [...row.children].map((cell) => cell.textContent),
    );
    expect(rows).toEqual([
      ["Name", "Acme, Inc. renewal"],
      ["owner_id", "null"],
      ["", "3 more not shown"],
    ]);
  });

  it("draws nothing for a summary that is a sentence alone", () => {
    const { container } = render(
      <SummaryFields parts={undefined} words={words} />,
    );
    expect(container.childElementCount).toBe(0);
  });

  it("leaves out the fields a surface already draws", () => {
    expect(summaryPartsWithout(parts, new Set(["name"]))?.fields).toEqual([
      { field: "owner_id", value: "null" },
    ]);
    expect(
      summaryPartsWithout(
        { ...parts, more_fields: 0 },
        new Set(["name", "owner_id"]),
      ),
    ).toBeUndefined();
  });

  it("puts the same fields on one line for a queue row", () => {
    expect(summaryFieldsLine(parts, words)).toBe(
      "Name: Acme, Inc. renewal · owner_id: null · 3 more not shown",
    );
    expect(summaryFieldsLine(undefined, words)).toBeUndefined();
  });
});
