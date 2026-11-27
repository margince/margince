/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { en as englishCopy } from "../i18n/en";
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

function Controlled({
  onChange,
}: Readonly<{ onChange: (q?: string) => void }>) {
  const [filters, setFilters] = useState<TimelineFilters>({});
  return (
    <LocaleProvider initial="en">
      <TimelineFilterBar
        value={filters}
        onChange={(next) => {
          onChange(next.q);
          setFilters(next);
        }}
      />
    </LocaleProvider>
  );
}

const field = (): HTMLInputElement => {
  const box = screen.getByLabelText(englishCopy["timeline.filters.search"]);
  if (!(box instanceof HTMLInputElement)) {
    throw new Error("the search field is not an input");
  }
  return box;
};

// userEvent under fake timers waits on a timer that testing-library only
// drains for jest, so keys are sent as change events.
const typeInto = (text: string) =>
  fireEvent.change(field(), { target: { value: field().value + text } });
const pause = (ms = SEARCH_DEBOUNCE_MS) =>
  act(() => vi.advanceTimersByTimeAsync(ms));

describe("TimelineFilterBar search", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it("commits once after typing pauses", async () => {
    const { onChange } = mount();
    typeInto("cut");
    expect(onChange).not.toHaveBeenCalled();
    await pause();
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledWith({ q: "cut" });
  });

  it("restarts the pause when typing continues", async () => {
    const { onChange } = mount();
    typeInto("cu");
    await pause(SEARCH_DEBOUNCE_MS - 1);
    typeInto("t");
    await pause(SEARCH_DEBOUNCE_MS - 1);
    expect(onChange).not.toHaveBeenCalled();
    await pause(1);
    expect(onChange).toHaveBeenCalledTimes(1);
    expect(onChange).toHaveBeenCalledWith({ q: "cut" });
  });

  it("commits Enter at once and not again after the pause", async () => {
    const { onChange, rerender } = mount();
    typeInto("cut");
    fireEvent.keyDown(field(), { key: "Enter" });
    expect(onChange).toHaveBeenCalledTimes(1);
    rerender({ q: "cut" });
    await pause(SEARCH_DEBOUNCE_MS * 2);
    expect(onChange).toHaveBeenCalledTimes(1);
  });

  it("commits on blur at once and not again after the pause", async () => {
    const { onChange, rerender } = mount();
    typeInto("cut");
    fireEvent.blur(field());
    expect(onChange).toHaveBeenCalledTimes(1);
    rerender({ q: "cut" });
    await pause(SEARCH_DEBOUNCE_MS * 2);
    expect(onChange).toHaveBeenCalledTimes(1);
  });

  it("commits a cleared field after the pause", async () => {
    const { onChange } = mount({ q: "cut" });
    fireEvent.change(field(), { target: { value: "" } });
    expect(onChange).not.toHaveBeenCalled();
    await pause();
    expect(onChange).toHaveBeenCalledWith({ q: "" });
  });

  it("does not commit a word already applied", async () => {
    const { onChange } = mount({ q: "cut" });
    typeInto(" ");
    await pause(SEARCH_DEBOUNCE_MS * 2);
    expect(onChange).not.toHaveBeenCalled();
  });

  it("does not write a stale draft over an outside reset", async () => {
    const { onChange, rerender } = mount({ q: "old" });
    typeInto("x");
    rerender({});
    await pause(SEARCH_DEBOUNCE_MS * 2);
    expect(onChange).not.toHaveBeenCalled();
    expect(field().value).toBe("");
  });

  it("waits for an input method to finish composing", async () => {
    const { onChange } = mount();
    fireEvent.compositionStart(field());
    typeInto("cut");
    await pause(SEARCH_DEBOUNCE_MS * 2);
    expect(onChange).not.toHaveBeenCalled();
    fireEvent.compositionEnd(field());
    await pause();
    expect(onChange).toHaveBeenCalledWith({ q: "cut" });
  });

  it("commits one settled word once while the parent has not stored it", async () => {
    const { onChange, rerender } = mount();
    typeInto("cut");
    await pause();
    rerender({});
    rerender({});
    rerender({});
    expect(onChange).toHaveBeenCalledTimes(1);
  });

  it("drops a pending word when the record changes", async () => {
    const { onChange, rerender } = mount();
    typeInto("acme");
    rerender({}, "b");
    await pause(SEARCH_DEBOUNCE_MS * 2);
    expect(onChange).not.toHaveBeenCalled();
    expect(field().value).toBe("");
  });

  it("commits after blur even if a composition never ended", async () => {
    const { onChange, rerender } = mount();
    fireEvent.compositionStart(field());
    typeInto("cut");
    fireEvent.blur(field());
    rerender({ q: "cut" });
    typeInto("x");
    await pause();
    expect(onChange).toHaveBeenLastCalledWith({ q: "cutx" });
  });

  it("keeps the space typed after a word once the parent stores the pause's commit", async () => {
    const onChange = vi.fn();
    render(<Controlled onChange={onChange} />);
    typeInto("fleet ");
    await pause();
    typeInto("renewal");
    await pause();
    expect(onChange).toHaveBeenLastCalledWith("fleet renewal");
    expect(field().value).toBe("fleet renewal");
  });

  it("keeps the space typed after a word once the parent stores an Enter commit", async () => {
    render(<Controlled onChange={vi.fn()} />);
    typeInto("fleet ");
    fireEvent.keyDown(field(), { key: "Enter" });
    typeInto("renewal");
    expect(field().value).toBe("fleet renewal");
  });

  it("keeps the space typed after a word once the parent stores a blur commit", async () => {
    render(<Controlled onChange={vi.fn()} />);
    typeInto("fleet ");
    fireEvent.blur(field());
    typeInto("renewal");
    expect(field().value).toBe("fleet renewal");
  });
});
