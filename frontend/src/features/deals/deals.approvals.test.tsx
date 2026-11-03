/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { translate } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { DealScreen } from "./deals";
import { deal, jsonResponse, REP_GRANTS, render } from "./deals.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
  localStorage.clear();
});

const approval = (id: string, targetId: string) => ({
  id,
  kind: "advance_deal",
  status: "pending",
  proposed_by: "agent:capture",
  target_entity_type: "deal",
  target_entity_id: targetId,
  created_at: "2026-10-01T09:00:00Z",
});

// Answers the target-scoped read the way the server does: only the rows staged
// against the record the request names.
function stubBackend(
  approvals: ReturnType<typeof approval>[],
  onDecide?: (url: string) => void,
) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = String(request ? request.url : input);
    const method = request ? request.method : (init?.method ?? "GET");
    if (method === "POST" && /\/approvals\/[^/]+\/(approve|reject)/.test(url)) {
      onDecide?.(url);
      return jsonResponse({});
    }
    if (url.includes("/me")) {
      return jsonResponse(meFixture({ allow: REP_GRANTS }));
    }
    if (url.includes("/approvals")) {
      const target = new URL(url).searchParams.get("target_entity_id");
      return jsonResponse({
        data: approvals.filter(
          (approval) => approval.target_entity_id === target,
        ),
        page: { next_cursor: null, has_more: false },
      });
    }
    if (url.includes("/commitments")) {
      return jsonResponse({ complete: true, has_more: false, data: [] });
    }
    if (url.includes("/deals/d1") && !url.includes("/offers")) {
      return jsonResponse(deal({}));
    }
    return jsonResponse({ data: [], page: { next_cursor: null } });
  });
}

describe("DealScreen, pending approvals", () => {
  it("lists only this deal's pending approvals and rejects one from the row", async () => {
    const decided: string[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([approval("ap1", "d1"), approval("ap2", "other")], (url) =>
        decided.push(url),
      ),
    );
    render(<DealScreen id="d1" />);
    expect(
      await screen.findByText(translate("en", "deal.pendingApprovals")),
    ).toBeTruthy();
    expect(screen.getAllByText(translate("en", "trust.accept"))).toHaveLength(
      1,
    );
    await userEvent.click(
      screen.getByRole("button", { name: translate("en", "trust.dismiss") }),
    );
    await vi.waitFor(() => expect(decided).toHaveLength(1));
    expect(decided[0]).toContain("/approvals/ap1/reject");
  });

  it("draws no panel while the deal has nothing pending", async () => {
    vi.stubGlobal("fetch", stubBackend([approval("ap2", "other")]));
    render(<DealScreen id="d1" />);
    expect(await screen.findByText(deal({}).name)).toBeTruthy();
    expect(
      screen.queryByText(translate("en", "deal.pendingApprovals")),
    ).toBeNull();
  });
});
