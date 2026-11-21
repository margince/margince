// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { PAGE_WAIT_DELAY_MS, PageWait, usePageWait } from "./pagewait";

function Frame({ pending }: Readonly<{ pending: boolean }>) {
  const waiting = usePageWait(pending);
  return <PageWait waiting={waiting} label="Loading Account…" />;
}

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("PageWait", () => {
  it("keeps its status region mounted and empty while nothing waits", () => {
    render(<Frame pending={false} />);
    expect(screen.getByRole("status")).toHaveTextContent("");
    expect(document.querySelector(".pagewait-line")).toBeNull();
  });

  it("shows the line and names the page once the wait has lasted the delay", () => {
    render(<Frame pending />);

    act(() => vi.advanceTimersByTime(PAGE_WAIT_DELAY_MS - 1));
    expect(screen.getByRole("status")).toHaveTextContent("");

    act(() => vi.advanceTimersByTime(1));
    expect(screen.getByRole("status")).toHaveTextContent("Loading Account…");
    expect(document.querySelector(".pagewait-line")).not.toBeNull();
  });

  it("starts the delay over for the next wait", () => {
    const { rerender } = render(<Frame pending />);
    act(() => vi.advanceTimersByTime(PAGE_WAIT_DELAY_MS));
    rerender(<Frame pending={false} />);
    expect(screen.getByRole("status")).toHaveTextContent("");

    rerender(<Frame pending />);
    expect(screen.getByRole("status")).toHaveTextContent("");
    act(() => vi.advanceTimersByTime(PAGE_WAIT_DELAY_MS));
    expect(screen.getByRole("status")).toHaveTextContent("Loading Account…");
  });
});
