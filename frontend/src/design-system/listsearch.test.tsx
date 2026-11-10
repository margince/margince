/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { SEARCH_DEBOUNCE_MS, type SearchResult } from "./debouncedsearch";
import { type ListChip, ListSurface } from "./listsurface";

const STAGE: ListChip = {
  key: "stage_id",
  label: "Stage",
  allLabel: "Any stage",
  options: [{ value: "proposal", label: "Proposal" }],
};

function pendingReply() {
  let receive: ((rows: readonly SearchResult[]) => void) | undefined;
  const promise = new Promise<readonly SearchResult[]>((resolve) => {
    receive = resolve;
  });
  return {
    promise,
    resolve: (rows: readonly SearchResult[]) => {
      if (!receive) throw new Error("the search reply has not been created");
      receive(rows);
    },
  };
}

function Surface({
  chips = [STAGE],
  onFilter,
}: Readonly<{
  chips?: readonly ListChip[];
  onFilter: (key: string, value: string) => void;
}>) {
  const [query, setQuery] = useState("");
  const [chosen, setChosen] = useState<Record<string, string>>({});
  return (
    <LocaleProvider initial="en">
      <ListSurface
        search={{ value: query, onChange: setQuery }}
        chips={chips}
        chosen={chosen}
        onChipChange={(key, value) => {
          onFilter(key, value);
          setChosen((previous) => ({ ...previous, [key]: value }));
        }}
      >
        <p>Rows</p>
      </ListSurface>
    </LocaleProvider>
  );
}

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  cleanup();
  vi.clearAllTimers();
  vi.useRealTimers();
});

it("picks the same chip value as View and clears the typed query", async () => {
  const onFilter = vi.fn();
  render(<Surface onFilter={onFilter} />);
  const field = screen.getByRole("combobox", { name: "Search" });
  fireEvent.focus(field);
  fireEvent.change(field, { target: { value: "prop" } });
  fireEvent.click(screen.getByRole("option", { name: "Stage is Proposal" }));
  expect(onFilter).toHaveBeenCalledWith("stage_id", "proposal");
  expect(field).toHaveValue("");
  expect(field).toHaveFocus();
  expect(
    screen.getByRole("group", { name: "Stage: Proposal" }),
  ).toBeInTheDocument();
});

it("keeps text search on Enter without a choice and on Escape", async () => {
  const onFilter = vi.fn();
  render(<Surface onFilter={onFilter} />);
  const field = screen.getByRole("combobox", { name: "Search" });
  fireEvent.focus(field);
  fireEvent.change(field, { target: { value: "prop" } });
  fireEvent.keyDown(field, { key: "Enter" });
  expect(field).toHaveValue("prop");
  expect(field).toHaveAttribute("aria-expanded", "false");
  expect(onFilter).not.toHaveBeenCalled();
  fireEvent.keyDown(field, { key: "ArrowDown" });
  fireEvent.keyDown(field, { key: "Escape" });
  expect(field).toHaveValue("prop");
  expect(field).toHaveAttribute("aria-expanded", "false");
  expect(onFilter).not.toHaveBeenCalled();
});

it("walks filter suggestions with arrows and Enter", async () => {
  const onFilter = vi.fn();
  render(<Surface onFilter={onFilter} />);
  const field = screen.getByRole("combobox", { name: "Search" });
  fireEvent.focus(field);
  fireEvent.change(field, { target: { value: "prop" } });
  fireEvent.keyDown(field, { key: "ArrowDown" });
  fireEvent.keyDown(field, { key: "Enter" });
  expect(onFilter).toHaveBeenCalledWith("stage_id", "proposal");
  expect(field).toHaveValue("");
});

it("asks once per pause and drops an older server answer", async () => {
  const old = pendingReply();
  const next = pendingReply();
  const search = vi
    .fn()
    .mockReturnValueOnce(old.promise)
    .mockReturnValueOnce(next.promise);
  const company: ListChip = {
    key: "company_id",
    label: "Company",
    allLabel: "Any company",
    options: [],
    search,
    appliedName: undefined,
  };
  const onFilter = vi.fn();
  const { rerender } = render(
    <Surface chips={[company]} onFilter={onFilter} />,
  );
  const field = screen.getByRole("combobox", { name: "Search" });
  fireEvent.focus(field);
  fireEvent.change(field, { target: { value: "a" } });
  fireEvent.change(field, { target: { value: "ac" } });
  expect(search).not.toHaveBeenCalled();
  await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS));
  expect(search).toHaveBeenCalledTimes(1);
  expect(search).toHaveBeenLastCalledWith("ac");
  rerender(<Surface chips={[{ ...company }]} onFilter={onFilter} />);
  await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS));
  expect(search).toHaveBeenCalledTimes(1);
  fireEvent.change(field, { target: { value: "acm" } });
  fireEvent.change(field, { target: { value: "acme" } });
  await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS));
  expect(search).toHaveBeenCalledTimes(2);
  expect(search).toHaveBeenLastCalledWith("acme");
  await act(async () => {
    next.resolve([{ value: "new", label: "Acme current" }]);
  });
  expect(
    screen.getByRole("option", { name: "Company is Acme current" }),
  ).toBeInTheDocument();
  await act(async () => {
    old.resolve([{ value: "old", label: "Acme stale" }]);
  });
  expect(
    screen.queryByRole("option", { name: "Company is Acme stale" }),
  ).not.toBeInTheDocument();
  fireEvent.click(
    screen.getByRole("option", { name: "Company is Acme current" }),
  );
  expect(onFilter).toHaveBeenCalledWith("company_id", "new");
});

it("retains ordinary search and reports failed filter suggestions", async () => {
  const search = vi.fn().mockRejectedValue(new Error("unavailable"));
  const company: ListChip = {
    key: "company_id",
    label: "Company",
    allLabel: "Any company",
    options: [],
    search,
    appliedName: undefined,
  };
  const onFilter = vi.fn();
  render(<Surface chips={[company]} onFilter={onFilter} />);
  const field = screen.getByRole("combobox", { name: "Search" });
  fireEvent.focus(field);
  fireEvent.change(field, { target: { value: "acme" } });
  await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS));
  expect(
    screen.getByRole("option", {
      name: /Search for “acme”.*Filter suggestions did not load/,
    }),
  ).toBeInTheDocument();
  fireEvent.keyDown(field, { key: "Enter" });
  expect(field).toHaveValue("acme");
  expect(onFilter).not.toHaveBeenCalled();
});

it("searches the value after a relation filter label", async () => {
  const search = vi.fn(async () => [{ value: "acme", label: "Acme" }]);
  render(
    <Surface
      onFilter={vi.fn()}
      chips={[
        {
          key: "company_id",
          label: "Company",
          allLabel: "Any company",
          options: [],
          search,
          appliedName: undefined,
        },
      ]}
    />,
  );
  const input = screen.getByRole("combobox", { name: "Search" });
  fireEvent.focus(input);
  fireEvent.change(input, { target: { value: "Company Acme" } });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS);
  });
  expect(search).toHaveBeenCalledExactlyOnceWith("Acme");
  expect(
    screen.getByRole("option", { name: "Company is Acme" }),
  ).toBeInTheDocument();
});

it("omits applied values and bounds the filter offers", () => {
  const options = Array.from({ length: 25 }, (_, index) => ({
    value: String(index),
    label: `Colleague ${index}`,
  }));
  render(
    <Surface
      onFilter={vi.fn()}
      chips={[
        { key: "owner_id", label: "Owner", allLabel: "Any owner", options },
      ]}
    />,
  );
  const input = screen.getByRole("combobox", { name: "Search" });
  fireEvent.focus(input);
  fireEvent.change(input, { target: { value: "Owner" } });
  expect(screen.getAllByRole("option")).toHaveLength(21);
  fireEvent.click(screen.getByRole("option", { name: "Owner is Colleague 0" }));
  fireEvent.change(input, { target: { value: "Owner" } });
  expect(
    screen.queryByRole("option", { name: "Owner is Colleague 0" }),
  ).not.toBeInTheDocument();
  expect(
    screen.getByRole("option", { name: "Owner is Colleague 20" }),
  ).toBeInTheDocument();
});

it.each([
  ["Công ty", "cong ty Acme"],
  ["Công ty", "Co\u0302ng ty Acme"],
  ["Office", "Oﬃce Acme"],
  ["Responsible team", "responsible   team Acme"],
])("searches after the folded %s label in %s", async (label, query) => {
  const search = vi.fn(async () => [{ value: "acme", label: "Acme" }]);
  render(
    <Surface
      onFilter={vi.fn()}
      chips={[
        {
          key: "company_id",
          label,
          allLabel: "Any company",
          options: [],
          search,
          appliedName: undefined,
        },
      ]}
    />,
  );
  const input = screen.getByRole("combobox", { name: "Search" });
  fireEvent.focus(input);
  fireEvent.change(input, { target: { value: query } });
  await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS));
  expect(search).toHaveBeenCalledExactlyOnceWith("Acme");
  expect(
    screen.getByRole("option", { name: `${label} is Acme` }),
  ).toBeInTheDocument();
});
