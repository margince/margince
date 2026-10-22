/** @vitest-environment happy-dom */
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import {
  type BoardColumn,
  type BoardRecord,
  firstFilledColumn,
  PipelineBoard,
} from "./pipelineboard";

// A search whose only match sits in an off-screen stage read as an empty
// board. The board brings the first stage holding a card into view.

const column = (
  stage: string,
  deals: BoardRecord[],
): BoardColumn<BoardRecord> => ({ stage, label: stage, deals });

const columns = [
  column("open", []),
  column("qualified", []),
  column("lost", [{ id: "d-1", name: "Fraunhofer tender" }]),
];

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function board(revealKey?: string) {
  return render(
    <LocaleProvider initial="en">
      <PipelineBoard
        variant="plain"
        columns={columns}
        renderCard={(record) => <span>{record.name}</span>}
        revealKey={revealKey}
      />
    </LocaleProvider>,
  );
}

describe("firstFilledColumn", () => {
  it("names the first stage holding a card", () => {
    expect(firstFilledColumn(columns)).toBe("lost");
  });

  it("names nothing when every stage is empty", () => {
    expect(firstFilledColumn([column("open", [])])).toBeUndefined();
  });
});

describe("PipelineBoard reveal", () => {
  it("scrolls the matching stage into view when a search lands", () => {
    const scrolled: string[] = [];
    vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(
      function (this: HTMLElement) {
        scrolled.push(this.dataset.stage ?? "");
      },
    );
    board("frau");
    expect(scrolled).toEqual(["lost"]);
  });

  it("reveals once per search, not again when more results load", () => {
    const scrolled: string[] = [];
    vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(
      function (this: HTMLElement) {
        scrolled.push(this.dataset.stage ?? "");
      },
    );
    const { rerender } = board("frau");
    const moreLoaded = [
      column("open", [{ id: "d-2", name: "Frauke renewal" }]),
      ...columns.slice(1),
    ];
    rerender(
      <LocaleProvider initial="en">
        <PipelineBoard
          variant="plain"
          columns={moreLoaded}
          renderCard={(record) => <span>{record.name}</span>}
          revealKey="frau"
        />
      </LocaleProvider>,
    );
    expect(scrolled).toEqual(["lost"]);
  });

  it("leaves the board where it is without a search", () => {
    const scrolled: string[] = [];
    vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(
      function (this: HTMLElement) {
        scrolled.push(this.dataset.stage ?? "");
      },
    );
    board(undefined);
    expect(scrolled).toEqual([]);
  });
});
