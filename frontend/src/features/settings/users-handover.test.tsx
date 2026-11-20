/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { UsersAdminCard } from "./users-admin";
import {
  backend,
  HANDOVER_OWNED,
  render,
  rowMenu,
} from "./users-admin.testkit";

// Deactivating moves nothing a member owns. So the members screen says what is
// left behind and hands it to a colleague the server says may own records.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("handing over a member's work", () => {
  it("hands a deactivated member's records to a colleague who may own them", async () => {
    const user = userEvent.setup();
    const calls: { method: string; url: string; body?: unknown }[] = [];
    vi.stubGlobal("fetch", backend(calls));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Otto Off")).toBeTruthy());

    await user.click(
      (await rowMenu(user, "Otto Off")).getByText("Hand over work"),
    );
    const dialog = await screen.findByRole("dialog");
    await waitFor(() =>
      expect(
        within(dialog).getByText(
          "Otto Off still owns 16 deals, 4 leads, 2 webhooks, 3 pending handoffs and 2 record responsibilities.",
        ),
      ).toBeTruthy(),
    );

    // Only colleagues the server marked assignable are offered: not the
    // member who left, not an invitation nobody redeemed.
    const receiver = within(dialog).getByRole("combobox", {
      name: /give it to/i,
    });
    await user.click(receiver);
    const listbox = await screen.findByRole("listbox");
    expect(
      within(listbox).queryByRole("option", { name: "Ivy Invited" }),
    ).toBeNull();
    expect(
      within(listbox).queryByRole("option", { name: "Otto Off" }),
    ).toBeNull();
    await user.click(
      within(listbox).getByRole("option", { name: "Ada Active" }),
    );
    await user.click(within(dialog).getByRole("button", { name: "Hand over" }));

    await waitFor(() =>
      expect(
        calls.find(
          (c) => c.method === "POST" && c.url.includes("/users/u-off/handover"),
        )?.body,
      ).toEqual({ to_user_id: "u-active" }),
    );
  });

  it("warns that drafts can be deleted, and says how many were", async () => {
    const user = userEvent.setup();
    const kit = backend([]);
    const nothing = { ...HANDOVER_OWNED, deal: 0, lead: 0, webhook: 0 };
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const req =
          input instanceof Request ? input : new Request(String(input), init);
        if (req.url.includes("/handover") && req.method === "GET") {
          return Response.json({ owned: { ...nothing, mail_draft: 2 } });
        }
        if (req.url.includes("/handover") && req.method === "POST") {
          return Response.json({
            moved: { ...nothing, mail_draft: 1 },
            left: { ...nothing, sdr_handoff: 0, record_assignment: 0 },
            mail_draft_discarded: 1,
          });
        }
        return kit(input, init);
      }),
    );
    render(
      <ToastProvider>
        <UsersAdminCard />
        <ToastRegion />
      </ToastProvider>,
    );
    await waitFor(() => expect(screen.getByText("Otto Off")).toBeTruthy());
    await user.click(
      (await rowMenu(user, "Otto Off")).getByText("Hand over work"),
    );
    const dialog = await screen.findByRole("dialog");
    await waitFor(() =>
      expect(
        within(dialog).getByText(
          /If the colleague you pick already has a draft in the same place/,
        ),
      ).toBeTruthy(),
    );
    await user.click(
      within(dialog).getByRole("combobox", { name: /give it to/i }),
    );
    await user.click(
      within(await screen.findByRole("listbox")).getByRole("option", {
        name: "Ada Active",
      }),
    );
    await user.click(within(dialog).getByRole("button", { name: "Hand over" }));
    await waitFor(() =>
      expect(
        screen.getByText(
          /1 saved email draft was deleted, because Ada Active already had a draft in the same place\./,
        ),
      ).toBeTruthy(),
    );
  });

  it("says in the deactivate dialog what the member still owns", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backend([]));
    render(<UsersAdminCard />);
    await waitFor(() => expect(screen.getByText("Ada Active")).toBeTruthy());

    await user.click(
      (await rowMenu(user, "Ada Active")).getByText("Deactivate"),
    );
    const dialog = await screen.findByRole("dialog");
    await waitFor(() =>
      expect(
        within(dialog).getByText(
          /Ada Active still owns 16 deals, 4 leads, 2 webhooks, 3 pending handoffs and 2 record responsibilities\./,
        ),
      ).toBeTruthy(),
    );
    expect(
      within(dialog).getByRole("button", { name: "Hand over work" }),
    ).toBeTruthy();
  });
});
