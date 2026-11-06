/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { writeMessage } from "../../design-system/richtext-testing";
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
import { refusalIsCurrent } from "./composesend";

// The composer shows one status for the message. It is the engine's preview
// while the rep writes. After a refused Send it is that refusal, until the
// message changes or a newer check answers.

type Preview = components["schemas"]["SendAuthorizationPreview"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const READY = { name: /recipients checked/i };

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
  // drew the red refusal and "Recipients checked" side by side.
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
    expect(
      await screen.findByRole("button", { name: /recipients checked/i }),
    ).toBeInTheDocument();

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
    expect(
      await screen.findByRole("button", { name: /recipients checked/i }),
    ).toBeInTheDocument();
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
    expect(
      await screen.findByRole("button", { name: /recipients checked/i }),
    ).toBeInTheDocument();
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

describe("an edited message is a different message", () => {
  // A remedy on a stale refusal would act on the message as it was sent, while
  // looking like it is about the draft on screen.
  it.each([
    [
      "the subject",
      async () => {
        await userEvent.type(screen.getByLabelText("Subject"), " again");
      },
    ],
    [
      "the body",
      async () => {
        writeMessage("Body", "Different words");
      },
    ],
  ])("clears the refusal and its remedies when %s changes", async (_, edit) => {
    stubEngine(
      (addresses) => allowedPreview(addresses),
      () =>
        problemResponse(
          {
            code: "consent_not_granted",
            title: "Conflict",
            details: {
              review_id: "r-1",
              available_actions: ["overrule"],
              overrulable: [{ contact_id: "c-1", category: "marketing" }],
              refusals: [
                {
                  address: "a@x.com",
                  reason_code: "no_marketing_consent",
                  can_be_overruled: true,
                },
              ],
            },
          },
          409,
        ),
    );
    openComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await fillSendableForm();
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Consent needed for this message");

    await edit();
    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
    expect(screen.queryByText("Consent needed for this message")).toBeNull();
  });
});

describe("the mark beside Send", () => {
  // The consent check passing is not the whole send. A form still missing a
  // field will stop at the button, so the mark makes no passed claim.
  it("claims no passed check while a field is missing", async () => {
    const asks: string[][] = [];
    stubEngine((addresses) => {
      asks.push(addresses);
      return allowedPreview(addresses);
    }, refusedSend([]));
    openComposer();
    await screen.findByRole("combobox", { name: WHY_ASK });
    await userEvent.type(screen.getByLabelText("To"), "a@x.com");
    await userEvent.tab();
    await pickWhy(WHY_LABEL.requestedFollowup);
    await waitFor(() => expect(asks.length).toBeGreaterThan(0));
    await waitFor(() =>
      expect(screen.queryByText(/checking whether/i)).toBeNull(),
    );
    expect(screen.queryByRole("button", READY)).toBeNull();

    await userEvent.type(screen.getByLabelText("Subject"), "Hi there");
    writeMessage("Body", "Body content");
    expect(
      await screen.findByRole("button", { name: /recipients checked/i }),
    ).toBeInTheDocument();
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

describe("an answer that is not a full answer is never a passed check", () => {
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
    "an answer naming someone not asked": (addresses) => ({
      ...allowedPreview(addresses),
      recipients: [
        ...allowedPreview(addresses).recipients,
        ...refusedPreview("stranger@x.com", {
          reason_code: "marketing_objection",
          decided_by: "subject",
        }).recipients,
      ],
    }),
    "an answer naming one addressee twice": (addresses) =>
      allowedPreview([...addresses, ...addresses]),
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

describe("whether a refusal is about the message on screen", () => {
  type Request = Parameters<typeof refusalIsCurrent>[1];
  const answerAtSend = allowedPreview(["a@x.com"]);
  const sent: Request = {
    activityId: "act-1",
    isChannelReply: false,
    mail: {
      subject: "Hi",
      body: "Words",
      to: ["a@x.com"],
      attachment_ids: ["f-1"],
      mail_draft_id: undefined,
    },
    channelBody: { body: "Words", attachment_ids: ["f-1"] },
    links: [],
    answerAtSend,
  };

  it("is, while nothing the request carries has changed", () => {
    // A draft id arriving after the press is bookkeeping, not a new message.
    const now = { ...sent, mail: { ...sent.mail, mail_draft_id: "d-1" } };
    expect(refusalIsCurrent(sent, now, answerAtSend)).toBe(true);
  });

  it("is not, once a file is added", () => {
    const now = {
      ...sent,
      mail: { ...sent.mail, attachment_ids: ["f-1", "f-2"] },
      channelBody: { ...sent.channelBody, attachment_ids: ["f-1", "f-2"] },
    };
    expect(refusalIsCurrent(sent, now, answerAtSend)).toBe(false);
  });

  it("is not, once the check answers anew", () => {
    expect(refusalIsCurrent(sent, sent, allowedPreview(["a@x.com"]))).toBe(
      false,
    );
  });
});
