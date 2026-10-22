/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { openChecks } from "../../testing/composedraft.testkit";
import {
  allowedPreview,
  isPreviewDoor,
  refusedPreview,
} from "../../testing/sendpermission.testkit";
import { ComposeModal } from "./compose";
import {
  activity202,
  jsonResponse,
  render,
  stubRoutes,
} from "./compose.testkit";
import { checksVerdict, reasonTargetOf } from "./composechecks";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const NOTHING_ON_RECORD = {
  reason_code: "no_compatible_evidence",
  decided_by: "machine",
  can_be_overruled: true,
  resolved_category: "requested_followup",
} as const;

const contactOf = (address: string) =>
  address === "anna@example.test" ? "c-1" : undefined;

describe("the count the foot shows", () => {
  const quiet = { preview: undefined, asking: false, unanswered: false };

  it("says checking while the engine is asked", () => {
    expect(
      checksVerdict({
        permission: { ...quiet, asking: true },
        missingReason: true,
        sharedUnsubscribe: false,
      }),
    ).toEqual({ state: "checking" });
  });

  it("counts a refusal, a missing reason and a shared unsubscribe", () => {
    expect(
      checksVerdict({
        permission: {
          ...quiet,
          preview: refusedPreview("anna@example.test", NOTHING_ON_RECORD),
        },
        missingReason: true,
        sharedUnsubscribe: true,
      }),
    ).toEqual({ state: "blocked", count: 3 });
  });

  // Ready is a claim about an answer, so a message nobody asked about is not.
  it("says ready only on an engine answer that allows", () => {
    const nothingToAsk = {
      missingReason: false,
      sharedUnsubscribe: false,
    };
    expect(checksVerdict({ permission: quiet, ...nothingToAsk })).toEqual({
      state: "unasked",
    });
    expect(
      checksVerdict({
        permission: {
          ...quiet,
          preview: allowedPreview(["anna@example.test"]),
        },
        ...nothingToAsk,
      }),
    ).toEqual({ state: "ready" });
  });
});

describe("who a reason to write is recorded on", () => {
  it("is the contact behind the deciding address, for its category", () => {
    expect(
      reasonTargetOf(
        refusedPreview("anna@example.test", NOTHING_ON_RECORD),
        contactOf,
      ),
    ).toEqual({
      contactId: "c-1",
      address: "anna@example.test",
      category: "requested_followup",
    });
  });

  // A form that could only answer 404, or one for a refusal nobody may lift,
  // is the dead control the permission surface refuses to draw.
  it("is nobody where the composer knows no contact or nobody may lift it", () => {
    expect(
      reasonTargetOf(
        refusedPreview("stranger@example.test", NOTHING_ON_RECORD),
        contactOf,
      ),
    ).toBeUndefined();
    expect(
      reasonTargetOf(
        refusedPreview("anna@example.test", {
          reason_code: "marketing_objection",
          decided_by: "subject",
        }),
        contactOf,
      ),
    ).toBeUndefined();
  });
});

describe("recording a reason to write where the refusal is read", () => {
  it("records it on the contact for the category the engine resolved, then asks again", async () => {
    const user = userEvent.setup();
    let vouched = false;
    const sent = stubRoutes({
      "GET /activities/act-1": () =>
        jsonResponse({ ...activity202, direction: "inbound" }),
      "GET /contacts/c-1/360": () =>
        jsonResponse({
          as_of: "2026-08-01T09:00:00Z",
          sections_omitted: [],
          contact: {
            id: "c-1",
            full_name: "Anna Brandt",
            emails: [{ email: "anna@example.test" }],
          },
        }),
      "POST /contacts/c-1/consent/allow": () => {
        vouched = true;
        return jsonResponse({ override_id: "o-1" }, 201);
      },
    });
    // The preview door answers from the vouch, as the engine would.
    const fetcher = vi.mocked(fetch);
    const stubbed = fetcher.getMockImplementation();
    if (!stubbed) throw new Error("Missing test transport");
    fetcher.mockImplementation(async (input, init) => {
      const url = input instanceof Request ? input.url : String(input);
      if (isPreviewDoor(new URL(url, "https://test.local").pathname)) {
        return jsonResponse(
          vouched
            ? allowedPreview(["anna@example.test"])
            : refusedPreview("anna@example.test", NOTHING_ON_RECORD),
        );
      }
      return stubbed(input, init);
    });
    render(
      <ComposeModal
        activityId="act-1"
        entityType="contact"
        entityId="c-1"
        contactId="c-1"
        open
        onClose={vi.fn()}
      />,
    );
    await screen.findByText(/Replying to/);
    await user.type(screen.getByLabelText("To"), "anna@example.test");
    await user.tab();

    await screen.findByRole("button", { name: "1 check blocks sending" });
    const checks = within(openChecks());
    await user.click(
      await checks.findByRole("button", { name: "Record reason for contact" }),
    );
    await user.type(
      checks.getByLabelText("Why you may write to anna@example.test"),
      "She asked for the revised quote at the fair.",
    );
    await user.click(checks.getByRole("button", { name: "Record reason" }));

    await waitFor(() =>
      expect(
        sent.find((call) => call.key === "POST /contacts/c-1/consent/allow")
          ?.body,
      ).toEqual({
        category: "requested_followup",
        reason: "She asked for the revised quote at the fair.",
      }),
    );
    expect(
      await screen.findByRole("button", { name: "Ready to send" }),
    ).toBeTruthy();
  });
});
