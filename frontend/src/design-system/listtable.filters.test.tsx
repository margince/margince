/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  act,
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { openView, openViewStep } from "../testing/listview.testkit";
import { type ListChip, ListTable } from "./listtable";
import { columns, render } from "./listtable.testkit";

afterEach(cleanup);

describe("the Filters step", () => {
  const chips: readonly ListChip[] = [
    {
      key: "status",
      label: "Status",
      allLabel: "All statuses",
      options: [
        { value: "new", label: "New" },
        { value: "won", label: "Won" },
      ],
    },
    {
      key: "priority",
      label: "Priority",
      allLabel: "All priorities",
      options: [{ value: "high", label: "High" }],
    },
  ];

  it("narrows the attribute list as you type", async () => {
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        chips={chips}
      />,
    );
    await openViewStep(userEvent, "Filters");
    expect(screen.getByRole("button", { name: "Status" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Priority" })).toBeTruthy();

    await userEvent.type(screen.getByLabelText("Search attributes"), "sta");
    expect(screen.getByRole("button", { name: "Status" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Priority" })).toBeNull();
  });

  // Picking an attribute detaches the pressed button before the document's
  // click listener runs, and a detached node has no ancestors to be inside.
  it("stays open when a click's own target has left the document", async () => {
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        chips={chips}
      />,
    );
    await openViewStep(userEvent, "Filters");

    // Reproduce the real sequence: a node inside the document is clicked, and
    // the click's own handling removes it before the document-level listener
    // runs. A capture-phase listener stands in for the re-render.
    const probe = document.createElement("button");
    document.body.append(probe);
    const detach = () => probe.remove();
    document.addEventListener("click", detach, true);
    await act(async () => {
      probe.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    document.removeEventListener("click", detach, true);

    expect(screen.getByRole("button", { name: "Status" })).toBeTruthy();
  });

  it("picking an attribute then a value applies the filter", async () => {
    const onChipChange = vi.fn();
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        chips={chips}
        onChipChange={onChipChange}
      />,
    );
    await openViewStep(userEvent, "Filters");
    await userEvent.click(screen.getByRole("button", { name: "Status" }));
    await userEvent.click(screen.getByRole("radio", { name: "New" }));
    expect(onChipChange).toHaveBeenCalledWith("status", "new");
  });

  it("an applied filter reads as attribute/condition/value row, and Delete filter clears it", async () => {
    const onChipChange = vi.fn();
    const { container } = render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        chips={chips}
        chosen={{ status: "new" }}
        onChipChange={onChipChange}
      />,
    );
    const row = container.querySelector(".lt-frow");
    expect(row?.getAttribute("aria-label")).toBe("Status: New");
    // The condition trigger and the value trigger share the row. The row's own
    // (closed) menus also carry an "is" and a "New". So scope to the triggers
    // themselves rather than a plain name match.
    expect(row?.querySelector(".lt-frow-seg:not(.lt-frow-value)")).toBeTruthy();
    expect(row?.querySelector(".lt-frow-value")?.textContent).toBe("New");

    await userEvent.click(
      screen.getByRole("button", {
        name: "More actions for the Status filter",
      }),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "Delete filter" }),
    );
    expect(onChipChange).toHaveBeenCalledWith("status", "");
  });

  it("the condition menu offers exactly one condition", async () => {
    const { container } = render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        chips={chips}
        chosen={{ status: "new" }}
      />,
    );
    // The condition trigger reads "is", the same text its own single menu
    // entry carries. Scope to the trigger segment: a plain name match would
    // also catch that (closed) entry.
    const trigger = container.querySelector<HTMLElement>(
      ".lt-frow-seg:not(.lt-frow-value)",
    );
    if (!trigger) {
      throw new Error("the condition trigger did not render");
    }
    await userEvent.click(trigger);
    const condition = within(
      screen.getByRole("group", { name: "Condition" }),
    ).getByRole("radio", { name: "is" });
    expect(condition).toBeChecked();
  });

  it("offers every attribute, an applied one with its value", async () => {
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        chips={chips}
        chosen={{ status: "new" }}
      />,
    );
    const panel = await openView(userEvent);
    expect(
      within(panel).getByRole("button", {
        name: "Filters, Status, changed",
      }),
    ).toBeInTheDocument();
    const step = within(await openViewStep(userEvent, "Filters"));
    expect(
      step.getByRole("button", { name: /^Status, .+, changed$/ }),
    ).toBeTruthy();
    expect(step.getByRole("button", { name: "Priority" })).toBeInTheDocument();
  });
});

describe("a chip with an async search source", () => {
  type SearchFn = Extract<ListChip, { search: unknown }>["search"];
  function chipWithSearch(search: SearchFn): readonly ListChip[] {
    return [
      {
        key: "company",
        label: "Company",
        allLabel: "All companies",
        options: [],
        search,
        appliedName: undefined,
      },
    ];
  }

  async function openCompanyValueStep() {
    await openViewStep(userEvent, "Filters");
    await userEvent.click(screen.getByRole("button", { name: "Company" }));
  }

  it("queries on debounce and renders the results", async () => {
    const search = vi.fn().mockResolvedValue([{ value: "o1", label: "Acme" }]);
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        chips={chipWithSearch(search)}
      />,
    );
    await openCompanyValueStep();
    expect(screen.getByText("Type to search")).toBeTruthy();

    vi.useFakeTimers();
    try {
      fireEvent.change(screen.getByLabelText("Search Company values"), {
        target: { value: "ac" },
      });
      act(() => {
        vi.advanceTimersByTime(250);
      });
    } finally {
      vi.useRealTimers();
    }

    await waitFor(() => expect(search).toHaveBeenCalledWith("ac"));
    expect(await screen.findByRole("radio", { name: "Acme" })).toBeTruthy();
  });

  it("keeps the previous results on screen while the next query is in flight", async () => {
    let resolveSecond:
      | ((value: readonly { value: string; label: string }[]) => void)
      | undefined;
    const search = vi
      .fn()
      .mockResolvedValueOnce([{ value: "o1", label: "Acme" }])
      .mockImplementationOnce(
        () =>
          new Promise<readonly { value: string; label: string }[]>(
            (resolve) => {
              resolveSecond = resolve;
            },
          ),
      );
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        chips={chipWithSearch(search)}
      />,
    );
    await openCompanyValueStep();
    const input = screen.getByLabelText("Search Company values");

    vi.useFakeTimers();
    try {
      fireEvent.change(input, { target: { value: "ac" } });
      act(() => {
        vi.advanceTimersByTime(250);
      });
    } finally {
      vi.useRealTimers();
    }
    expect(await screen.findByRole("radio", { name: "Acme" })).toBeTruthy();

    vi.useFakeTimers();
    try {
      fireEvent.change(input, { target: { value: "acm" } });
      act(() => {
        vi.advanceTimersByTime(250);
      });
    } finally {
      vi.useRealTimers();
    }
    await waitFor(() => expect(search).toHaveBeenCalledTimes(2));
    // The prior result stays up while the next query is still in flight.
    expect(screen.getByRole("radio", { name: "Acme" })).toBeTruthy();
    expect(screen.getByText("Searching…")).toBeTruthy();

    resolveSecond?.([{ value: "o2", label: "Acme Renewals" }]);
    expect(
      await screen.findByRole("radio", { name: "Acme Renewals" }),
    ).toBeTruthy();
  });

  it("shows a failure when the search rejects", async () => {
    const search = vi.fn().mockRejectedValue(new Error("search down"));
    render(
      <ListTable
        rows={[]}
        columns={columns}
        rowKey={(row) => row.id}
        unit="rows"
        chips={chipWithSearch(search)}
      />,
    );
    await openCompanyValueStep();

    vi.useFakeTimers();
    try {
      fireEvent.change(screen.getByLabelText("Search Company values"), {
        target: { value: "ac" },
      });
      act(() => {
        vi.advanceTimersByTime(250);
      });
    } finally {
      vi.useRealTimers();
    }

    expect(await screen.findByText("Search failed. Retry.")).toBeTruthy();
  });
});
