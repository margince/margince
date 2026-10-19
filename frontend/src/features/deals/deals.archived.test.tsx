/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../../testing/mefixture";
import { DealScreen } from "./deals";
import {
  deal,
  jsonResponse,
  REP_GRANTS,
  render,
  stubBackend,
} from "./deals.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
  localStorage.clear();
});

// The reads the server refuses for a deal that is archived, by path.
const REFUSED_FOR_ARCHIVED = [
  "/deals/x/coverage",
  "/deals/x/status",
  "/deals/x/offers",
  "/records/deal/x/lists",
];

function refusedPath(url: string): string | undefined {
  return REFUSED_FOR_ARCHIVED.find((path) => url.includes(path));
}

// The deal's own backend, with the four refused reads answering as the server
// does for an archived deal. `asked` is every URL the page requested.
function dealBackend(archivedAt: string | null) {
  const d = deal({ id: "x", archived_at: archivedAt });
  const base = stubBackend([d], { single: d });
  const asked: string[] = [];
  const fetchStub = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input instanceof Request ? input.url : input);
      asked.push(url);
      // The lists card draws only where the installation offers lists.
      if (url.endsWith("/me")) {
        return jsonResponse(
          meFixture({
            allow: REP_GRANTS,
            settingsAvailability: { lists: true },
          }),
        );
      }
      if (archivedAt && refusedPath(url)) {
        return jsonResponse(
          { status: 404, code: "not_found", title: "Not found" },
          404,
        );
      }
      return base(input, init);
    },
  );
  return { fetchStub, asked };
}

// The details column mounts its cards only while it is open, and the lists
// card is one of them.
async function openDetails() {
  const toggle = await screen.findByRole("button", { name: /details$/ });
  if (toggle.getAttribute("aria-pressed") !== "true") {
    await userEvent.click(toggle);
  }
}

describe("DealScreen — an archived deal does not ask for what the server refuses", () => {
  beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));

  it("asks for none of the four refused reads and reports no section as failed", async () => {
    const { fetchStub, asked } = dealBackend("2026-07-13T00:00:00Z");
    vi.stubGlobal("fetch", fetchStub);
    render(<DealScreen id="x" />);
    await openDetails();

    expect(
      await screen.findByText("This deal is archived and takes no changes."),
    ).toBeTruthy();
    // Let every query the page mounts settle before judging what it asked.
    await new Promise((resolve) => setTimeout(resolve, 50));

    expect(asked.filter((url) => refusedPath(url))).toEqual([]);
    expect(screen.queryByText("This section did not load.")).toBeNull();
    expect(
      screen.queryByText("Could not load this view. Reload the page."),
    ).toBeNull();
  });

  it("still reads all four for a live deal", async () => {
    const { fetchStub, asked } = dealBackend(null);
    vi.stubGlobal("fetch", fetchStub);
    render(<DealScreen id="x" />);
    await openDetails();

    await waitFor(() => {
      const paths = asked.map(refusedPath).filter(Boolean);
      expect(new Set(paths).size).toBe(REFUSED_FOR_ARCHIVED.length);
    });
  });
});
