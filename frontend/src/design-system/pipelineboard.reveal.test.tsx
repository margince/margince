/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { type Locale, LocaleProvider } from "../i18n";
import { AnsweredSearch } from "./answeredsearch";
import { ListTable } from "./listtable";
import {
  type BoardColumn,
  type BoardRecord,
  PipelineBoard,
} from "./pipelineboard";
import { matchingColumns } from "./pipelinereveal";

// A search whose only matches sit in an off-screen stage read as an empty
// board. The board says where the matches are and brings that stage into view.

const column = (
  stage: string,
  label: string,
  deals: BoardRecord[],
): BoardColumn<BoardRecord> => ({ stage, label, deals });

const won = [
  { id: "d-1", name: "Fraunhofer tender" },
  { id: "d-2", name: "Fraunhofer renewal" },
  { id: "d-3", name: "Fraunhofer pilot" },
];

// Matches only in the last column, the shape the report described.
const columns = [
  column("open", "Discovery", []),
  column("qualified", "Qualified", []),
  column("proposal", "Proposal", []),
  column("won", "Closed Won", won),
];

// How much of each stage shows inside the board, as the browser would measure
// it. The board's own IntersectionObserver is the boundary under test.
let shown: Record<string, number> = {};
let scrolled: string[] = [];

class MeasuredObserver {
  private readonly targets: Element[] = [];
  private reported = false;
  constructor(private readonly callback: IntersectionObserverCallback) {}
  observe(target: Element) {
    this.targets.push(target);
    if (this.reported) return;
    this.reported = true;
    queueMicrotask(() =>
      this.callback(
        this.targets.map(
          (node) =>
            ({
              target: node,
              intersectionRatio:
                shown[(node as HTMLElement).dataset.stage ?? ""] ?? 1,
            }) as IntersectionObserverEntry,
        ),
        this as unknown as IntersectionObserver,
      ),
    );
  }
  disconnect() {}
  unobserve() {}
  takeRecords() {
    return [];
  }
}

beforeEach(() => {
  shown = { won: 0 };
  scrolled = [];
  vi.stubGlobal("IntersectionObserver", MeasuredObserver);
  vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(
    function (this: HTMLElement) {
      scrolled.push(this.dataset.stage ?? "");
    },
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function board(revealKey?: string, locale: Locale = "en", stages = columns) {
  return (
    <LocaleProvider initial={locale}>
      <AnsweredSearch.Provider value={revealKey}>
        <PipelineBoard
          variant="plain"
          columns={stages}
          renderCard={(record) => <span>{record.name}</span>}
        />
      </AnsweredSearch.Provider>
    </LocaleProvider>
  );
}

describe("matchingColumns", () => {
  it("names the stages holding a card, in board order", () => {
    expect(matchingColumns(columns).map((c) => c.stage)).toEqual(["won"]);
  });

  it("names nothing when every stage is empty", () => {
    expect(matchingColumns([column("open", "Open", [])])).toEqual([]);
  });
});

describe("PipelineBoard search off screen", () => {
  it("says where the matches are and scrolls their stage into view", async () => {
    render(board("fraunhofer"));
    const hint = await screen.findByRole("status");
    expect(hint.textContent).toContain("3 results in Closed Won");
    expect(scrolled).toEqual(["won"]);
  });

  it("sets the note in the wrapper a list surface gives the board's gutter", async () => {
    render(board("fraunhofer"));
    const hint = await screen.findByRole("status");
    expect(hint.parentElement?.className).toBe("board-reveal");
    expect(hint.parentElement?.nextElementSibling?.className).toBe("board");
  });

  it("names the column in the reader's language", async () => {
    render(board("fraunhofer", "vi"));
    const hint = await screen.findByRole("status");
    expect(hint.textContent).toContain("3 kết quả ở cột Closed Won");
  });

  it("takes the reader back to the column on request", async () => {
    const user = userEvent.setup();
    render(board("fraunhofer"));
    await user.click(
      await screen.findByRole("button", { name: "Go to column" }),
    );
    expect(scrolled).toEqual(["won", "won"]);
  });

  it("says nothing and moves nothing when a match already shows", async () => {
    shown = { won: 1 };
    render(board("fraunhofer"));
    await Promise.resolve();
    expect(screen.queryByRole("status")).toBeNull();
    expect(scrolled).toEqual([]);
  });

  it("checks once per search, not again when more results load", async () => {
    const { rerender } = render(board("fraunhofer"));
    await screen.findByRole("status");
    const moreLoaded = [
      column("open", "Discovery", [{ id: "d-4", name: "Fraunhofer lab" }]),
      ...columns.slice(1),
    ];
    rerender(board("fraunhofer", "en", moreLoaded));
    await Promise.resolve();
    expect(scrolled).toEqual(["won"]);
  });

  it("leaves the board where it is without a search", async () => {
    render(board(undefined));
    await Promise.resolve();
    expect(screen.queryByRole("status")).toBeNull();
    expect(scrolled).toEqual([]);
  });
});

// No board takes the search as a prop. The list it is drawn in hands it over,
// so no screen can leave the note out.
describe("a board drawn as a searchable list's body", () => {
  function listBody(search: { value: string; answered: string }) {
    return (
      <LocaleProvider initial="en">
        <ListTable<BoardRecord>
          rows={[]}
          columns={[]}
          rowKey={(row) => row.id}
          unit="deals"
          search={{ ...search, onChange: () => undefined }}
          body={
            <PipelineBoard
              variant="plain"
              columns={columns}
              renderCard={(record) => <span>{record.name}</span>}
            />
          }
        />
      </LocaleProvider>
    );
  }

  it("says where the matches are for the search the rows answer", async () => {
    render(listBody({ value: "fraunhofer", answered: "fraunhofer" }));
    const hint = await screen.findByRole("status");
    expect(hint.textContent).toContain("3 results in Closed Won");
  });

  it("waits for the typing to settle before it looks", async () => {
    render(listBody({ value: "fraunhofer", answered: "" }));
    await Promise.resolve();
    expect(screen.queryByRole("status")).toBeNull();
    expect(scrolled).toEqual([]);
  });
});
