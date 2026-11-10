/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { sessionOptions } from "../../lib/session";
import { meFixture } from "../../testing/mefixture";
import {
  jsonResponse,
  problemResponse,
  render,
  stubRoutes,
} from "./compose.testkit";
import { RecordConversationFact } from "./composecontextfact";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const target = {
  subject: { entity_type: "contact", entity_id: "contact-one" },
  address: "anna@example.test",
  category: "precontract_quote",
} as const;

it.each([
  { allow: { contact: ["read"] }, seat: "full" },
  { allow: { contact: ["read", "update"] }, seat: "read" },
] as const)(
  "requires a contact update grant and a full seat: %j",
  async ({ allow, seat }) => {
    const me = meFixture({ allow, seat });
    const sent = stubRoutes({ "GET /me": () => jsonResponse(me) });
    const { queryClient } = render(<RecordConversationFact target={target} />);
    await waitFor(() =>
      expect(queryClient.getQueryData(sessionOptions.queryKey)).toEqual(me),
    );
    expect(
      screen.queryByRole("button", { name: "Record what happened" }),
    ).toBeNull();
    expect(sent.some((call) => call.key.startsWith("POST "))).toBe(false);
  },
);

it.each(["contact", "lead"] as const)(
  "keeps a %s request, recipient and deal after a failed save, then retries without sending",
  async (kind) => {
    const subject = { entity_type: kind, entity_id: "subject-one" };
    const verbs: readonly ("read" | "update")[] = ["read", "update"];
    const endpoint = `POST /${kind === "lead" ? "leads" : "contacts"}/subject-one/consent/qualifying-events`;
    const user = userEvent.setup();
    let attempts = 0;
    const sent = stubRoutes({
      "GET /me": () =>
        jsonResponse(meFixture({ allow: { [kind]: verbs } })),
      [endpoint]: () => {
        attempts += 1;
        return attempts === 1
          ? problemResponse(
              { title: "Could not record the exchange", status: 503 },
              503,
            )
          : jsonResponse({ id: "request-one" }, 201);
      },
    });
    render(
      <RecordConversationFact
        target={{ ...target, subject }}
        dealId="deal-one"
      />,
    );
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
      "Asked for a revised quote by phone.",
    );
    await user.type(dialog.getByLabelText("Date"), "2020-03-02");
    await user.tab();
    await user.type(dialog.getByLabelText("Time"), "09:00");
    await user.tab();
    await user.click(
      dialog.getByRole("button", { name: "Record the exchange" }),
    );
    await waitFor(() => expect(attempts).toBe(1));
    expect((await dialog.findByRole("alert")).textContent).toMatch(/./);
    const note = dialog.getByLabelText("What was agreed?");
    expect(note).toBeInstanceOf(HTMLTextAreaElement);
    if (!(note instanceof HTMLTextAreaElement))
      throw new Error("Missing note field");
    expect(note.value).toBe("Asked for a revised quote by phone.");
    await user.click(
      dialog.getByRole("button", { name: "Record the exchange" }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    const writes = sent.filter((call) => call.key.startsWith("POST "));
    expect(writes).toHaveLength(2);
    for (const write of writes) {
      expect(write.key).toBe(endpoint);
      expect(write.body).toMatchObject({
        kind: "requested_by_subject",
        note: "Asked for a revised quote by phone.",
        asks_for_quote: true,
        about_deal: "deal-one",
      });
    }
  },
);

it("does not offer a lead repair under a contact-only update grant", async () => {
  const me = meFixture({
    allow: { contact: ["read", "update"], lead: ["read"] },
  });
  const sent = stubRoutes({ "GET /me": () => jsonResponse(me) });
  const { queryClient } = render(
    <RecordConversationFact
      target={{
        ...target,
        subject: { entity_type: "lead", entity_id: "lead-one" },
      }}
    />,
  );
  await waitFor(() =>
    expect(queryClient.getQueryData(sessionOptions.queryKey)).toEqual(me),
  );
  expect(
    screen.queryByRole("button", { name: "Record what happened" }),
  ).toBeNull();
  expect(sent.some((call) => call.key.startsWith("POST "))).toBe(false);
});
