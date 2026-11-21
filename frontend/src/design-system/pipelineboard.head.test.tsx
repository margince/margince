/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import {
  type BoardColumn,
  type BoardRecord,
  PipelineBoard,
} from "./pipelineboard";

const LABEL = "Contract negotiation and legal review";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

// jsdom lays nothing out; these are the two measurements a clip is read from.
function stubWidths({ scroll, client }: { scroll: number; client: number }) {
  vi.spyOn(HTMLElement.prototype, "scrollWidth", "get").mockReturnValue(scroll);
  vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(client);
}

function board(column: Partial<BoardColumn<BoardRecord>>, folds = false) {
  render(
    <LocaleProvider initial="en">
      <PipelineBoard
        variant="plain"
        columns={[{ stage: "review", label: LABEL, deals: [], ...column }]}
        renderCard={(record) => <span>{record.name}</span>}
        onToggleColumn={folds ? () => undefined : undefined}
      />
    </LocaleProvider>,
  );
  const head = document.querySelector<HTMLElement>(".board-col-head");
  if (!head) throw new Error("the board drew no stage head");
  return head;
}

describe("a board column's head", () => {
  it("shows a stage name it cut short in full on focus", () => {
    stubWidths({ scroll: 480, client: 220 });
    const head = board({});
    expect(head).toHaveAttribute("tabindex", "0");

    act(() => head.focus());

    expect(screen.getByRole("tooltip")).toHaveTextContent(LABEL);
    expect(head).toHaveAccessibleDescription(LABEL);
  });

  it("takes no tab stop while the stage name fits", () => {
    stubWidths({ scroll: 220, client: 220 });
    expect(board({})).not.toHaveAttribute("tabindex");
  });

  it("does not describe a folding head by the name it is already called", () => {
    stubWidths({ scroll: 480, client: 220 });
    const head = board({ collapsed: false }, true);
    expect(head).toHaveAttribute("role", "button");

    act(() => head.focus());

    expect(screen.getByRole("tooltip")).toHaveTextContent(LABEL);
    expect(head).not.toHaveAttribute("aria-describedby");
  });
});
