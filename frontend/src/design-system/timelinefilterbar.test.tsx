/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { SEARCH_DEBOUNCE_MS } from "./debouncedsearch";
import { NO_TIMELINE_FILTERS, type TimelineFilters } from "./recordtimeline";
import { TimelineFilterBar } from "./timelinefilterbar";

function mount(value: TimelineFilters = NO_TIMELINE_FILTERS) {
  const onChange = vi.fn();
  const ui = (v: TimelineFilters, recordId = "a") => (
    <LocaleProvider initial="en">
      <TimelineFilterBar key={recordId} value={v} onChange={onChange} />
    </LocaleProvider>
  );
  const view = render(ui(value));
  return {
    onChange,
    rerender: (v: TimelineFilters, recordId?: string) =>
      view.rerender(ui(v, recordId)),
  };
}

const field = () => screen.getByLabelText("Search this timeline");

describe("TimelineFilterBar search", () => {
  beforeEach(() =>
    vi.useFakeTimers({ shouldAdvanceTime: true, advanceTimeDelta: 5 }),
  );
  afterEach(() => vi.useRealTimers());

  it("commits once after typing pauses", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onChange } = mount();
    await user.type(field(), "cut");
    expect(onChange).not.toHaveBeenCalled();
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS));
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledWith({ q: "cut" });
  });

  it("restarts the pause when typing continues", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onChange } = mount();
    await user.type(field(), "cu");
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS - 50));
    await user.type(field(), "t");
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS - 50));
    expect(onChange).not.toHaveBeenCalled();
    await act(() => vi.advanceTimersByTimeAsync(50));
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledWith({ q: "cut" });
  });

  it("commits Enter at once and not again after the pause", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onChange, rerender } = mount();
    await user.type(field(), "cut{Enter}");
    expect(onChange).toHaveBeenCalledTimes(1);
    rerender({ q: "cut" });
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS * 2));
    expect(onChange).toHaveBeenCalledTimes(1);
  });

  it("commits on blur at once and not again after the pause", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onChange, rerender } = mount();
    await user.type(field(), "cut");
    await user.tab();
    expect(onChange).toHaveBeenCalledTimes(1);
    rerender({ q: "cut" });
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS * 2));
    expect(onChange).toHaveBeenCalledTimes(1);
  });

  it("commits a cleared field after the pause", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onChange } = mount({ q: "cut" });
    await user.clear(field());
    expect(onChange).not.toHaveBeenCalled();
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS));
    expect(onChange).toHaveBeenCalledWith({ q: "" });
  });

  it("does not commit a word already applied", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onChange } = mount({ q: "cut" });
    await user.type(field(), " ");
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS * 2));
    expect(onChange).not.toHaveBeenCalled();
  });

  it("does not write a stale draft over an outside reset", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onChange, rerender } = mount({ q: "old" });
    await user.type(field(), "x");
    rerender({});
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS * 2));
    expect(onChange).not.toHaveBeenCalled();
    expect((field() as HTMLInputElement).value).toBe("");
  });

  it("waits for an input method to finish composing", async () => {
    const { onChange } = mount();
    fireEvent.compositionStart(field());
    fireEvent.change(field(), { target: { value: "cut" } });
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS * 2));
    expect(onChange).not.toHaveBeenCalled();
    fireEvent.compositionEnd(field());
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS));
    expect(onChange).toHaveBeenCalledWith({ q: "cut" });
  });

  it("commits one settled word once while the parent has not stored it", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onChange, rerender } = mount();
    await user.type(field(), "cut");
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS));
    rerender({});
    rerender({});
    rerender({});
    expect(onChange).toHaveBeenCalledTimes(1);
  });

  it("drops a pending word when the record changes", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onChange, rerender } = mount();
    await user.type(field(), "acme");
    rerender({}, "b");
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS * 2));
    expect(onChange).not.toHaveBeenCalled();
    expect((field() as HTMLInputElement).value).toBe("");
  });

  it("commits after blur even if a composition never ended", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const { onChange, rerender } = mount();
    fireEvent.compositionStart(field());
    await user.type(field(), "cut");
    await user.tab();
    rerender({ q: "cut" });
    await user.type(field(), "x");
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS));
    expect(onChange).toHaveBeenLastCalledWith({ q: "cutx" });
  });
});
