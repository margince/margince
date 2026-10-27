/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Panel } from "../design-system/panel";
import { translate } from "../i18n";
import { emptyPage, ListTableHarness, render } from "./listquery.testkit";
import { mineEmptyNote } from "./recordlist";
import { RefusedFiltersNote } from "./refusedfiltersnote";

// Each press below takes its own button away. Focus goes to the list's title,
// its panel's title or its search box, never to the page.

beforeEach(() => {
  window.location.hash = "#/contacts?status=new&owner_id=u-1";
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  window.location.hash = "";
});

const fetchPage = async () => emptyPage();

async function pressAndExpectFocused(name: string, heading = "Contacts") {
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name }));
  expect(screen.getByRole("heading", { name: heading })).toHaveFocus();
}

describe("a press that takes its own button away", () => {
  it("hands focus to the list's title from Clear filters", async () => {
    render(<ListTableHarness title="Contacts" fetchPage={fetchPage} />);
    await pressAndExpectFocused("Clear filters");
  });

  it("hands focus to the panel's title when the list has none", async () => {
    render(
      <Panel title="Products">
        <ListTableHarness fetchPage={fetchPage} />
      </Panel>,
    );
    await pressAndExpectFocused("Clear filters", "Products");
  });

  it("hands focus to the panel's title when clearing leaves an empty catalogue", async () => {
    render(
      <Panel title="Products">
        <ListTableHarness fetchPage={fetchPage} hideShortChrome />
      </Panel>,
    );
    await pressAndExpectFocused("Clear filters", "Products");
  });

  it("hands focus to the list's own title before its panel's", async () => {
    render(
      <Panel title="Products">
        <ListTableHarness title="Contacts" fetchPage={fetchPage} />
      </Panel>,
    );
    await pressAndExpectFocused("Clear filters", "Contacts");
  });

  it("hands focus to the search box of a list with no title and no panel", async () => {
    window.location.hash = "#/contacts?q=zzz";
    // The rows overflow, so their scroll box is a named region the search box
    // must still win over.
    vi.spyOn(HTMLDivElement.prototype, "scrollWidth", "get").mockReturnValue(
      900,
    );
    vi.spyOn(HTMLDivElement.prototype, "clientWidth", "get").mockReturnValue(
      300,
    );
    const user = userEvent.setup();
    render(<ListTableHarness fetchPage={fetchPage} />);
    await user.click(
      await screen.findByRole("button", { name: "Clear filters" }),
    );
    expect(screen.getByRole("region")).toBeInTheDocument();
    expect(screen.getByRole("searchbox", { name: "Search" })).toHaveFocus();
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
    await pressAndExpectFocused("Show all");
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
    await pressAndExpectFocused("Remove Status");
  });
});
