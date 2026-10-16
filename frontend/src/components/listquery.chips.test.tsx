/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ListQuery } from "./listquery";
import {
  emptyPage,
  ListTableHarness,
  type Row,
  render,
} from "./listquery.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a data-driven chip narrows the list", () => {
  it("sends the parameter its option names, not the chip's own key", async () => {
    const user = userEvent.setup();
    const fetchPage = vi.fn(
      async (_query: ListQuery, _cursor: string | null) => ({
        data: [] as Row[],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(
      <ListTableHarness
        fetchPage={fetchPage}
        // The owner dial is a dataChip: it names the viewer's teams, which are
        // server strings rather than message keys.
        dataChips={[
          {
            key: "owner",
            label: "Owner",
            allLabel: "Any owner",
            options: [
              { value: "owner_id:u-1", label: "My records" },
              { value: "unassigned:true", label: "Unassigned" },
            ],
          },
        ]}
      />,
    );
    await waitFor(() => expect(fetchPage).toHaveBeenCalled());

    await user.click(await screen.findByRole("button", { name: "Filter" }));
    await user.click(screen.getByRole("button", { name: "Owner" }));
    await user.click(screen.getByRole("radio", { name: "Unassigned" }));

    // `unassigned=true`, not `owner=unassigned:true`. The server ignores a
    // parameter it does not know. So the wrong spelling answers the whole list
    // with 200 OK: a filter that reads as working and is not.
    await waitFor(() =>
      expect(fetchPage.mock.calls.at(-1)?.[0].filters.unassigned).toBe("true"),
    );
    expect(fetchPage.mock.calls.at(-1)?.[0].filters).not.toHaveProperty(
      "owner",
    );
  });
});

describe("two chips on one list", () => {
  it("does not let one dial clear the other's answer", async () => {
    const user = userEvent.setup();
    const fetchPage = vi.fn(
      async (_query: ListQuery, _cursor: string | null) => ({
        data: [] as Row[],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(
      <ListTableHarness
        fetchPage={fetchPage}
        chips={[
          {
            key: "lifecycle",
            label: "company.lifecycle",
            allLabel: "company.filterLifecycleAll",
            options: [
              { value: "customer", label: "company.lifecycle.customer" },
            ],
          },
        ]}
        dataChips={[
          {
            key: "owner",
            label: "Owner",
            allLabel: "Any owner",
            options: [{ value: "unassigned:true", label: "Unassigned" }],
          },
        ]}
      />,
    );
    await waitFor(() => expect(fetchPage).toHaveBeenCalled());

    await user.click(await screen.findByRole("button", { name: "Filter" }));
    await user.click(screen.getByRole("button", { name: "Owner" }));
    await user.click(screen.getByRole("radio", { name: "Unassigned" }));
    await waitFor(() =>
      expect(fetchPage.mock.calls.at(-1)?.[0].filters.unassigned).toBe("true"),
    );

    // Now narrow by something else. Clearing every composite parameter on the
    // surface (rather than only the chip being changed) would drop the owner
    // answer here. Picking a lifecycle would then silently widen the list back
    // to every owner, while the owner chip still showed "Unassigned".
    await user.click(screen.getByRole("button", { name: "Lifecycle" }));
    await user.click(screen.getByRole("radio", { name: "Customer" }));

    await waitFor(() =>
      expect(fetchPage.mock.calls.at(-1)?.[0].filters.lifecycle).toBe(
        "customer",
      ),
    );
    expect(fetchPage.mock.calls.at(-1)?.[0].filters.unassigned).toBe("true");
  });
});

describe("a chip whose options arrive late", () => {
  it("does not throw the reader back to page 1", async () => {
    const user = userEvent.setup();
    const page = (from: number) => ({
      data: Array.from({ length: 25 }, (_, i) => ({
        id: `r-${from + i}`,
        name: `Row ${from + i}`,
      })),
      page: { next_cursor: `c-${from + 25}`, has_more: true },
    });
    let served = 0;
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      page(25 * served++),
    );
    const option = (value: string, label: string) => ({ value, label });
    // The owner dial names the viewer's teams, which arrive on their own query.
    // This stands in for that: the roster answers after the list has rendered,
    // and the chip gains an option without the reader touching anything.
    function LateChips() {
      const [teamKnown, setTeamKnown] = useState(false);
      const chip = {
        key: "owner",
        label: "Owner",
        allLabel: "Any owner",
        options: [
          option("owner_id:u-1", "My records"),
          ...(teamKnown ? [option("owner_team_id:t-9", "Team Neukunden")] : []),
          option("unassigned:true", "Unassigned"),
        ],
      };
      return (
        <>
          <button type="button" onClick={() => setTeamKnown(true)}>
            roster answers
          </button>
          <ListTableHarness fetchPage={fetchPage} dataChips={[chip]} />
        </>
      );
    }
    render(<LateChips />);
    await waitFor(() => expect(screen.getByText("Row 0")).toBeTruthy());

    await user.click(screen.getByRole("button", { name: /Next/ }));
    await waitFor(() => expect(screen.getByText("Row 25")).toBeTruthy());

    // The reader did not touch a filter, so the page they are on must survive.
    // A list that jumps back to the top because a picker finished loading loses
    // the reader's place for a reason they cannot see.
    await user.click(screen.getByRole("button", { name: "roster answers" }));
    await waitFor(() => expect(screen.getByText("Row 25")).toBeTruthy());
    expect(screen.queryByText("Row 0")).toBeNull();
  });
});

describe("a scope the chips cannot see", () => {
  it("puts the reader back on page 1 when it changes", async () => {
    const user = userEvent.setup();
    const page = (from: number) => ({
      data: Array.from({ length: 25 }, (_, i) => ({
        id: `r-${from + i}`,
        name: `Row ${from + i}`,
      })),
      page: { next_cursor: `c-${from + 25}`, has_more: true },
    });
    let served = 0;
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      page(25 * served++),
    );
    // Deals stands behind this: its pipeline picker is screen state. So
    // switching pipelines leaves `filters` and every chip as they were,
    // while the rows change completely. Page 2 of one pipeline is not
    // page 2 of another, so the reader must not be left standing on it.
    function ScopedList() {
      const [pipeline, setPipeline] = useState("p-1");
      return (
        <>
          <button type="button" onClick={() => setPipeline("p-2")}>
            other pipeline
          </button>
          <ListTableHarness fetchPage={fetchPage} scopeKey={pipeline} />
        </>
      );
    }
    render(<ScopedList />);
    await waitFor(() => expect(screen.getByText("Row 0")).toBeTruthy());
    await user.click(screen.getByRole("button", { name: /Next/ }));
    await waitFor(() => expect(screen.getByText("Row 25")).toBeTruthy());

    await user.click(screen.getByRole("button", { name: "other pipeline" }));
    await waitFor(() => expect(screen.getByText("Row 0")).toBeTruthy());
  });
});

describe("a late option that matches a filter already set", () => {
  it("still keeps the reader on their page", async () => {
    const user = userEvent.setup();
    const page = (from: number) => ({
      data: Array.from({ length: 25 }, (_, i) => ({
        id: `r-${from + i}`,
        name: `Row ${from + i}`,
      })),
      page: { next_cursor: `c-${from + 25}`, has_more: true },
    });
    let served = 0;
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      page(25 * served++),
    );
    // The harder half of the same bug. A saved view restores
    // `owner_team_id=t-9` before the roster has answered, so the filter is
    // already set when the matching option finally arrives. chosenFor then
    // gains the chip's own key, so the chosen result changes shape. The query,
    // and therefore every row, stays the same as it was.
    function LateMatchingOption() {
      const [teamKnown, setTeamKnown] = useState(false);
      const chip = {
        key: "owner",
        label: "Owner",
        allLabel: "Any owner",
        options: teamKnown
          ? [{ value: "owner_team_id:t-9", label: "Team Neukunden" }]
          : [],
      };
      return (
        <>
          <button type="button" onClick={() => setTeamKnown(true)}>
            roster answers
          </button>
          <ListTableHarness
            fetchPage={fetchPage}
            dataChips={[chip]}
            initialFilters={{ owner_team_id: "t-9" }}
          />
        </>
      );
    }
    render(<LateMatchingOption />);
    await waitFor(() => expect(screen.getByText("Row 0")).toBeTruthy());
    await user.click(screen.getByRole("button", { name: /Next/ }));
    await waitFor(() => expect(screen.getByText("Row 25")).toBeTruthy());

    await user.click(screen.getByRole("button", { name: "roster answers" }));
    await waitFor(() => expect(screen.getByText("Row 25")).toBeTruthy());
    expect(screen.queryByText("Row 0")).toBeNull();
  });
});

describe("a late option that matches a filter already set", () => {
  it("shows the chip as chosen once the option arrives", async () => {
    const user = userEvent.setup();
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    // The display half of the same moment. Keeping the reader in place must not
    // cost them the truth about what the list is narrowed by. Once the roster
    // answers, the dial has to read "Team Neukunden", not "Any owner".
    function LateMatchingOption() {
      const [teamKnown, setTeamKnown] = useState(false);
      const chip = {
        key: "owner",
        label: "Owner",
        allLabel: "Any owner",
        options: teamKnown
          ? [{ value: "owner_team_id:t-9", label: "Team Neukunden" }]
          : [],
      };
      return (
        <>
          <button type="button" onClick={() => setTeamKnown(true)}>
            roster answers
          </button>
          <ListTableHarness
            fetchPage={fetchPage}
            dataChips={[chip]}
            initialFilters={{ owner_team_id: "t-9" }}
          />
        </>
      );
    }
    render(<LateMatchingOption />);
    await user.click(screen.getByRole("button", { name: "roster answers" }));
    await waitFor(() =>
      expect(
        screen.getByRole("group", { name: "Owner: Team Neukunden" }),
      ).toBeTruthy(),
    );
  });
});
