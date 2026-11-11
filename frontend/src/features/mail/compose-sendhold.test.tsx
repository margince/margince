// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { messageBox } from "../../design-system/richtext-testing";
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
  WHY_LABEL,
} from "./compose.testkit";

// Send follows the engine's answer for the message as it stands. It waits
// while a check runs or after one failed. It stays off for a refusal no reason
// may lift, and asks for a reason where one may lift every refusal.

type Preview = components["schemas"]["SendAuthorizationPreview"];
type Recipient = Preview["recipients"][number];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const WAITS = "Send waits for the send check.";
const OFF =
  "Send is off. Fix what the check above names, or remove that recipient.";
const WITH_REASON =
  "The check refused this message. Send asks for your reason, and the reason covers this one message only.";

const OBJECTED = {
  reason_code: "marketing_objection",
  decided_by: "subject",
} as const;

const NOTHING_ON_RECORD = {
  reason_code: "no_compatible_evidence",
  decided_by: "machine",
  can_be_overruled: true,
  resolved_category: "requested_followup",
} as const;

const REVIEW = {
  id: "r-1",
  state: "open",
  kind: "single",
  reason_code: "no_compatible_evidence",
  refusals: [],
  warning: {
    version: "override-v2",
    text: "The send check still blocks this message.",
  },
};

type Engine = (addresses: string[]) => Preview | Response | Promise<Response>;

/** The composer's routes, with the preview door answered by `engine`. */
function stubEngine(
  engine: Engine,
  overrides: Parameters<typeof stubRoutes>[0] = {},
) {
  const sent = stubRoutes(overrides);
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
      const answer = await engine(previewedAddresses(body));
      return answer instanceof Response ? answer : jsonResponse(answer);
    }
    return routes(input, init);
  });
  return sent;
}

/** One preview answer that waits until the test releases it. */
function heldAnswer() {
  let release: (answer: Response) => void = () => {
    throw new Error("No preview is waiting");
  };
  const answer = new Promise<Response>((resolve) => {
    release = resolve;
  });
  return {
    answer,
    release: (preview: Preview) => release(jsonResponse(preview)),
  };
}

function openComposer() {
  return render(
    <ComposeModal
      entityType="contact"
      entityId="c-1"
      contactId="c-1"
      open
      onClose={vi.fn()}
    />,
  );
}

const sendButton = (name: string | RegExp = "Send") =>
  screen.getByRole<HTMLButtonElement>("button", { name });

const posted = (sent: ReturnType<typeof stubRoutes>) =>
  sent.filter((call) => call.key === "POST /emails");

/** Every recipient but `refused` allowed; `refused` answered with `over`. */
function oneRefused(
  refused: string,
  over: Parameters<typeof refusedPreview>[1],
): Engine {
  return (addresses) => ({
    allowed: false,
    recipients: addresses.map(
      (address): Recipient =>
        address === refused
          ? { ...refusedPreview(address, over).recipients[0], address }
          : { ...allowedPreview([address]).recipients[0], address },
    ),
  });
}

describe("Send follows the check", () => {
  it("is off for a refusal, waits while the check runs, and frees once it allows", async () => {
    let held: ReturnType<typeof heldAnswer> | undefined;
    stubEngine((addresses) =>
      held ? held.answer : refusedPreview(addresses[0] ?? "", OBJECTED),
    );
    openComposer();
    await fillSendableForm();
    await screen.findByText(OFF);
    expect(sendButton()).toBeDisabled();

    held = heldAnswer();
    await pickWhy(WHY_LABEL.marketing);
    await screen.findByText(WAITS);
    expect(sendButton()).toBeDisabled();

    held.release(allowedPreview(["a@x.com"]));
    await screen.findByRole("button", { name: /recipients checked/i });
    expect(sendButton()).toBeEnabled();
    expect(screen.queryByText(WAITS)).toBeNull();
    expect(screen.queryByText(OFF)).toBeNull();
  });

  it("names the one refused recipient of two and stays off", async () => {
    const sent = stubEngine(oneRefused("b@x.com", OBJECTED));
    openComposer();
    await fillSendableForm();
    await userEvent.type(screen.getByLabelText("To"), "b@x.com");
    await userEvent.tab();

    await screen.findByText("This applies to b@x.com.");
    expect(screen.queryByText(/a@x\.com/, { selector: "p" })).toBeNull();
    await screen.findByText(OFF);
    await userEvent.click(sendButton());
    expect(posted(sent)).toHaveLength(0);
  });

  it("gives each refused recipient its own line", async () => {
    stubEngine((addresses) => ({
      allowed: false,
      recipients: addresses.map((address) => ({
        ...refusedPreview(
          address,
          address === "a@x.com" ? OBJECTED : NOTHING_ON_RECORD,
        ).recipients[0],
        address,
      })),
    }));
    openComposer();
    await fillSendableForm();
    await userEvent.type(screen.getByLabelText("To"), "b@x.com");
    await userEvent.tab();

    expect(
      await screen.findByText(/^a@x\.com: The recipient objected to marketing/),
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        /^b@x\.com: Sending is refused until a reason is recorded/,
      ),
    ).toBeInTheDocument();
    // One stop among them holds Send for every recipient.
    expect(sendButton()).toBeDisabled();
  });

  it("waits when the check failed, and frees once it is asked again", async () => {
    let failing = true;
    stubEngine((addresses) =>
      failing ? new Response(null, { status: 502 }) : allowedPreview(addresses),
    );
    openComposer();
    await fillSendableForm();
    await screen.findByText(WAITS);
    expect(sendButton()).toBeDisabled();
    expect(
      screen.getByText(
        "The send check did not complete. Send waits until it does.",
      ),
    ).toBeInTheDocument();

    failing = false;
    await userEvent.click(screen.getByRole("button", { name: "Check again" }));
    await waitFor(() => expect(sendButton()).toBeEnabled());
  });

  // The key of the answer is the whole question. An answer about the earlier
  // recipients is not one about the message now on screen.
  it("does not free Send with the answer for the recipients before a change", async () => {
    let held: ReturnType<typeof heldAnswer> | undefined;
    stubEngine((addresses) => (held ? held.answer : allowedPreview(addresses)));
    openComposer();
    await fillSendableForm();
    await screen.findByRole("button", { name: /recipients checked/i });
    expect(sendButton()).toBeEnabled();

    held = heldAnswer();
    await userEvent.type(screen.getByLabelText("To"), "b@x.com");
    await userEvent.tab();
    await screen.findByText(WAITS);
    expect(sendButton()).toBeDisabled();
    expect(
      screen.queryByRole("button", { name: /recipients checked/i }),
    ).toBeNull();

    held.release(allowedPreview(["a@x.com", "b@x.com"]));
    await waitFor(() => expect(sendButton()).toBeEnabled());
  });

  it("holds Cmd/Ctrl+Enter whenever it holds the button", async () => {
    let allowed = false;
    const sent = stubEngine((addresses) =>
      allowed
        ? allowedPreview(addresses)
        : refusedPreview(addresses[0] ?? "", OBJECTED),
    );
    openComposer();
    await fillSendableForm();
    await screen.findByText(OFF);

    fireEvent.keyDown(messageBox("Body"), { key: "Enter", ctrlKey: true });
    fireEvent.keyDown(messageBox("Body"), { key: "Enter", metaKey: true });

    // Once the check allows it the same keys send. The one request is for the
    // message as allowed: a held press that had gone through would be the
    // earlier, refused one.
    allowed = true;
    await pickWhy(WHY_LABEL.marketing);
    await screen.findByRole("button", { name: /recipients checked/i });
    fireEvent.keyDown(messageBox("Body"), { key: "Enter", ctrlKey: true });
    await waitFor(() => expect(posted(sent)).toHaveLength(1));
    expect(posted(sent)[0]?.body).toMatchObject({
      communication_context: "marketing",
    });
  });
});

describe("a refusal a reason may lift", () => {
  it("says so on Send, and the refused press opens the reason form", async () => {
    const sent = stubEngine(
      (addresses) => refusedPreview(addresses[0] ?? "", NOTHING_ON_RECORD),
      {
        "POST /emails": () =>
          problemResponse(
            {
              code: "consent_not_granted",
              title: "Conflict",
              details: {
                review_id: "r-1",
                available_actions: ["direct_send"],
                refusals: [
                  {
                    address: "a@x.com",
                    reason_code: "no_compatible_evidence",
                    category: "requested_followup",
                    can_be_overruled: true,
                  },
                ],
              },
            },
            409,
          ),
        "GET /communication-reviews/r-1": () => jsonResponse(REVIEW),
      },
    );
    openComposer();
    await fillSendableForm();
    await screen.findByText(WITH_REASON);
    const press = sendButton("Send with a reason…");
    expect(press).toBeEnabled();

    await userEvent.click(press);
    expect(
      await screen.findByRole("button", {
        name: "Record the exception and send",
      }),
    ).toBeInTheDocument();
    expect(posted(sent)).toHaveLength(1);

    // Closed without sending, the next press reopens it and asks nothing new.
    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Record the exception and send" }),
      ).toBeNull(),
    );
    await userEvent.click(sendButton("Send with a reason…"));
    await screen.findByRole("button", {
      name: "Record the exception and send",
    });
    expect(posted(sent)).toHaveLength(1);
  });

  it("offers no exception for a refusal nobody may lift", async () => {
    const sent = stubEngine((addresses) =>
      refusedPreview(addresses[0] ?? "", OBJECTED),
    );
    openComposer();
    await fillSendableForm();
    await screen.findByText(OFF);
    expect(sendButton()).toBeDisabled();
    expect(
      screen.queryByRole("button", { name: "Send with a reason…" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Send with a recorded exception" }),
    ).toBeNull();
    expect(posted(sent)).toHaveLength(0);
  });
});
