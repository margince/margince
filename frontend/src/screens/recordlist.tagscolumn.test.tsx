// @vitest-environment jsdom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { LocaleProvider, useT } from "../i18n";
import { tagsColumn } from "./recordlist";

// The column three lists share. It lives in recordlist.tsx rather than in each
// screen for the reason that file was written: three lists once carried three
// copies of the Owner column, and the fix reached two of them.

type Row = { tags?: readonly { tag_id: string; name: string }[] };

function Harness({
  onColumn,
}: Readonly<{ onColumn: (sort?: string) => void }>) {
  const t = useT();
  const column = tagsColumn<Row>(t);
  onColumn(column.sort);
  return (
    <div>
      <span>{column.header}</span>
      <span>
        {column.cell({ tags: [{ tag_id: "t-1", name: "Key Account" }] })}
      </span>
    </div>
  );
}

describe("the shared Tags column", () => {
  it("draws the row's words in the cell, and does not offer a sort", () => {
    // Sorting a multi-value cell has to pick one of its values to sort by, and
    // whichever it picks the resulting order is a claim the row does not make.
    let sort: string | undefined = "unset";
    render(
      <LocaleProvider initial="en">
        <Harness
          onColumn={(value) => {
            sort = value;
          }}
        />
      </LocaleProvider>,
    );
    expect(screen.getByText("Key Account")).toBeInTheDocument();
    expect(sort).toBeUndefined();
  });
});
