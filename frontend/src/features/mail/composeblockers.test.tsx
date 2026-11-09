/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { createQueryClient } from "../../app/queryclient";
import { writeMessage } from "../../design-system/richtext-testing";
import { pickOption } from "../../design-system/select-testing";
import { meFixture } from "../../testing/mefixture";
import {
  allowedPreview,
  isPreviewDoor,
  refusedPreview,
} from "../../testing/sendpermission.testkit";
import { ComposeModal } from "./compose";
import { MailSendNotices } from "./compose.parts";
import {
  activity202,
  jsonResponse,
  render,
  stubRoutes,
} from "./compose.testkit";
import { newContactsOf, reasonTargetOf } from "./composeblockers";

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

// The rep claimed an active deal and no deal backs it. The engine records the
// claim and resolves to its own reading, which differs.
const DEAL_CLAIM_UNBACKED = {
  ...NOTHING_ON_RECORD,
  requested_category: "active_deal_followup",
} as const;

const settled = (preview: ReturnType<typeof refusedPreview>) => ({
  preview,
  refusal: null,
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

  it("keeps the requested category when its factual evidence is missing", () => {
    expect(
      reasonTargetOf(
        settled(refusedPreview("anna@example.test", DEAL_CLAIM_UNBACKED)),
        contactOf,
      ),
    ).toEqual({
      contactId: "c-1",
      address: "anna@example.test",
      category: "active_deal_followup",
    });
    expect(
      reasonTargetOf(
        settled(
          refusedPreview("anna@example.test", {
            ...NOTHING_ON_RECORD,
            requested_category: "requested_followup",
          }),
        ),
        contactOf,
      ),
    ).toEqual({
      contactId: "c-1",
      address: "anna@example.test",
      category: "requested_followup",
    });
  });

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

function requestBackend(
  refused: Parameters<typeof refusedPreview>[1] = NOTHING_ON_RECORD,
  dealId?: string,
) {
  let recorded = false;
  const sent = stubRoutes({
    "GET /users": () => jsonResponse({ data: [], page: { has_more: false } }),
    "GET /deals/deal-one": () =>
      jsonResponse({
        id: "deal-one",
        company_id: "company-one",
        name: "The quote deal",
      }),
    "GET /companies/company-one/360": () =>
      jsonResponse({
        company: { id: "company-one", display_name: "Acme" },
        contacts: {
          data: [
            {
              contact_id: "c-1",
              primary_email: "anna@example.test",
              full_name: "Anna Brandt",
            },
          ],
          page: { has_more: false },
        },
        projects: [],
      }),

    "GET /activities/act-1": () =>
      jsonResponse({
        ...activity202,
        direction: "inbound",
        links: dealId ? [{ entity_type: "deal", entity_id: dealId }] : [],
      }),
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
    "GET /me": () =>
      jsonResponse(
        meFixture({
          allow: { contact: ["read", "update"], activity: ["read", "update"] },
        }),
      ),
    "POST /contacts/c-1/consent/qualifying-events": () => {
      recorded = true;
      return jsonResponse({ id: "fact-one" }, 201);
    },
  });
  const fetcher = vi.mocked(fetch);
  const stubbed = fetcher.getMockImplementation();
  if (!stubbed) throw new Error("Missing test transport");
  fetcher.mockImplementation(async (input, init) => {
    const url = input instanceof Request ? input.url : String(input);
    if (isPreviewDoor(new URL(url, "https://test.local").pathname)) {
      return jsonResponse(
        recorded
          ? allowedPreview(["anna@example.test"])
          : refusedPreview("anna@example.test", refused),
      );
    }
    return stubbed(input, init);
  });
  return sent;
}

describe("recording a fact where the refusal is read", () => {
  it("records the exchange for this recipient and rechecks without sending or granting consent", async () => {
    const user = userEvent.setup();
    const sent = requestBackend();
    render(
      <ComposeModal
        activityId="act-1"
        entityType="contact"
        entityId="c-1"
        contactId="c-1"
        open
        onClose={vi.fn()}
      />,
      createQueryClient(),
    );
    await user.type(await screen.findByLabelText("To"), "anna@example.test");
    await user.tab();
    writeMessage("Body", "The agreed follow-up.");
    await user.click(
      await screen.findByRole("button", { name: "Record what happened" }),
    );
    const dialog = within(
      await screen.findByRole("dialog", {
        name: "Record the exchange with anna@example.test",
      }),
    );
    await pickOption(
      user,
      dialog.getByRole("combobox", { name: "What happened?" }),
      "They asked me to write",
    );
    await user.type(
      dialog.getByLabelText("What was agreed?"),
      "They asked by phone for the next steps.",
    );
    await user.type(dialog.getByLabelText("Date"), "2020-03-02");
    await user.tab();
    await user.type(dialog.getByLabelText("Time"), "09:00");
    await user.tab();
    await user.click(
      dialog.getByRole("button", { name: "Record the exchange" }),
    );
    await waitFor(() =>
      expect(
        sent.find(
          (call) => call.key === "POST /contacts/c-1/consent/qualifying-events",
        )?.body,
      ).toMatchObject({
        kind: "requested_by_subject",
        note: "They asked by phone for the next steps.",
        asks_for_quote: false,
      }),
    );
    await waitFor(() =>
      expect(
        screen.queryByRole("button", { name: "Record what happened" }),
      ).toBeNull(),
    );
    expect(
      sent.some(
        (call) =>
          call.key.endsWith("/consent/allow") ||
          call.key.endsWith("/send-email") ||
          call.key === "POST /emails",
      ),
    ).toBe(false);
  });

  it.each(["deal page", "anchored reply"])(
    "keeps the %s deal on a recorded quote request",
    async (source) => {
      const user = userEvent.setup();
      const sent = requestBackend(
        {
          ...NOTHING_ON_RECORD,
          requested_category: "precontract_quote",
          resolved_category: "precontract_quote",
        },
        "deal-one",
      );
      render(
        <ComposeModal
          entityType={source === "deal page" ? "deal" : "contact"}
          entityId={source === "deal page" ? "deal-one" : "c-1"}
          contactId="c-1"
          activityId={source === "anchored reply" ? "act-1" : undefined}
          open
          onClose={vi.fn()}
        />,
      );
      await user.type(await screen.findByLabelText("To"), "anna@example.test");
      await user.tab();
      if (source === "deal page") {
        await pickOption(
          user,
          screen.getByRole("combobox", { name: "Reason for contact" }),
          "Quote or proposal they requested",
        );
      }
      await user.click(
        await screen.findByRole("button", { name: "Record what happened" }),
      );
      const dialog = within(
        await screen.findByRole("dialog", {
          name: "Record the exchange with anna@example.test",
        }),
      );
      await pickOption(
        user,
        dialog.getByRole("combobox", { name: "What happened?" }),
        "They asked for this quote",
      );
      await user.type(
        dialog.getByLabelText("What was agreed?"),
        "They asked for a quote for this deal.",
      );
      await user.type(dialog.getByLabelText("Date"), "2020-03-02");
      await user.tab();
      await user.type(dialog.getByLabelText("Time"), "09:00");
      await user.tab();
      await user.click(
        dialog.getByRole("button", { name: "Record the exchange" }),
      );
      await waitFor(() =>
        expect(
          sent.find(
            (call) =>
              call.key === "POST /contacts/c-1/consent/qualifying-events",
          )?.body,
        ).toMatchObject({
          kind: "requested_by_subject",
          asks_for_quote: true,
          about_deal: "deal-one",
        }),
      );
      expect(
        sent.some(
          (call) =>
            call.key === "POST /emails" || call.key.endsWith("/send-email"),
        ),
      ).toBe(false);
    },
  );

  it("asks for the message's reason first and makes no standing override from that choice", async () => {
    const user = userEvent.setup();
    const sent = requestBackend();
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
    expect(
      screen.queryByRole("button", { name: "Record what happened" }),
    ).toBeNull();
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Reason for contact" }),
      "Follow-up they requested",
    );
    await screen.findByRole("button", { name: "Record what happened" });
    expect(
      sent.some(
        (call) =>
          call.key.endsWith("/consent/allow") ||
          call.key.endsWith("/consent/qualifying-events"),
      ),
    ).toBe(false);
  });

  it("asks for a deal before offering a factual request for that deal", async () => {
    const user = userEvent.setup();
    const sent = requestBackend(DEAL_CLAIM_UNBACKED);
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

    const checks = within(screen.getAllByRole("dialog")[0]);
    await pickOption(
      user,
      await checks.findByRole("combobox", { name: "Reason for contact" }),
      "Active deal",
    );

    await checks.findByText("Choose the deal for this follow-up.");
    expect(
      checks.queryByRole("button", { name: "Record what happened" }),
    ).toBeNull();
    expect(
      sent.some(
        (call) =>
          call.key === "POST /contacts/c-1/consent/allow" ||
          call.key === "POST /contacts/c-1/consent/qualifying-events",
      ),
    ).toBe(false);
  });
});

// An address no record holds is not a refusal: the send files it as the rep's
// contact. The composer says so once the engine's answer is in, and names no
// address while that answer is still out.
describe("a new address", () => {
  const preview = allowedPreview(["new@prospect.test", "known@customer.test"]);
  preview.recipients[0] = { ...preview.recipients[0], new_contact: true };

  it("is named from the settled answer only", () => {
    expect(newContactsOf(settled(preview))).toEqual(["new@prospect.test"]);
    expect(newContactsOf({ ...settled(preview), asking: true })).toEqual([]);
    expect(newContactsOf({ ...settled(preview), unanswered: true })).toEqual(
      [],
    );
  });

  it("says the send adds it as the rep's contact", () => {
    render(
      <MailSendNotices
        to={["new@prospect.test", "known@customer.test"]}
        cc={[]}
        context={undefined}
        newContacts={newContactsOf(settled(preview))}
      />,
    );
    expect(
      screen.getByText(
        "new@prospect.test is on no record yet. Sending adds it as your contact.",
      ),
    ).toBeTruthy();
    expect(
      screen.queryByText(/known@customer\.test is on no record/),
    ).toBeNull();
  });
});

// The engine answers every preview with the one it is handed.
function answeringBackend(preview: ReturnType<typeof refusedPreview>) {
  const answered = { count: 0 };
  stubRoutes({});
  const fetcher = vi.mocked(fetch);
  const stubbed = fetcher.getMockImplementation();
  if (!stubbed) throw new Error("Missing test transport");
  fetcher.mockImplementation(async (input, init) => {
    const url = input instanceof Request ? input.url : String(input);
    if (isPreviewDoor(new URL(url, "https://test.local").pathname)) {
      answered.count += 1;
      return jsonResponse(preview);
    }
    return stubbed(input, init);
  });
  return answered;
}

async function newMessageTo(address: string) {
  const user = userEvent.setup();
  render(
    <ComposeModal
      entityType="contact"
      entityId="c-1"
      contactId="c-1"
      open
      onClose={vi.fn()}
    />,
  );
  await user.type(await screen.findByLabelText("To"), address);
  await user.tab();
  return within(screen.getAllByRole("dialog")[0]);
}

describe("a message with no reason for contact yet", () => {
  it("says the reason is missing once, in the strip that picks it", async () => {
    const answered = answeringBackend(
      refusedPreview("anna@example.test", {
        reason_code: "unknown_purpose",
        decided_by: "machine",
      }),
    );
    const checks = await newMessageTo("anna@example.test");

    expect(await checks.findByText("No contact reason.")).toBeTruthy();
    await waitFor(() => {
      expect(answered.count).toBeGreaterThan(0);
      expect(checks.queryAllByText(/^Checking whether/)).toHaveLength(0);
    });
    expect(checks.queryByText("This message needs one more detail")).toBeNull();
  });

  it("shows a refusal for another cause beside the strip", async () => {
    answeringBackend(
      refusedPreview("anna@example.test", {
        reason_code: "marketing_objection",
        decided_by: "subject",
      }),
    );
    const checks = await newMessageTo("anna@example.test");

    expect(await checks.findByText("Message cannot be sent")).toBeTruthy();
    expect(checks.getByText("No contact reason.")).toBeTruthy();
  });

  it("shows a missing consent beside the strip", async () => {
    answeringBackend(
      refusedPreview("anna@example.test", {
        reason_code: "no_marketing_consent",
        decided_by: "machine",
      }),
    );
    const checks = await newMessageTo("anna@example.test");

    expect(
      await checks.findByText("Permission needed for this message"),
    ).toBeTruthy();
    expect(checks.getByText("No contact reason.")).toBeTruthy();
  });
});

it("keeps an open exchange and its note through a background permission check", async () => {
  requestBackend();
  const user = userEvent.setup();
  const { queryClient } = render(
    <ComposeModal
      activityId="act-1"
      entityType="contact"
      entityId="c-1"
      contactId="c-1"
      open
      onClose={vi.fn()}
    />,
    createQueryClient(),
  );
  await user.type(await screen.findByLabelText("To"), "anna@example.test");
  await user.tab();
  writeMessage("Body", "The agreed follow-up.");
  await user.click(
    await screen.findByRole("button", { name: "Record what happened" }),
  );
  const dialog = await screen.findByRole("dialog", {
    name: "Record the exchange with anna@example.test",
  });
  await user.type(
    within(dialog).getByLabelText("What was agreed?"),
    "Asked for a revised quote.",
  );
  let finishCheck: ((answer: Response) => void) | undefined;
  const pending = new Promise<Response>((resolve) => {
    finishCheck = resolve;
  });
  const fetcher = vi.mocked(fetch);
  const backend = fetcher.getMockImplementation();
  if (!backend) throw new Error("Missing test transport");
  fetcher.mockImplementation((input, init) => {
    const url = input instanceof Request ? input.url : String(input);
    return isPreviewDoor(new URL(url, "https://test.local").pathname)
      ? pending
      : backend(input, init);
  });
  let refreshed: Promise<void> | undefined;
  act(() => {
    refreshed = queryClient.invalidateQueries({
      queryKey: ["send-permission"],
    });
  });
  expect(
    await screen.findByText("Checking whether this message can be sent…"),
  ).toBeInTheDocument();
  expect(within(dialog).getByLabelText("What was agreed?")).toHaveValue(
    "Asked for a revised quote.",
  );
  expect(dialog).toBeInTheDocument();
  const answerCheck = finishCheck;
  if (!answerCheck) throw new Error("No preview is waiting");
  await act(async () => {
    answerCheck(jsonResponse(allowedPreview(["anna@example.test"])));
    await refreshed;
  });
  expect(dialog).toBeInTheDocument();
  expect(within(dialog).getByLabelText("What was agreed?")).toHaveValue(
    "Asked for a revised quote.",
  );
});
