/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { ToastProvider } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";
import { jsonResponse } from "../../testing/company.fixtures";
import { ContactRecordActions } from "../contacts/contactrecordactions";

// The merge picker names more than a contact's name, because two contacts may
// share one and a wrong pick folds two different contacts together.

type Contact = components["schemas"]["Contact"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const source = {
  id: "p-source",
  full_name: "Anna Berg",
  version: 3,
} as Contact;

const searchRows = [
  source,
  {
    id: "p-a",
    full_name: "Anna Berg",
    employer: { company_id: "o-a", company_name: "Company A" },
    title: "CFO",
    primary_email: "anna.berg@example.com",
    writable: true,
  },
  {
    id: "p-b",
    full_name: "Anna Berg",
    employer: { company_id: "o-b", company_name: "Company B" },
    title: null,
    primary_email: "a.berg@example.com",
    writable: true,
  },
  // A reader who may not see the employment edge gets no employer. A contact
  // with no address gets a null one. Neither is drawn.
  {
    id: "p-c",
    full_name: "Anna Bergmann",
    title: null,
    primary_email: null,
    writable: true,
  },
];

// A merge that never answers holds the write in flight for as long as a test looks.
async function openPickerAndSearch(
  term: string,
  merge: Response | Promise<Response> = jsonResponse({}),
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) =>
      new URL(request.url).pathname.endsWith("/contacts")
        ? jsonResponse({
            data: searchRows,
            page: { next_cursor: null, has_more: false },
          })
        : merge,
    ),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <ContactRecordActions contact={source} />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
  const user = userEvent.setup();
  await user.click(screen.getByTestId("merge-record"));
  const dialog = screen.getByRole("dialog");
  await user.type(within(dialog).getByRole("searchbox"), term);
  return { dialog, user };
}

describe("the contact merge picker", () => {
  it("tells two contacts of one name apart by company, title and email", async () => {
    const { dialog } = await openPickerAndSearch("anna");

    expect(
      await within(dialog).findByText(
        "Company A · CFO · anna.berg@example.com",
      ),
    ).toBeTruthy();
    expect(
      within(dialog).getByText("Company B · a.berg@example.com"),
    ).toBeTruthy();
  });

  it("draws a contact with nothing to add by its name alone", async () => {
    const { dialog } = await openPickerAndSearch("anna");

    const option = (await within(dialog).findByText("Anna Bergmann")).closest(
      "button",
    );
    expect(option?.textContent).toBe("Anna Bergmann");
  });

  it("never offers the contact being merged away", async () => {
    const { dialog } = await openPickerAndSearch("anna");

    await within(dialog).findByText("Anna Bergmann");
    // The source reads "Anna Berg" with no detail line; every option left
    // carries one, or is the other name.
    const options = within(within(dialog).getByRole("list"))
      .getAllByRole("button")
      .map((option) => option.textContent);
    expect(options).toEqual([
      "Anna BergCompany A · CFO · anna.berg@example.com",
      "Anna BergCompany B · a.berg@example.com",
      "Anna Bergmann",
    ]);
  });

  it("keeps the detail of a picked look-alike on the selected line and in the confirmation", async () => {
    const { dialog, user } = await openPickerAndSearch("anna");

    await user.click(
      await within(dialog).findByRole("button", { name: /Company B/ }),
    );

    expect(dialog.querySelector(".recordpicker-selected")?.textContent).toBe(
      "Anna BergCompany B · a.berg@example.com",
    );
    expect(
      within(dialog).getByText(
        "Merge Anna Berg into Anna Berg (Company B · a.berg@example.com)? This archives Anna Berg.",
      ),
    ).toBeTruthy();
  });

  it("holds the pick while the merge runs", async () => {
    const { dialog, user } = await openPickerAndSearch(
      "anna",
      new Promise<Response>(() => {}),
    );
    await user.click(
      await within(dialog).findByRole("button", { name: /Company A/ }),
    );
    await user.type(within(dialog).getByRole("searchbox"), "anna");
    await within(dialog).findByText("Company B · a.berg@example.com");

    await user.click(within(dialog).getByTestId("merge-confirm"));

    expect(within(dialog).getByRole("searchbox")).toHaveProperty(
      "disabled",
      true,
    );
    for (const option of within(within(dialog).getByRole("list")).getAllByRole(
      "button",
    )) {
      expect(option).toHaveProperty("disabled", true);
    }
  });
});
