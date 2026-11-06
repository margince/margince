/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import { useRowSelection } from "./listselection";
import { ListTable } from "./listtable";
import { columns, type Row, render, testRows } from "./listtable.testkit";

afterEach(cleanup);

const selectPage = en["bulk.selectPage"];
const selectAll = en["bulk.selectAll"];
const clearSelection = en["bulk.clearSelection"];

// A list whose server holds `total` rows and answers `perRead` at a time.
// It hands the table its rows and next-page call as a cursor-paged screen does.
function PagedList({
  total,
  perRead,
  limit,
  selectable,
}: Readonly<{
  total: number;
  perRead: number;
  limit?: number;
  selectable?: (row: Row) => boolean;
}>) {
  const all = testRows(total);
  const [held, setHeld] = useState(perRead);
  const rows = all.slice(0, held);
  const chosen = useRowSelection({ rows, keyOf: (row) => row.id, limit });
  return (
    <ListTable<Row>
      rows={rows}
      columns={columns}
      rowKey={(row) => row.id}
      unit="rows"
      hasMore={held < total}
      onLoadMore={() => setHeld((now) => Math.min(total, now + perRead))}
      selection={{
        selected: chosen.selected,
        limit: chosen.limit,
        selectable,
        onChange: chosen.onChange,
        label: (row) => `Select ${row.name}`,
        bar: <span>{chosen.selected.size} chosen</span>,
      }}
    />
  );
}

describe("selecting every row of a list", () => {
  it("ticks and unticks the rendered page from the header box", async () => {
    const user = userEvent.setup();
    render(<PagedList total={60} perRead={60} />);
    const box = screen.getByRole("checkbox", { name: selectPage });
    expect(box).not.toBeChecked();

    await user.click(box);
    expect(screen.getByText("25 chosen")).toBeInTheDocument();
    expect(box).toBeChecked();

    await user.click(box);
    expect(screen.queryByText(/chosen/)).not.toBeInTheDocument();
  });

  it("shows the header box as mixed while only some of the page is ticked", async () => {
    const user = userEvent.setup();
    render(<PagedList total={10} perRead={10} />);
    await user.click(screen.getByRole("checkbox", { name: "Select Row 03" }));
    const box = screen.getByRole("checkbox", { name: selectPage });
    expect(box).not.toBeChecked();
    expect(box).toHaveProperty("indeterminate", true);
  });

  it("reads the next pages until it holds every matching row, then ticks them all", async () => {
    const user = userEvent.setup();
    render(<PagedList total={130} perRead={50} />);
    await user.click(screen.getByRole("checkbox", { name: "Select Row 01" }));
    await user.click(screen.getByRole("button", { name: selectAll }));
    expect(await screen.findByText("130 chosen")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: selectAll }),
    ).not.toBeInTheDocument();
  });

  it("stops at the limit and says why the rest are not selected", async () => {
    const user = userEvent.setup();
    render(<PagedList total={400} perRead={100} limit={150} />);
    await user.click(screen.getByRole("checkbox", { name: "Select Row 01" }));
    await user.click(screen.getByRole("button", { name: selectAll }));
    expect(await screen.findByText("150 chosen")).toBeInTheDocument();
    expect(
      screen.getByText(
        en["bulk.selectionLimit_other"].replaceAll("{count}", "150"),
      ),
    ).toBeInTheDocument();
  });

  it("leaves the rows the verbs cannot act on out of Select all", async () => {
    const user = userEvent.setup();
    render(
      <PagedList
        total={30}
        perRead={30}
        selectable={(row) => row.value % 2 === 0}
      />,
    );
    await user.click(screen.getByRole("checkbox", { name: "Select Row 02" }));
    await user.click(screen.getByRole("button", { name: selectAll }));
    expect(await screen.findByText("15 chosen")).toBeInTheDocument();
  });

  it("clears every selected row, on this page and the others", async () => {
    const user = userEvent.setup();
    render(<PagedList total={60} perRead={60} />);
    await user.click(screen.getByRole("checkbox", { name: "Select Row 01" }));
    await user.click(screen.getByRole("button", { name: selectAll }));
    await screen.findByText("60 chosen");
    await user.click(screen.getByRole("button", { name: clearSelection }));
    await waitFor(() =>
      expect(screen.queryByText(/chosen/)).not.toBeInTheDocument(),
    );
  });

  it("adds no row past the limit by hand, and still lets one be unticked", async () => {
    const user = userEvent.setup();
    render(<PagedList total={5} perRead={5} limit={2} />);
    await user.click(screen.getByRole("checkbox", { name: "Select Row 01" }));
    await user.click(screen.getByRole("checkbox", { name: "Select Row 02" }));
    await user.click(screen.getByRole("checkbox", { name: "Select Row 03" }));
    expect(screen.getByText("2 chosen")).toBeInTheDocument();
    expect(
      screen.getByRole("checkbox", { name: "Select Row 03" }),
    ).not.toBeChecked();

    await user.click(screen.getByRole("checkbox", { name: "Select Row 01" }));
    await user.click(screen.getByRole("checkbox", { name: "Select Row 03" }));
    expect(screen.getByText("2 chosen")).toBeInTheDocument();
    expect(
      screen.getByRole("checkbox", { name: "Select Row 03" }),
    ).toBeChecked();
  });
});
