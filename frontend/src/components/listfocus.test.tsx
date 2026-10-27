/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { translate } from "../i18n";
import { emptyPage, ListTableHarness, render } from "./listquery.testkit";
import { mineEmptyNote } from "./recordlist";
import { RefusedFiltersNote } from "./refusedfiltersnote";

// Each press below empties the list's note and takes its own button away. The
// reader's focus goes to the list's title rather than falling to the page.

beforeEach(() => {
  window.location.hash = "#/contacts?status=new&owner_id=u-1";
});

afterEach(() => {
  cleanup();
  window.location.hash = "";
});

const fetchPage = async () => emptyPage();

async function pressAndExpectTitleFocused(name: string) {
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name }));
  expect(screen.getByRole("heading", { name: "Contacts" })).toHaveFocus();
}

describe("a press that takes its own button away", () => {
  it("hands focus to the list's title from Clear filters", async () => {
    render(<ListTableHarness title="Contacts" fetchPage={fetchPage} />);
    await pressAndExpectTitleFocused("Clear filters");
  });

  it("hands focus to the list's named rows when it has no title", async () => {
    const user = userEvent.setup();
    render(<ListTableHarness fetchPage={fetchPage} />);
    await user.click(
      await screen.findByRole("button", { name: "Clear filters" }),
    );
    expect(
      screen.getByRole("region", { name: translate("en", "nav.contacts") }),
    ).toHaveFocus();
  });

  it("hands focus to the list's title from Show all", async () => {
    window.location.hash = "#/contacts?owner_id=u-1";
    render(
      <ListTableHarness
        title="Contacts"
        fetchPage={fetchPage}
        emptyNote={(state) =>
          mineEmptyNote({
            t: (key, params) => translate("en", key, params),
            state,
            viewerId: "u-1",
            unit: "unit.leads",
          })
        }
      />,
    );
    await pressAndExpectTitleFocused("Show all");
  });

  it("hands focus to the list's title from a refused filter's Remove", async () => {
    render(
      <ListTableHarness
        title="Contacts"
        fetchPage={fetchPage}
        emptyNote={() => (
          <RefusedFiltersNote
            message="Your role cannot narrow this list by Status."
            dials={["status"]}
            names="Status"
          />
        )}
      />,
    );
    await pressAndExpectTitleFocused("Remove Status");
  });
});
