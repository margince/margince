// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { NextBar } from "./nextbar";

afterEach(cleanup);

describe("the Next bar", () => {
  it("says nothing is due rather than drawing an empty move", () => {
    render(<NextBar />);
    expect(screen.getByText("Nothing is due next.")).toBeInTheDocument();
    expect(screen.queryByRole("button")).toBeNull();
  });

  it("draws the late tag, the reason, Snooze and the other tasks", () => {
    const onSnooze = vi.fn();
    const onOpen = vi.fn();
    render(
      <NextBar
        title="Send the kickoff agenda"
        late={{ label: "41 days late", tone: "danger" }}
        sub="Due 3 Jul · assigned to you"
        snooze={{ onSnooze }}
        more={{ count: 1, onOpen }}
      />,
    );
    expect(screen.getByText("41 days late")).toBeInTheDocument();
    screen.getByRole("button", { name: "Snooze" }).click();
    screen.getByRole("button", { name: "+1 task" }).click();
    expect(onSnooze).toHaveBeenCalledOnce();
    expect(onOpen).toHaveBeenCalledOnce();
  });

  it("draws no task link when there is no other task", () => {
    render(
      <NextBar title="Reply to Dana" more={{ count: 0, onOpen: vi.fn() }} />,
    );
    expect(screen.queryByRole("button", { name: /task/ })).toBeNull();
  });

  it("names the other tasks when the link is focused", async () => {
    const user = userEvent.setup();
    render(
      <NextBar
        title="Send the quote"
        more={{
          count: 2,
          onOpen: vi.fn(),
          peek: {
            head: "Other open tasks",
            items: [
              { key: "a", title: "Book the site visit", sub: "28 Oct" },
              { key: "b", title: "Update the capacity sheet", sub: "No date" },
            ],
          },
        }}
      />,
    );

    await user.tab();

    const card = screen.getByRole("region", { name: "Other open tasks" });
    expect(card).toHaveTextContent("Book the site visit");
    expect(card).toHaveTextContent("Update the capacity sheet");
  });
});
