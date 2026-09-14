/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import { afterEach, expect, it } from "vitest";
import type { components } from "../api/schema";
import { ContactDetails } from "./contactdetails";
import { view } from "./contactpage.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

afterEach(cleanup);
const contact = { ...view.contact, version: 3, title: "Director" };
function mount(
  patch: (body: unknown, request?: Request) => Response,
  current = contact,
) {
  installFetchStub({
    "GET /me": meRoute({ contact: ["read", "update"] }, { seat: "full" }),
    "PATCH /contacts/p-1": patch,
  });
  render(
    <StoryProviders>
      <ContactDetails contact={current} />
    </StoryProviders>,
  );
}
it("saves only the corrected scalar", async () => {
  const user = userEvent.setup();
  const sent: unknown[] = [];
  mount((body) => {
    sent.push(body);
    return jsonResponse({ ...contact, version: 4 });
  });
  await user.click(await screen.findByRole("button", { name: "Change Title" }));
  await user.clear(screen.getByRole("textbox", { name: "Title" }));
  await user.type(screen.getByRole("textbox", { name: "Title" }), "CEO{Enter}");
  await waitFor(() => expect(sent).toEqual([{ title: "CEO" }]));
});
it("corrects an email while preserving other addresses, types and primary flags", async () => {
  const user = userEvent.setup();
  const sent: unknown[] = [];
  const emails: components["schemas"]["Contact"]["emails"] = [
    ...(contact.emails ?? []),
    {
      source: "manual",
      captured_by: "human:u1",
      position: 1,
      id: "e2",
      email: "personal@example.test",
      email_type: "personal",
      is_primary: false,
    },
  ];
  mount(
    (body) => {
      sent.push(body);
      return jsonResponse(contact);
    },
    { ...contact, emails },
  );
  await user.click(await screen.findByRole("button", { name: "Change Email" }));
  const input = screen.getByDisplayValue("dana@brandt.example");
  await user.clear(input);
  await user.type(input, "dana@brandt.de");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() =>
    expect(sent).toEqual([
      {
        emails: [
          {
            email: "dana@brandt.de",
            email_type: "work",
            is_primary: true,
            position: 0,
          },
          {
            email: "personal@example.test",
            email_type: "personal",
            is_primary: false,
            position: 1,
          },
        ],
      },
    ]),
  );
});
it("allows removing the final address", async () => {
  const user = userEvent.setup();
  const sent: unknown[] = [];
  mount((body) => {
    sent.push(body);
    return jsonResponse(contact);
  });
  await user.click(await screen.findByRole("button", { name: "Change Email" }));
  await user.click(screen.getByRole("button", { name: /^Remove row/ }));
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(sent).toEqual([{ emails: [] }]));
});
it("keeps a refused correction and offers the existing record on a duplicate", async () => {
  const user = userEvent.setup();
  mount(() =>
    jsonResponse(
      {
        type: "about:blank",
        title: "Conflict",
        status: 409,
        code: "duplicate_email",
        details: { existing_id: "p2" },
      },
      409,
    ),
  );
  await user.click(await screen.findByRole("button", { name: "Change Email" }));
  const input = screen.getByDisplayValue("dana@brandt.example");
  await user.clear(input);
  await user.type(input, "taken@example.test");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await screen.findByRole("button", { name: "View existing record" });
  expect(screen.getByDisplayValue("taken@example.test")).toBeTruthy();
  expect(screen.queryByRole("dialog")).toBeNull();
});
it("shows friendly conflict copy and retains the draft", async () => {
  const user = userEvent.setup();
  mount(() =>
    jsonResponse(
      {
        type: "about:blank",
        title: "Conflict",
        status: 409,
        code: "version_skew",
        detail: "internal version mismatch",
      },
      409,
    ),
  );
  await user.click(await screen.findByRole("button", { name: "Change Title" }));
  const input = screen.getByRole("textbox", { name: "Title" });
  await user.clear(input);
  await user.type(input, "CEO{Enter}");
  expect((await screen.findByRole("alert")).textContent).toContain(
    "This record changed since you opened it",
  );
  expect(screen.getByDisplayValue("CEO")).toBeTruthy();
  expect(screen.queryByText("internal version mismatch")).toBeNull();
});

it("retains the email action beside its field editor", async () => {
  mount(() => jsonResponse(contact));
  expect(
    await screen.findByRole("link", { name: "dana@brandt.example" }),
  ).toHaveAttribute("href", "mailto:dana@brandt.example");
  expect(
    await screen.findByRole("button", { name: "Change Email" }),
  ).toBeTruthy();
});
it("does not offer to clear the owner of a private contact", async () => {
  const user = userEvent.setup();
  mount(() => jsonResponse(contact), {
    ...contact,
    visibility: "owner",
    owner_id: "u1",
  });
  await user.click(await screen.findByRole("button", { name: "Change Owner" }));
  await user.click(screen.getByRole("combobox"));
  expect(screen.queryByRole("option", { name: "Not set" })).toBeNull();
});
