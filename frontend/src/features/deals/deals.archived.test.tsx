/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
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

const OFFERS = "/deals/x/offers";

// The reads the server refuses for a deal that is archived, by path.
const REFUSED_FOR_ARCHIVED = [
  "/deals/x/coverage",
  "/deals/x/status",
  "/records/deal/x/lists",
  "/records/deal/x/tags",
  "/deals/x/legal-reviews",
];

function refusedPath(url: string): string | undefined {
  return REFUSED_FOR_ARCHIVED.find((path) => url.includes(path));
}

// The deal's own backend, with the refused reads answering as the server does
// for an archived deal. `asked` is every URL the page requested.
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
      if (url.includes("/legal-reviews/settings")) {
        return jsonResponse({ enabled: true });
      }
      if (url.includes(OFFERS)) {
        return jsonResponse({ data: [], page: { next_cursor: null } });
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

describe("DealScreen — an archived deal does not ask for what the server refuses", () => {
  beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));

  it("asks for none of the refused reads and reports no section as failed", async () => {
    const { fetchStub, asked } = dealBackend("2026-07-13T00:00:00Z");
    vi.stubGlobal("fetch", fetchStub);
    render(<DealScreen id="x" />);

    expect(await screen.findByText(en["deal.archivedReadOnly"])).toBeTruthy();
    // The offers are the read an archived deal keeps, and the panel they feed
    // draws once they land. Every query the page mounts has been asked by then.
    await waitFor(() =>
      expect(asked.some((u) => u.includes(OFFERS))).toBe(true),
    );

    expect(asked.filter((url) => refusedPath(url))).toEqual([]);
    expect(screen.queryByText(en["state.failed"])).toBeNull();
    expect(screen.queryByText(en["common.error"])).toBeNull();
  });

  it("keeps the offers panel, read-only, and leaves out the committee", async () => {
    const { fetchStub } = dealBackend("2026-07-13T00:00:00Z");
    vi.stubGlobal("fetch", fetchStub);
    render(<DealScreen id="x" />);

    expect(await screen.findByText(en["deal.offersEmpty"])).toBeTruthy();
    const newOffer = screen.getByRole("button", { name: en["deal.newOffer"] });
    expect(newOffer.hasAttribute("disabled")).toBe(true);
    expect(
      screen.queryByRole("button", { name: en["rel.addStakeholder"] }),
    ).toBeNull();
    expect(document.querySelector(".dc-sides")).toBeNull();
  });

  it("offers no Legal review tab", async () => {
    const { fetchStub, asked } = dealBackend("2026-07-13T00:00:00Z");
    vi.stubGlobal("fetch", fetchStub);
    render(<DealScreen id="x" />);

    expect(await screen.findByText(en["deal.archivedReadOnly"])).toBeTruthy();
    await waitFor(() =>
      expect(asked.some((url) => url.includes("/legal-reviews/settings"))).toBe(
        true,
      ),
    );
    expect(
      screen.queryByRole("button", { name: en["legalReview.tab"] }),
    ).toBeNull();
    expect(asked.filter((url) => url.includes("/legal-reviews"))).toEqual([
      expect.stringContaining("/legal-reviews/settings"),
    ]);
  });

  it("still reads the refused reads for a live deal, and offers the Legal review tab", async () => {
    const { fetchStub, asked } = dealBackend(null);
    vi.stubGlobal("fetch", fetchStub);
    render(<DealScreen id="x" />);

    expect(
      await screen.findByRole("button", { name: en["legalReview.tab"] }),
    ).toBeTruthy();
    await waitFor(() => {
      const paths = new Set(asked.map(refusedPath).filter(Boolean));
      for (const path of REFUSED_FOR_ARCHIVED.filter(
        (each) => each !== "/deals/x/legal-reviews",
      )) {
        expect(paths.has(path)).toBe(true);
      }
    });
  });
});
