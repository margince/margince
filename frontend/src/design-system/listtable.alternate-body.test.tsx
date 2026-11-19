/** @vitest-environment happy-dom */
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import { ListTable } from "./listtable";

afterEach(cleanup);

describe("ListTable alternate body paging ownership", () => {
  it("keeps shared controls but suppresses the table count and pager", () => {
    render(
      <LocaleProvider initial="en">
        <ListTable
          rows={[{ id: "lead-1" }]}
          columns={[
            {
              key: "id",
              header: "Name",
              cell: (row) => row.id,
              fixed: true,
            },
          ]}
          rowKey={(row) => row.id}
          unit="rows"
          search={{ value: "", answered: "", onChange: () => undefined }}
          body={<div>Complete board</div>}
          bodyOwnsPaging
          hasMore
        />
      </LocaleProvider>,
    );

    expect(screen.getByRole("searchbox")).toBeTruthy();
    expect(screen.getByText("Complete board")).toBeTruthy();
    expect(screen.queryByText(/of 1 rows/)).toBeNull();
    expect(screen.queryByRole("navigation", { name: "Pages" })).toBeNull();

    cleanup();
    render(
      <LocaleProvider initial="en">
        <ListTable
          rows={[{ id: "lead-1" }]}
          columns={[
            {
              key: "id",
              header: "Name",
              cell: (row) => row.id,
              fixed: true,
            },
          ]}
          rowKey={(row) => row.id}
          unit="rows"
          search={{ value: "", answered: "", onChange: () => undefined }}
          hasMore
        />
      </LocaleProvider>,
    );

    expect(screen.getByText("1 to 1 of 1 rows loaded")).toBeTruthy();
    expect(screen.getByRole("navigation", { name: "Pages" })).toBeTruthy();
  });

  it("heads an empty body with the grid's no-match line and its Clear", () => {
    const cleared: string[] = [];
    render(
      <LocaleProvider initial="en">
        <ListTable
          rows={[]}
          columns={[{ key: "id", header: "Name", cell: () => "", fixed: true }]}
          rowKey={() => ""}
          unit="rows"
          search={{
            value: "zzz",
            answered: "zzz",
            onChange: (next) => cleared.push(next),
          }}
          body={<div>Empty board</div>}
          bodyOwnsPaging
          bodyEmpty
        />
      </LocaleProvider>,
    );

    expect(screen.getByText("Empty board")).toBeTruthy();
    expect(screen.getByText(/No rows match these filters\./)).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "Clear filters" }));
    expect(cleared).toEqual([""]);
  });
});
