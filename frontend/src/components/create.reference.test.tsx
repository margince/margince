/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it } from "vitest";
import { LocaleProvider, useT } from "../i18n";
import { fieldControl } from "./create.controls";
import type { CreateField } from "./create.fields";

// The searched reference control names its value: by the candidate just
// picked, and otherwise by the field's own options.

afterEach(cleanup);

const field: CreateField = {
  key: "company_id",
  labelText: "Company",
  searchTargets: async () => [{ id: "o-1", name: "Name from the search" }],
  options: [{ value: "o-1", label: "Name from the options" }],
};

function Harness() {
  const t = useT();
  const [value, setValue] = useState("");
  return (
    <>
      {fieldControl(field, { id: "company" }, value, setValue, t)}
      <button type="button" onClick={() => setValue("o-1")}>
        Restore
      </button>
    </>
  );
}

it("names a pick by its candidate, and a restored value by the options", async () => {
  const user = userEvent.setup();
  render(
    <LocaleProvider initial="en">
      <Harness />
    </LocaleProvider>,
  );

  await user.type(screen.getByRole("searchbox"), "Name");
  await user.click(
    await screen.findByRole("button", { name: "Name from the search" }),
  );
  expect(screen.getByText("Name from the search")).toBeTruthy();

  await user.click(screen.getByRole("button", { name: "Not set" }));
  await user.click(screen.getByRole("button", { name: "Restore" }));
  expect(screen.getByText("Name from the options")).toBeTruthy();
  expect(screen.queryByText("Name from the search")).toBeNull();
});
