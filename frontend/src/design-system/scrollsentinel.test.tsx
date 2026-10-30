// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { act, cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ScrollSentinel, type ScrollSentinelProps } from "./scrollsentinel";

// The browser's IntersectionObserver is the boundary: each test says when the
// marker is in view, and the sentinel decides whether that asks for a batch.
class ViewportObserver {
  static live: ViewportObserver[] = [];
  private connected = true;
  constructor(private readonly callback: IntersectionObserverCallback) {
    ViewportObserver.live.push(this);
  }
  observe() {}
  unobserve() {}
  takeRecords() {
    return [];
  }
  disconnect() {
    this.connected = false;
  }
  report(isIntersecting: boolean) {
    if (!this.connected) return;
    const entry = { isIntersecting } as IntersectionObserverEntry;
    this.callback([entry], this as unknown as IntersectionObserver);
  }
}

/** The marker enters view, as every connected observer would report it. */
function inView(isIntersecting = true) {
  act(() => {
    for (const observer of [...ViewportObserver.live]) {
      observer.report(isIntersecting);
    }
  });
}

beforeEach(() => {
  ViewportObserver.live = [];
  vi.stubGlobal("IntersectionObserver", ViewportObserver);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function sentinel(props: Partial<ScrollSentinelProps> = {}) {
  const onLoadMore = props.onLoadMore ?? vi.fn();
  const all: ScrollSentinelProps = {
    list: "today",
    cursor: "c1",
    loading: false,
    failed: false,
    end: "All loaded",
    ...props,
    onLoadMore,
  };
  const view = render(
    <LocaleProvider initial="en">
      <ScrollSentinel {...all} />
    </LocaleProvider>,
  );
  const update = (next: Partial<ScrollSentinelProps>) =>
    view.rerender(
      <LocaleProvider initial="en">
        <ScrollSentinel {...all} {...next} />
      </LocaleProvider>,
    );
  return { onLoadMore, update };
}

describe("ScrollSentinel", () => {
  it("asks for the next batch once when the marker comes into view", () => {
    const { onLoadMore } = sentinel();
    inView(false);
    expect(onLoadMore).not.toHaveBeenCalled();
    inView();
    inView();
    expect(onLoadMore).toHaveBeenCalledTimes(1);
  });

  it("keeps asking while a short list leaves the marker in view", () => {
    const { onLoadMore, update } = sentinel();
    inView();
    update({ loading: true });
    inView();
    update({ cursor: "c2", loading: false });
    inView();
    expect(onLoadMore).toHaveBeenCalledTimes(2);
  });

  it("asks nothing while a batch is in flight", () => {
    const { onLoadMore } = sentinel({ loading: true });
    inView();
    expect(onLoadMore).not.toHaveBeenCalled();
    expect(screen.getByText("Loading more…")).toBeInTheDocument();
  });

  it("stops after a failure and retries the same continuation on request", async () => {
    const { onLoadMore, update } = sentinel();
    inView();
    update({ loading: true });
    update({ loading: false, failed: true });
    inView();
    expect(onLoadMore).toHaveBeenCalledTimes(1);
    expect(screen.getByText("Could not load more.")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Retry" }));
    expect(onLoadMore).toHaveBeenCalledTimes(2);
  });

  it("offers recovery instead of looping when an answered batch did not move the cursor", () => {
    const { onLoadMore, update } = sentinel();
    inView();
    update({ loading: true });
    update({ loading: false });
    inView();
    inView();
    expect(onLoadMore).toHaveBeenCalledTimes(1);
    expect(
      screen.getByText("The list stopped moving forward. Retry to continue."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
  });

  it("says the list is complete only once no continuation is left", () => {
    const { onLoadMore } = sentinel({ cursor: undefined });
    inView();
    expect(onLoadMore).not.toHaveBeenCalled();
    expect(screen.getByText("All loaded")).toBeInTheDocument();
  });

  it("starts a new list fresh when the list changes", () => {
    const { onLoadMore, update } = sentinel();
    inView();
    update({ loading: true });
    update({ loading: false });
    inView();
    update({ list: "decisions" });
    inView();
    expect(onLoadMore).toHaveBeenCalledTimes(2);
  });

  it("offers a button where the browser cannot observe the viewport", async () => {
    vi.unstubAllGlobals();
    vi.stubGlobal("IntersectionObserver", undefined);
    const { onLoadMore } = sentinel();
    await userEvent.click(screen.getByRole("button", { name: "Load more" }));
    expect(onLoadMore).toHaveBeenCalledTimes(1);
  });
});
