/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import {
  type BoardColumn,
  type BoardRecord,
  PipelineBoard,
} from "./pipelineboard";

// An opened fold takes a whole stage width and can end past the board's edge.
// So the board brings the stage the reader just opened into view.

let scrolled: string[] = [];

beforeEach(() => {
  scrolled = [];
  vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(
    function (this: HTMLElement) {
      scrolled.push(this.dataset.stage ?? "");
    },
  );
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// The lead board's shape: open stages beside a terminal one that folds. Its
// caller may refuse to open a fold it is asked about.
function FoldingBoard({ opens = true }: Readonly<{ opens?: boolean }>) {
  const [open, setOpen] = useState(false);
  const columns: BoardColumn<BoardRecord>[] = [
    { stage: "new", label: "New", deals: [{ id: "l-1", name: "Jonas" }] },
    {
      stage: "disqualified",
      label: "Disqualified",
      count: 4,
      collapsed: !open,
      deals: open ? [{ id: "l-2", name: "Petra" }] : [],
    },
  ];
  return (
    <LocaleProvider initial="en">
      <PipelineBoard
        variant="plain"
        columns={columns}
        renderCard={(record) => <span>{record.name}</span>}
        onToggleColumn={() => {
          if (opens) setOpen((was) => !was);
        }}
      />
    </LocaleProvider>
  );
}

const fold = () => screen.getByRole("button", { name: /Disqualified/ });

describe("PipelineBoard unfolding a stage", () => {
  it("brings the opened stage into view", async () => {
    const user = userEvent.setup();
    render(<FoldingBoard />);
    await user.click(fold());
    expect(await screen.findByText("Petra")).toBeTruthy();
    expect(scrolled).toEqual(["disqualified"]);
  });

  it("does the same from the keyboard", async () => {
    const user = userEvent.setup();
    render(<FoldingBoard />);
    fold().focus();
    await user.keyboard("{Enter}");
    expect(await screen.findByText("Petra")).toBeTruthy();
    expect(scrolled).toEqual(["disqualified"]);
  });

  it("leaves the board where it is when a stage folds again", async () => {
    const user = userEvent.setup();
    render(<FoldingBoard />);
    await user.click(fold());
    await screen.findByText("Petra");
    await user.click(fold());
    expect(fold().getAttribute("aria-expanded")).toBe("false");
    expect(scrolled).toEqual(["disqualified"]);
  });

  it("moves nothing when the caller keeps the stage folded", async () => {
    const user = userEvent.setup();
    render(<FoldingBoard opens={false} />);
    await user.click(fold());
    expect(fold().getAttribute("aria-expanded")).toBe("false");
    expect(scrolled).toEqual([]);
  });
});
