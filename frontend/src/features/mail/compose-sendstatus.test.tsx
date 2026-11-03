/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import {
  allowedPreview,
  isPreviewDoor,
  previewedAddresses,
  refusedPreview,
} from "../../testing/sendpermission.testkit";
import { ComposeModal } from "./compose";
import {
  fillSendableForm,
  jsonResponse,
  pickWhy,
  problemResponse,
  render,
  stubRoutes,
  WHY_ASK,
  WHY_LABEL,
} from "./compose.testkit";

// The composer shows ONE status for the message: the engine's preview while
// the rep writes, and the refusal a pressed Send came back with until the
// message changes or a newer check answers. Never two answers at once.

type Preview = components["schemas"]["SendAuthorizationPreview"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const READY = { name: /ready to send/i };

const NO_DEAL = {
  reason_code: "no_compatible_evidence",
  decided_by: "machine",
  resolved_category: "active_deal_followup",
} as const;

/** The composer's routes, with the preview door answered by `engine`. */
function stubEngine(
  engine: (addresses: string[]) => Preview | Response,
  sendAnswer: () => Response,
) {
  const sent = stubRoutes({ "POST /activities/act-1/send-email": sendAnswer });
  const fetcher = vi.mocked(fetch);
  const routes = fetcher.getMockImplementation();
  if (!routes) throw new Error("missing test transport");
  fetcher.mockImplementation(async (input, init) => {
    const request = input instanceof Request ? input : null;
    const url = new URL(request ? request.url : String(input), "https://t");
    if (isPreviewDoor(url.pathname)) {
      const body: unknown = request
        ? await request.clone().json()
        : JSON.parse(String(init?.body));
      const answer = engine(previewedAddresses(body));
      return answer instanceof Response ? answer : jsonResponse(answer);
    }
    return routes(input, init);
  });
  return sent;
}

function refusedSend(refusals: unknown[]) {
  return () =>
    problemResponse(
      {
        code: "consent_not_granted",
        title: "Conflict",
        details: { review_id: "r-1", available_actions: [], refusals },
      },
      409,
    );
}

function openComposer() {
  return render(
    <ComposeModal
      activityId="act-1"
      entityType="contact"
      entityId="p-1"
      open
      onClose={vi.fn()}
    />,
  );
}

describe("a refused send and the check after it", () => {
  // The defect: the preview said ready, Send was refused, and the composer
  // drew the red refusal and "Ready to send" side by side.
  it("replaces the ready mark with the refusal, then clears it for a changed message", async () => {
    let refused = true;
    stubEngine(
      (addresses) => allowedPreview(addresses),
      () =>
        refused
          ? refusedSend([
              {
                address: "a@x.com",
                reason_code: "marketing_objection",
                category: "marketing",
              },
            ])()
          : jsonResponse({}, 202),
    );
    openComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    expect(await screen.findByRole("button", READY)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/objected to marketing/i);
    expect(alert).toHaveTextContent("This applies to a@x.com.");
    expect(screen.queryByRole("button", READY)).toBeNull();
    // The draft survives the refusal.
    expect(screen.getByLabelText("Subject")).toHaveValue("Hi there");

    // The rep changes what the message is; the old refusal is about another.
    refused = false;
    await pickWhy(WHY_LABEL.marketing);
    expect(await screen.findByRole("button", READY)).toBeInTheDocument();
    expect(screen.queryByRole("alert")).toBeNull();
  });

  it("clears the refusal when a newer check on the same message allows it", async () => {
    let fixed = false;
    stubEngine(
      (addresses) =>
        fixed
          ? allowedPreview(addresses)
          : refusedPreview(addresses[0] ?? "", NO_DEAL),
      refusedSend([
        {
          address: "a@x.com",
          reason_code: "no_compatible_evidence",
          category: "active_deal_followup",
        },
      ]),
    );
    const { queryClient } = openComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    await screen.findByText("Choose the deal for this follow-up.");

    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    const alert = await screen.findByRole("alert");
    // A missing deal is a missing detail, never a consent refusal.
    expect(alert).toHaveTextContent("Choose the deal for this follow-up.");
    expect(alert).not.toHaveTextContent(/consent/i);
    expect(
      screen.getAllByText("Choose the deal for this follow-up."),
    ).toHaveLength(1);

    fixed = true;
    await queryClient.invalidateQueries({ queryKey: ["send-permission"] });
    expect(await screen.findByRole("button", READY)).toBeInTheDocument();
    expect(screen.queryByRole("alert")).toBeNull();
    expect(
      screen.queryByText("Choose the deal for this follow-up."),
    ).toBeNull();
  });

  // A refusal whose reasons this build cannot read names no cause it does not
  // know, and keeps the review reference.
  it("says the message cannot be sent, without claiming consent, when the reasons are unreadable", async () => {
    stubEngine(
      (addresses) => allowedPreview(addresses),
      refusedSend([{ reason: "something new" }]),
    );
    openComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Message cannot be sent");
    expect(alert).not.toHaveTextContent(/consent/i);
    expect(screen.queryByRole("button", READY)).toBeNull();
  });
});

describe("several recipients", () => {
  it("names the recipient that refuses, not the one that is allowed", async () => {
    stubEngine(
      (addresses) => ({
        allowed: false,
        recipients: addresses.map((address) =>
          address === "b@x.com"
            ? {
                ...refusedPreview(address, {
                  reason_code: "marketing_objection",
                  decided_by: "subject",
                }).recipients[0],
                address,
              }
            : { ...allowedPreview([address]).recipients[0], address },
        ),
      }),
      refusedSend([]),
    );
    openComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    await userEvent.type(screen.getByLabelText("To"), "b@x.com");
    await userEvent.tab();

    expect(
      await screen.findByText("This applies to b@x.com."),
    ).toBeInTheDocument();
    expect(screen.queryByText(/a@x\.com\./)).toBeNull();
  });

  it("counts the other refused recipients of a refused send", async () => {
    stubEngine(
      (addresses) => allowedPreview(addresses),
      refusedSend([
        {
          address: "a@x.com",
          reason_code: "no_compatible_evidence",
          category: "active_deal_followup",
        },
        { address: "c@x.com", reason_code: "marketing_objection" },
      ]),
    );
    openComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));

    // The stop nobody here can lift is the one explained first.
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent(/objected to marketing/i);
    expect(alert).toHaveTextContent(
      "This applies to c@x.com and 1 other recipient.",
    );
  });
});

describe("an answer that is not a full answer is never Ready to send", () => {
  const answers: Record<string, (addresses: string[]) => Preview | Response> = {
    "an answer about nobody": () => allowedPreview([]),
    "an answer missing an addressee": () => allowedPreview(["other@x.com"]),
    "a refusal with no would_refuse": (addresses) => ({
      allowed: false,
      recipients: addresses.map((address) => ({
        address,
        verdict: "deny",
        reason_code: "marketing_objection",
      })),
    }),
    "a failed check": () => new Response(null, { status: 502 }),
  };
  for (const [name, engine] of Object.entries(answers)) {
    it(`says the check did not complete on ${name}`, async () => {
      stubEngine(engine, refusedSend([]));
      openComposer();
      await screen.findByRole("combobox", { name: WHY_ASK });
      await fillSendableForm();

      expect(
        await screen.findByText(/send check did not complete/i),
      ).toBeInTheDocument();
      expect(screen.queryByRole("button", READY)).toBeNull();
      await waitFor(() =>
        expect(screen.queryByRole("button", READY)).toBeNull(),
      );
    });
  }
});
