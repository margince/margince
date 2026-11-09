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
  contactId: "contact-one",
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

it("keeps the recorded request, recipient and deal after a failed save, then retries without sending", async () => {
  const user = userEvent.setup();
  let attempts = 0;
  const sent = stubRoutes({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { contact: ["read", "update"] } })),
    "POST /contacts/contact-one/consent/qualifying-events": () => {
      attempts += 1;
      return attempts === 1
        ? problemResponse(
            { title: "Could not record the exchange", status: 503 },
            503,
          )
        : jsonResponse({ id: "request-one" }, 201);
    },
  });
  render(<RecordConversationFact target={target} dealId="deal-one" />);
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
  await user.click(dialog.getByRole("button", { name: "Record the exchange" }));
  await waitFor(() => expect(attempts).toBe(1));
  expect(await dialog.findByRole("alert")).toHaveTextContent(/./);
  expect(dialog.getByLabelText("What was agreed?")).toHaveValue(
    "Asked for a revised quote by phone.",
  );
  await user.click(dialog.getByRole("button", { name: "Record the exchange" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  const writes = sent.filter((call) => call.key.startsWith("POST "));
  expect(writes).toHaveLength(2);
  for (const write of writes) {
    expect(write.key).toBe(
      "POST /contacts/contact-one/consent/qualifying-events",
    );
    expect(write.body).toMatchObject({
      kind: "requested_by_subject",
      note: "Asked for a revised quote by phone.",
      asks_for_quote: true,
      about_deal: "deal-one",
    });
  }
});
