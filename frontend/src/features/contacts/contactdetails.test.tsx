/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { RecordDetails } from "../../design-system/recorddetails";
import { en } from "../../i18n/en";
import { view } from "../../testing/contactpage.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactDetails, useContactAboutSections } from "./contactdetails";

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
  await user.click(await screen.findByRole("button", { name: "Edit emails" }));
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
  await user.click(await screen.findByRole("button", { name: "Edit emails" }));
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
  await user.click(await screen.findByRole("button", { name: "Edit emails" }));
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
    en["common.versionSkew"],
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
    await screen.findByRole("button", { name: "Edit emails" }),
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

it("adds the first phone from Details without changing email", async () => {
  const user = userEvent.setup();
  const sent: unknown[] = [];
  mount(
    (body) => {
      sent.push(body);
      return jsonResponse(contact);
    },
    { ...contact, phones: [] },
  );
  await user.click(
    await screen.findByRole("button", { name: "Edit phone numbers" }),
  );
  await user.click(screen.getByRole("button", { name: "Add phone" }));
  await user.type(
    screen.getByRole("textbox", { name: "Phone" }),
    "+442079460001",
  );
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() =>
    expect(sent).toEqual([
      {
        phones: [
          {
            phone: "+442079460001",
            phone_type: "work",
            is_primary: false,
            position: 0,
          },
        ],
      },
    ]),
  );
});

describe("the address in the About group", () => {
  function About({ page }: Readonly<{ page: typeof view }>) {
    return <RecordDetails sections={useContactAboutSections(page)} />;
  }
  const address = (
    id: string,
    email: string,
    position: number,
  ): components["schemas"]["ContactEmail"] => ({
    id,
    email,
    email_type: "work",
    is_primary: position === 1,
    position,
    source: "manual",
    captured_by: "human:u-1",
  });
  const front = address("pe-1", "dana@brandt.example", 0);
  const second = address("pe-2", "dana.private@brandt.example", 1);

  function show(primaryEmail: string | null) {
    installFetchStub({
      "GET /me": meRoute({ contact: ["read"] }, { seat: "full" }),
    });
    render(
      <StoryProviders>
        <About
          page={{
            ...view,
            contact: {
              ...view.contact,
              emails: [front, second],
              primary_email: primaryEmail,
            },
          }}
        />
      </StoryProviders>,
    );
  }

  it("is the server's address, not the first row", async () => {
    show("dana.private@brandt.example");
    expect(
      await screen.findByText("dana.private@brandt.example"),
    ).toBeInTheDocument();
    expect(screen.queryByText("dana@brandt.example")).toBeNull();
  });

  it("is empty when the server names none", async () => {
    const user = userEvent.setup();
    show(null);
    await user.click(
      await screen.findByRole("button", { name: /^Show \d+ empty field/ }),
    );
    expect(screen.getByText(en["create.email"])).toBeInTheDocument();
    expect(screen.queryByText(/@brandt\.example/)).toBeNull();
  });
});

describe("a postal address whose country does not fit", () => {
  function About({ page }: Readonly<{ page: typeof view }>) {
    return <RecordDetails sections={useContactAboutSections(page)} />;
  }
  const berlin = { postal_code: "10115", city: "Berlin", country: "AT" };

  function show(contact: (typeof view)["contact"]) {
    installFetchStub({
      "GET /me": meRoute({ contact: ["read"] }, { seat: "full" }),
    });
    render(
      <StoryProviders>
        <About page={{ ...view, contact }} />
      </StoryProviders>,
    );
  }

  it("flags the address with the country the postal code and city name", async () => {
    show({
      ...view.contact,
      address: berlin,
      address_country_mismatch: { resolved_country: "DE" },
    });
    expect(
      await screen.findByText("Postal code and city are in Germany"),
    ).toBeInTheDocument();
    expect(screen.getByText("Berlin, AT")).toBeInTheDocument();
  });

  it("shows no flag when the server reports no mismatch", async () => {
    show({ ...view.contact, address: berlin });
    expect(await screen.findByText("Berlin, AT")).toBeInTheDocument();
    expect(screen.queryByText(/Postal code and city are in/)).toBeNull();
  });
});
