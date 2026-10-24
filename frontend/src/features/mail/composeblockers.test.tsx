/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
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
import { reasonTargetOf } from "./composeblockers";

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

const settled = (preview: ReturnType<typeof refusedPreview>) => ({
  preview,
  asking: false,
  unanswered: false,
});

const contactOf = (address: string) =>
  address === "anna@example.test" ? "c-1" : undefined;

describe("who a reason to write is recorded on", () => {
  it("is the contact behind the deciding address, for its category", () => {
    expect(
      reasonTargetOf(
        settled(refusedPreview("anna@example.test", NOTHING_ON_RECORD)),
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
        settled(refusedPreview("stranger@example.test", NOTHING_ON_RECORD)),
        contactOf,
      ),
    ).toBeUndefined();
    expect(
      reasonTargetOf(
        settled(
          refusedPreview("anna@example.test", {
            reason_code: "marketing_objection",
            decided_by: "subject",
          }),
        ),
        contactOf,
      ),
    ).toBeUndefined();
  });

  // A vouch resting on an answer the engine is replacing would be recorded
  // for the wrong message.
  it("is nobody while the answer is still out or failed", () => {
    const refused = refusedPreview("anna@example.test", NOTHING_ON_RECORD);
    expect(
      reasonTargetOf({ ...settled(refused), asking: true }, contactOf),
    ).toBeUndefined();
    expect(
      reasonTargetOf({ ...settled(refused), unanswered: true }, contactOf),
    ).toBeUndefined();
  });
});

// The backend a vouch is recorded against: the preview door answers from the
// vouch, as the engine would.
function vouchBackend() {
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
  return sent;
}

describe("recording a reason to write where the refusal is read", () => {
  it("records it on the contact for the category the engine resolved, then asks again", async () => {
    const user = userEvent.setup();
    const sent = vouchBackend();
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

    // The refusal carries the pick itself: choosing a reason records it.
    const checks = within(screen.getAllByRole("dialog")[0]);
    await pickOption(
      user,
      await checks.findByRole("combobox", {
        name: "Why you may write to anna@example.test",
      }),
      "Follow-up they requested",
    );

    await waitFor(() =>
      expect(
        sent.find((call) => call.key === "POST /contacts/c-1/consent/allow")
          ?.body,
      ).toEqual({
        category: "requested_followup",
        reason: "Follow-up they requested",
      }),
    );
    // Asked again, the engine allows it and the warning goes.
    await waitFor(() =>
      expect(
        screen.queryByRole("combobox", {
          name: "Why you may write to anna@example.test",
        }),
      ).toBeNull(),
    );
  });

  it("asks once on a new message, then records the reason it names", async () => {
    const user = userEvent.setup();
    const sent = vouchBackend();
    render(
      <ComposeModal
        entityType="contact"
        entityId="c-1"
        contactId="c-1"
        open
        onClose={vi.fn()}
      />,
    );
    await user.type(await screen.findByLabelText("To"), "anna@example.test");
    await user.tab();

    // Until the message says why, the engine's refusal does not ask again.
    const checks = within(screen.getAllByRole("dialog")[0]);
    const why = await checks.findByRole("combobox", {
      name: "Reason for contact",
    });
    expect(
      checks.queryByRole("combobox", {
        name: "Why you may write to anna@example.test",
      }),
    ).toBeNull();

    await pickOption(user, why, "Follow-up they requested");
    await user.click(
      await checks.findByRole("button", {
        name: "Record this reason: Follow-up they requested, for anna@example.test",
      }),
    );

    await waitFor(() =>
      expect(
        sent.find((call) => call.key === "POST /contacts/c-1/consent/allow")
          ?.body,
      ).toEqual({
        category: "requested_followup",
        reason: "Follow-up they requested",
      }),
    );
  });
});
