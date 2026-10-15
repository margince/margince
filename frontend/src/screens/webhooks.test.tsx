/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import { WebhooksCard } from "./webhooks";

// The Settings → Integrations subscription list: renders from the typed
// listWebhookSubscriptions seam, gates the create/manage affordance on
// the webhook_subscription grants (the server stays the RBAC authority), and reads
// the deployment's 503 webhooks_not_configured as an honest "not enabled"
// state rather than an error.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const SUBSCRIPTIONS = {
  data: [
    {
      id: "sub-1",
      owner_id: "user-1",
      target_url: "https://example.test/hooks/margince",
      event_types: ["deal.stage_changed", "lead.promoted"],
      state: "active",
      version: 1,
      created_at: "2026-07-01T00:00:00Z",
      updated_at: "2026-07-01T00:00:00Z",
      archived_at: null,
    },
  ],
  page: { next_cursor: null, has_more: false },
  delivery_enabled: true,
};

// Create, edit/rotate/replay and archive are three grants on one object. The
// default fixture holds all three; the denied case holds none.
const WEBHOOK_OPERATOR: GrantSpec = {
  webhook_subscription: ["create", "update", "delete"],
};

// `deliveryEnabled` is the deployment's signing key, independent of the caller's
// grants: the two absences it crosses with are different causes, and the card
// has to pick which one it explains.
function backendFor(
  allow: GrantSpec,
  subscriptionsStatus = 200,
  deliveryEnabled = true,
) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req =
      input instanceof Request ? input : new Request(String(input), init);
    if (req.url.endsWith("/v1/me")) {
      return jsonResponse(meFixture({ allow }));
    }
    if (req.url.includes("/webhook-subscriptions") && req.method === "GET") {
      if (subscriptionsStatus === 503) {
        return jsonResponse(
          {
            title: "Service Unavailable",
            code: "webhooks_not_configured",
            detail:
              "outbound webhooks require a deployment signing key that is not configured",
          },
          503,
        );
      }
      return jsonResponse(
        { ...SUBSCRIPTIONS, delivery_enabled: deliveryEnabled },
        subscriptionsStatus,
      );
    }
    throw new Error(`unexpected request: ${req.method} ${req.url}`);
  });
}

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// Rotating the signing secret and archiving the subscription both live behind
// the row's overflow: neither is the same weight as Edit, and both are
// irreversible. Every assertion about them opens it first, the way a reader
// does.
const MORE_ACTIONS = "More actions";

async function openRowActions(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: MORE_ACTIONS }));
}

describe("WebhooksCard", () => {
  it("renders a subscription list from the typed seam", async () => {
    vi.stubGlobal("fetch", backendFor(WEBHOOK_OPERATOR));
    render(<WebhooksCard />);

    await waitFor(() =>
      expect(
        screen.getByText("https://example.test/hooks/margince"),
      ).toBeTruthy(),
    );
    expect(screen.getByText("deal.stage_changed")).toBeTruthy();
    expect(screen.getByText("lead.promoted")).toBeTruthy();
  });

  it("hides the create affordance for a non-admin/ops role", async () => {
    vi.stubGlobal("fetch", backendFor({}));
    render(<WebhooksCard />);

    await waitFor(() =>
      expect(
        screen.getByText("https://example.test/hooks/margince"),
      ).toBeTruthy(),
    );
    expect(screen.queryByTestId("new-webhook-subscription")).toBeNull();
  });

  it("shows the create affordance for an admin/ops role", async () => {
    vi.stubGlobal("fetch", backendFor(WEBHOOK_OPERATOR));
    render(<WebhooksCard />);

    await waitFor(() =>
      expect(screen.getByTestId("new-webhook-subscription")).toBeTruthy(),
    );
  });

  // One grant at a time. A fixture holding create+update+delete together cannot
  // distinguish a correct binding from a transposed one — swapping update and
  // delete in the screen passes such a suite outright.
  it("offers edit on the update grant while withholding archive and create", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor({ webhook_subscription: ["read", "update"] }),
    );
    render(<WebhooksCard />);

    await waitFor(() =>
      expect(
        screen.getByText("https://example.test/hooks/margince"),
      ).toBeTruthy(),
    );
    expect(screen.getByRole("button", { name: "Edit" })).toBeTruthy();
    await openRowActions(userEvent.setup());
    expect(screen.getByTestId("rotate-webhook-secret")).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Archive" })).toBeNull();
    expect(screen.queryByTestId("new-webhook-subscription")).toBeNull();
  });

  it("offers archive on the delete grant alone", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor({ webhook_subscription: ["read", "delete"] }),
    );
    render(<WebhooksCard />);

    await waitFor(() =>
      expect(
        screen.getByText("https://example.test/hooks/margince"),
      ).toBeTruthy(),
    );
    await openRowActions(userEvent.setup());
    expect(screen.getByRole("button", { name: "Archive" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
    expect(screen.queryByTestId("rotate-webhook-secret")).toBeNull();
  });

  // Three grants, three absent affordances, and a card that still lists every
  // subscription — so the reader has to be told the absence is their seat and
  // not a card that lost its buttons.
  it("states its read-only posture to a seat holding none of the three grants", async () => {
    vi.stubGlobal("fetch", backendFor({ webhook_subscription: ["read"] }));
    render(<WebhooksCard />);

    await waitFor(() =>
      expect(
        screen.getByText(
          /only an administrator or operations user can change subscriptions/i,
        ),
      ).toBeTruthy(),
    );
    expect(screen.queryByTestId("new-webhook-subscription")).toBeNull();
    expect(screen.queryByRole("button", { name: "Edit" })).toBeNull();
    // Not even the overflow: with no write grant there is nothing behind it.
    expect(screen.queryByRole("button", { name: MORE_ACTIONS })).toBeNull();
    expect(screen.queryByRole("button", { name: "Archive" })).toBeNull();
    // The subscription itself still reads: what it targets, which events it
    // takes, and its delivery health stay inspectable.
    expect(
      screen.getByText("https://example.test/hooks/margince"),
    ).toBeTruthy();
  });

  // The other direction. Without it the assertion above passes on a card that
  // shows the line to an operator too, which would deny an authority they hold.
  it("withholds the read-only line from a seat that holds the write grants", async () => {
    vi.stubGlobal("fetch", backendFor(WEBHOOK_OPERATOR));
    render(<WebhooksCard />);

    await waitFor(() =>
      expect(screen.getByTestId("new-webhook-subscription")).toBeTruthy(),
    );
    expect(
      screen.queryByText(
        /only an administrator or operations user can change subscriptions/i,
      ),
    ).toBeNull();
  });

  // Both causes at once, which is where the card has to choose. Delivery being
  // off withholds these controls from every seat including an admin's, so it is
  // the honest reason they are absent here — the seat sentence would send this
  // reader to ask for a grant that would change nothing until a key exists.
  it("blames the missing signing key rather than the seat when neither is in place", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor({ webhook_subscription: ["read"] }, 200, false),
    );
    render(<WebhooksCard />);

    await waitFor(() =>
      expect(
        screen.getByText(/not enabled on this installation/i),
      ).toBeTruthy(),
    );
    expect(
      screen.queryByText(
        /only an administrator or operations user can change subscriptions/i,
      ),
    ).toBeNull();
    expect(screen.queryByTestId("new-webhook-subscription")).toBeNull();
  });

  it("renders an honest not-enabled state on 503 webhooks_not_configured", async () => {
    vi.stubGlobal("fetch", backendFor(WEBHOOK_OPERATOR, 503));
    render(<WebhooksCard />);

    await waitFor(() =>
      expect(
        screen.getByText(/not enabled on this installation/i),
      ).toBeTruthy(),
    );
    expect(screen.queryByTestId("new-webhook-subscription")).toBeNull();
  });

  it("gates mutating controls on delivery_enabled while keeping the list readable", async () => {
    // The list works (200) but the deployment has no signing key
    // (delivery_enabled:false): existing subscriptions still render read-only,
    // but create/rotate/edit/archive are withheld — not offered controls that
    // would only 503 on click.
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        if (req.url.endsWith("/v1/me")) {
          return jsonResponse(meFixture({ allow: WEBHOOK_OPERATOR }));
        }
        if (
          req.url.includes("/webhook-subscriptions") &&
          req.method === "GET"
        ) {
          return jsonResponse({ ...SUBSCRIPTIONS, delivery_enabled: false });
        }
        throw new Error(`unexpected request: ${req.method} ${req.url}`);
      },
    );
    vi.stubGlobal("fetch", fetchMock);
    render(<WebhooksCard />);

    // The existing subscription is still listed (read-only).
    await waitFor(() =>
      expect(
        screen.getByText("https://example.test/hooks/margince"),
      ).toBeTruthy(),
    );
    expect(screen.getByText(/not enabled on this installation/i)).toBeTruthy();
    // Even for an admin, no create / rotate affordances when delivery is off.
    expect(screen.queryByTestId("new-webhook-subscription")).toBeNull();
    expect(screen.queryByTestId("rotate-webhook-secret")).toBeNull();
  });

  it("renders the empty state when no subscriptions exist", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        if (req.url.endsWith("/v1/me")) {
          return jsonResponse(meFixture({ allow: WEBHOOK_OPERATOR }));
        }
        if (
          req.url.includes("/webhook-subscriptions") &&
          req.method === "GET"
        ) {
          return jsonResponse({
            data: [],
            page: { next_cursor: null, has_more: false },
            delivery_enabled: true,
          });
        }
        throw new Error(`unexpected request: ${req.method} ${req.url}`);
      }),
    );
    render(<WebhooksCard />);

    await waitFor(() =>
      expect(screen.getByText("Nothing here yet.")).toBeTruthy(),
    );
  });

  it("hides the create trigger and reveal flow for a non-admin/ops role", async () => {
    vi.stubGlobal("fetch", backendFor({}));
    render(<WebhooksCard />);

    await waitFor(() =>
      expect(
        screen.getByText("https://example.test/hooks/margince"),
      ).toBeTruthy(),
    );
    expect(screen.queryByTestId("new-webhook-subscription")).toBeNull();
  });

  it("hides the manage row (edit/rotate/archive) for a non-admin/ops role", async () => {
    vi.stubGlobal("fetch", backendFor({}));
    render(<WebhooksCard />);

    await waitFor(() =>
      expect(
        screen.getByText("https://example.test/hooks/margince"),
      ).toBeTruthy(),
    );
    expect(screen.queryByTestId("edit-webhook-subscription")).toBeNull();
    expect(screen.queryByTestId("rotate-webhook-secret")).toBeNull();
    expect(screen.queryByTestId("archive-record")).toBeNull();
  });
});

describe("WebhooksCard — archive", () => {
  it("confirms then DELETEs /webhook-subscriptions/{id}", async () => {
    const user = userEvent.setup();
    let deleted = false;
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        if (req.url.endsWith("/v1/me")) {
          return jsonResponse(meFixture({ allow: WEBHOOK_OPERATOR }));
        }
        if (
          req.url.includes("/webhook-subscriptions") &&
          req.method === "GET"
        ) {
          return jsonResponse(SUBSCRIPTIONS);
        }
        if (req.url.includes("/sub-1") && req.method === "DELETE") {
          deleted = true;
          return jsonResponse({
            ...SUBSCRIPTIONS.data[0],
            archived_at: "2026-07-23T00:00:00Z",
          });
        }
        throw new Error(`unexpected request: ${req.method} ${req.url}`);
      },
    );
    vi.stubGlobal("fetch", fetchMock);
    render(<WebhooksCard />);

    await openRowActions(user);
    await user.click(await screen.findByTestId("archive-record"));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^Archive/,
      }),
    );

    await waitFor(() => expect(deleted).toBe(true));
  });
});

// Task 10 (B-E10.14/B-E10.15): the per-subscription deliveries + dead-letter
// inspection panel — lists the subscription's attempt log (newest-first, as
// the endpoint already orders it), honest `has_more` pagination via
// LoadMoreButton (the backend contract only carries a `limit` — there is no
// cursor query param on this endpoint, so "load more" honestly means
// re-asking for a bigger page, never a fabricated next_cursor), and a
// per-row replay action gated on webhook_subscription:update.
describe("WebhooksCard — deliveries panel (Task 10)", () => {
  const DELIVERED_DELIVERY = {
    id: "del-2",
    subscription_id: "sub-1",
    event_id: "evt-2",
    event_type: "offer.accepted",
    status: "delivered",
    attempts: 1,
    last_status_code: 200,
    last_error: null,
    next_retry_at: null,
    delivered_at: "2026-07-21T12:00:00Z",
    dead_lettered_at: null,
    created_at: "2026-07-21T11:59:00Z",
    updated_at: "2026-07-21T12:00:00Z",
  };
  const DEAD_LETTERED_DELIVERY = {
    id: "del-1",
    subscription_id: "sub-1",
    event_id: "evt-1",
    event_type: "company.updated",
    status: "dead_lettered",
    attempts: 6,
    last_status_code: 500,
    last_error: "connection refused",
    next_retry_at: null,
    delivered_at: null,
    dead_lettered_at: "2026-07-20T10:00:00Z",
    created_at: "2026-07-20T09:00:00Z",
    updated_at: "2026-07-20T10:00:00Z",
  };

  function backendForDeliveries(options: {
    hasMore: boolean;
    onReplay?: () => void;
  }) {
    const getDeliveryCalls: string[] = [];
    let replayed = false;
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        if (req.url.endsWith("/v1/me")) {
          return jsonResponse(meFixture({ allow: WEBHOOK_OPERATOR }));
        }
        if (
          req.url.includes("/webhook-subscriptions") &&
          !req.url.includes("/deliveries") &&
          req.method === "GET"
        ) {
          return jsonResponse(SUBSCRIPTIONS);
        }
        if (
          req.url.includes("/sub-1/deliveries/del-1/replay") &&
          req.method === "POST"
        ) {
          replayed = true;
          options.onReplay?.();
          return jsonResponse({
            ...DEAD_LETTERED_DELIVERY,
            status: "delivered",
          });
        }
        if (req.url.includes("/sub-1/deliveries") && req.method === "GET") {
          getDeliveryCalls.push(req.url);
          const dead = replayed
            ? { ...DEAD_LETTERED_DELIVERY, status: "delivered" }
            : DEAD_LETTERED_DELIVERY;
          return jsonResponse({
            data: [DELIVERED_DELIVERY, dead],
            page: { next_cursor: null, has_more: options.hasMore },
          });
        }
        throw new Error(`unexpected request: ${req.method} ${req.url}`);
      },
    );
    return { fetchMock, getDeliveryCalls: () => getDeliveryCalls };
  }

  function jsonResponse(body: unknown, status = 200) {
    return new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });
  }

  it("lists deliveries newest-first with status badges, grouping dead-lettered rows", async () => {
    const user = userEvent.setup();
    const { fetchMock } = backendForDeliveries({ hasMore: false });
    vi.stubGlobal("fetch", fetchMock);
    render(<WebhooksCard />);

    await user.click(await screen.findByTestId("view-deliveries"));

    await waitFor(() =>
      expect(screen.getByText("offer.accepted")).toBeTruthy(),
    );
    expect(screen.getByText("company.updated")).toBeTruthy();
    expect(screen.getByText("500")).toBeTruthy();
    expect(screen.getByText("connection refused")).toBeTruthy();
    expect(screen.getByText("Delivered")).toBeTruthy();
    expect(screen.getByText("Dead-lettered")).toBeTruthy();
    // Dead-lettered rows read as a visually distinct group, not just a badge
    // buried in an undifferentiated list.
    expect(screen.getByTestId("dead-letter-group")).toBeTruthy();
  });

  // The log is a SIBLING row of the subscription now, not a child of it — it is
  // the subject rather than an answer, so it takes the card's full width below.
  // The toggle therefore points across a boundary, and `aria-controls` naming a
  // region that is not there is the failure that move can produce: a screen
  // reader is told the button opened something and finds nothing to go to.
  it("points the toggle at the region the stacked row renders", async () => {
    const user = userEvent.setup();
    const { fetchMock } = backendForDeliveries({ hasMore: false });
    vi.stubGlobal("fetch", fetchMock);
    render(<WebhooksCard />);

    const toggle = await screen.findByTestId("view-deliveries");
    expect(toggle.getAttribute("aria-expanded")).toBe("false");

    await user.click(toggle);
    await waitFor(() =>
      expect(screen.getByText("offer.accepted")).toBeTruthy(),
    );
    expect(toggle.getAttribute("aria-expanded")).toBe("true");

    const controls = toggle.getAttribute("aria-controls");
    expect(controls).toBeTruthy();
    const region = document.getElementById(String(controls));
    // The named region exists AND is the one holding the log — not merely some
    // element that happens to carry the id.
    expect(region).toBeTruthy();
    expect(
      within(region as HTMLElement).getByText("offer.accepted"),
    ).toBeTruthy();

    // Closing takes the region away with it, so the button stops claiming to
    // control something.
    await user.click(toggle);
    expect(toggle.getAttribute("aria-expanded")).toBe("false");
    expect(document.getElementById(String(controls))).toBeNull();
  });

  it("shows LoadMoreButton honestly off the real has_more, and fetches a bigger page on click", async () => {
    const user = userEvent.setup();
    const { fetchMock, getDeliveryCalls } = backendForDeliveries({
      hasMore: true,
    });
    vi.stubGlobal("fetch", fetchMock);
    render(<WebhooksCard />);

    await user.click(await screen.findByTestId("view-deliveries"));
    await waitFor(() =>
      expect(screen.getByText("offer.accepted")).toBeTruthy(),
    );

    const loadMore = screen.getByRole("button", { name: "Load more" });
    expect(loadMore).toBeTruthy();

    await user.click(loadMore);
    await waitFor(() => expect(getDeliveryCalls().length).toBe(2));
    expect(getDeliveryCalls()[1]).not.toBe(getDeliveryCalls()[0]);
  });

  it("hides LoadMoreButton when has_more is false", async () => {
    const user = userEvent.setup();
    const { fetchMock } = backendForDeliveries({ hasMore: false });
    vi.stubGlobal("fetch", fetchMock);
    render(<WebhooksCard />);

    await user.click(await screen.findByTestId("view-deliveries"));
    await waitFor(() =>
      expect(screen.getByText("offer.accepted")).toBeTruthy(),
    );
    expect(screen.queryByRole("button", { name: "Load more" })).toBeNull();
  });

  it("replays a dead-lettered delivery via confirm, then refreshes the row", async () => {
    const user = userEvent.setup();
    const { fetchMock } = backendForDeliveries({ hasMore: false });
    vi.stubGlobal("fetch", fetchMock);
    render(<WebhooksCard />);

    await user.click(await screen.findByTestId("view-deliveries"));
    await waitFor(() =>
      expect(screen.getByText("company.updated")).toBeTruthy(),
    );

    await user.click(await screen.findByTestId("replay-delivery"));
    // The confirm names the ACT, not the dialog's mechanics — which is also
    // why it has to be found INSIDE the dialog: the row's own trigger says the
    // same word, as it should.
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: "Replay",
      }),
    );

    // The dead-lettered row refreshes to reflect the replay's outcome — the
    // list-invalidation contract (["webhook-deliveries", id]).
    await waitFor(() => {
      const badges = screen.getAllByText("Delivered");
      expect(badges.length).toBe(2);
    });
    expect(screen.queryByText("Dead-lettered")).toBeNull();
  });

  it("hides the replay action for a non-admin/ops role", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        if (req.url.endsWith("/v1/me")) {
          return jsonResponse(meFixture({ allow: {} }));
        }
        if (
          req.url.includes("/webhook-subscriptions") &&
          !req.url.includes("/deliveries") &&
          req.method === "GET"
        ) {
          return jsonResponse(SUBSCRIPTIONS);
        }
        if (req.url.includes("/sub-1/deliveries") && req.method === "GET") {
          return jsonResponse({
            data: [DEAD_LETTERED_DELIVERY],
            page: { next_cursor: null, has_more: false },
          });
        }
        throw new Error(`unexpected request: ${req.method} ${req.url}`);
      }),
    );
    render(<WebhooksCard />);

    await user.click(await screen.findByTestId("view-deliveries"));
    await waitFor(() =>
      expect(screen.getByText("company.updated")).toBeTruthy(),
    );
    expect(screen.queryByTestId("replay-delivery")).toBeNull();
  });
});

describe("WebhooksCard — rotate secret", () => {
  it("confirms, calls rotate-secret, and reveals the new secret via SecretRevealModal", async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        if (req.url.endsWith("/v1/me")) {
          return jsonResponse(meFixture({ allow: WEBHOOK_OPERATOR }));
        }
        if (
          req.url.includes("/webhook-subscriptions") &&
          req.method === "GET"
        ) {
          return jsonResponse(SUBSCRIPTIONS);
        }
        if (req.url.includes("/sub-1/rotate-secret") && req.method === "POST") {
          return jsonResponse({
            subscription: { ...SUBSCRIPTIONS.data[0], version: 2 },
            signing_secret: "whsec_rotatedNEW123==",
          });
        }
        throw new Error(`unexpected request: ${req.method} ${req.url}`);
      },
    );
    vi.stubGlobal("fetch", fetchMock);
    render(<WebhooksCard />);

    await openRowActions(user);
    await user.click(await screen.findByTestId("rotate-webhook-secret"));
    // The confirm names the ACT, not the dialog's mechanics — and it is a
    // danger button, because the old secret stops verifying the moment this
    // succeeds. Scoped to the dialog: the row's own trigger says the same
    // word, as it should.
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: "Rotate secret",
      }),
    );

    await waitFor(() =>
      expect(screen.getByText("whsec_rotatedNEW123==")).toBeTruthy(),
    );

    await user.click(screen.getByRole("button", { name: "Done" }));
    expect(screen.queryByText("whsec_rotatedNEW123==")).toBeNull();
  });
});
