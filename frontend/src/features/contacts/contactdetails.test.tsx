/** @vitest-environment happy-dom */
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { RecordDetails } from "../../design-system/recorddetails";
import { en } from "../../i18n/en";
import { openChoiceDialog } from "../../testing/choicedialog.testkit";
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
function mount(patch: (body: unknown) => Response, current = contact) {
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
  await user.click(
    await screen.findByRole("button", { name: /^Change Title/ }),
  );
  await user.clear(screen.getByRole("textbox", { name: "Title" }));
  await user.type(screen.getByRole("textbox", { name: "Title" }), "CEO{Enter}");
  await waitFor(() => expect(sent).toEqual([{ title: "CEO" }]));
});
it("renames with the full name alone, leaving the server to drop the old parts", async () => {
  const user = userEvent.setup();
  const sent: unknown[] = [];
  const named = {
    ...contact,
    full_name: "Stefan Altmann",
    first_name: "Stefan",
    last_name: "Altmann",
  };
  mount((body) => {
    sent.push(body);
    return jsonResponse({ ...named, version: 4 });
  }, named);
  await user.click(
    await screen.findByRole("button", { name: /^Change Full name/ }),
  );
  const box = screen.getByRole("textbox", { name: /Full name/ });
  await user.clear(box);
  await user.type(box, "Maria Neumann{Enter}");
  await waitFor(() => expect(sent).toEqual([{ full_name: "Maria Neumann" }]));
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
  expect(
    within(screen.getByRole("dialog", { name: "Email" })).getByDisplayValue(
      "taken@example.test",
    ),
  ).toBeTruthy();
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
  await user.click(
    await screen.findByRole("button", { name: /^Change Title/ }),
  );
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
  await user.click((await openChoiceDialog(user, "Owner")).chooser);
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
    await screen.findByRole("button", { name: /^Edit phone numbers/ }),
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
            is_primary: true,
            position: 0,
          },
        ],
      },
    ]),
  );
});

describe("the address in the About group", () => {
  function About({ page }: Readonly<{ page: typeof view }>) {
    return <RecordDetails sections={useContactAboutSections(page).sections} />;
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

  it("leads with the server's address, not the first row", async () => {
    show("dana.private@brandt.example");
    await screen.findByText("dana.private@brandt.example");
    // Compared by accessible name: an address drawn in two halves reads whole.
    const lines = screen.getAllByRole("link", { name: /@brandt\.example$/ });
    expect(lines).toHaveLength(2);
    expect(lines[0]).toHaveAccessibleName("dana.private@brandt.example");
    expect(lines[1]).toHaveAccessibleName("dana@brandt.example");
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
    return <RecordDetails sections={useContactAboutSections(page).sections} />;
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
    expect(screen.getByText("10115 Berlin AT")).toBeInTheDocument();
  });

  it("keeps the flag on an address row that reads empty", async () => {
    show({
      ...view.contact,
      address: {},
      address_country_mismatch: { resolved_country: "DE" },
    });
    await userEvent.click(
      await screen.findByRole("button", { name: /^Show \d+ empty field/ }),
    );
    expect(
      await screen.findByText("Postal code and city are in Germany"),
    ).toBeInTheDocument();
  });

  it("shows no flag when the server reports no mismatch", async () => {
    show({ ...view.contact, address: berlin });
    expect(await screen.findByText("10115 Berlin AT")).toBeInTheDocument();
    expect(screen.queryByText(/Postal code and city are in/)).toBeNull();
  });
});
it("converts the lead that holds an edited address by saving again", async () => {
  const user = userEvent.setup();
  const sent: Record<string, unknown>[] = [];
  mount((body) => {
    const sentBody = body as Record<string, unknown>;
    sent.push(sentBody);
    if (!sentBody.convert_lead_ids) {
      return jsonResponse(
        {
          type: "about:blank",
          title: "Conflict",
          status: 409,
          code: "lead_holds_email",
          details: {
            existing_id: "lead-9",
            lead_ids: ["lead-9"],
            convertible: true,
            field: "emails",
          },
        },
        409,
      );
    }
    return jsonResponse({ ...contact, version: 4 });
  });
  await user.click(await screen.findByRole("button", { name: "Edit emails" }));
  const input = screen.getByDisplayValue("dana@brandt.example");
  await user.clear(input);
  await user.type(input, "dana@lead.example");
  await user.click(screen.getByRole("button", { name: "Save" }));
  await user.click(
    await screen.findByRole("button", { name: "Convert the lead" }),
  );
  await waitFor(() => expect(sent).toHaveLength(2));
  expect(sent[1]).toMatchObject({
    convert_lead_ids: ["lead-9"],
    emails: [expect.objectContaining({ email: "dana@lead.example" })],
  });
});

describe("the left column's sections", () => {
  function Column({ page }: Readonly<{ page: typeof view }>) {
    const about = useContactAboutSections(page);
    return (
      <>
        <RecordDetails sections={about.sections} />
        {about.element}
      </>
    );
  }
  const placed = {
    ...contact,
    owner_id: "u-1",
    address: { city: "Berlin", country: "DE" },
  };

  function showColumn(
    allow: Parameters<typeof meRoute>[0],
    patch: (body: unknown) => Response = () => jsonResponse(placed),
  ) {
    installFetchStub({
      "GET /me": meRoute(allow, { seat: "full" }),
      "PATCH /contacts/p-1": patch,
    });
    render(
      <StoryProviders>
        <Column page={{ ...view, contact: placed }} />
      </StoryProviders>,
    );
  }

  it("offers one Edit per section, and rests every value as a reading", async () => {
    showColumn({ contact: ["read", "update"] });
    for (const name of ["Edit About", "Edit Address", "Edit Ownership"]) {
      expect(await screen.findByRole("button", { name })).toHaveTextContent(
        en["record.edit"],
      );
    }
    expect(screen.queryByRole("button", { name: /^Change / })).toBeNull();
    expect(
      screen.getByRole("link", { name: "dana@brandt.example" }),
    ).toHaveAttribute("href", "mailto:dana@brandt.example");
  });

  it("offers no Edit to a reader who may not change the contact", async () => {
    showColumn({ contact: ["read"] });
    await screen.findByText("Director");
    expect(screen.queryByRole("button", { name: /^Edit / })).toBeNull();
  });

  it("changes About's fields in one dialog and sends the changed title alone", async () => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    showColumn({ contact: ["read", "update"] }, (body) => {
      sent.push(body);
      return jsonResponse({ ...placed, title: "CEO", version: 4 });
    });
    await user.click(await screen.findByRole("button", { name: "Edit About" }));
    const dialog = within(await screen.findByRole("dialog", { name: "About" }));
    expect(dialog.queryByRole("textbox", { name: "City" })).toBeNull();
    expect(
      dialog.getByRole("button", { name: "Change Employer" }),
    ).toBeTruthy();
    const title = dialog.getByRole("textbox", { name: "Title" });
    await user.clear(title);
    await user.type(title, "CEO");
    await user.click(dialog.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(sent).toEqual([{ title: "CEO" }]));
  });

  it("changes the address in a dialog of its own", async () => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    showColumn({ contact: ["read", "update"] }, (body) => {
      sent.push(body);
      return jsonResponse(placed);
    });
    await user.click(
      await screen.findByRole("button", { name: "Edit Address" }),
    );
    const dialog = within(
      await screen.findByRole("dialog", { name: "Address" }),
    );
    expect(dialog.queryByRole("textbox", { name: "Title" })).toBeNull();
    const city = dialog.getByRole("textbox", { name: "City" });
    await user.clear(city);
    await user.type(city, "Hamburg");
    await user.click(dialog.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(sent).toEqual([
        { address: expect.objectContaining({ city: "Hamburg" }) },
      ]),
    );
  });
});

describe("the name parts in About", () => {
  function Column({ page }: Readonly<{ page: typeof view }>) {
    const about = useContactAboutSections(page);
    return (
      <>
        <RecordDetails sections={about.sections} />
        {about.element}
      </>
    );
  }

  it("corrects the first name a greeting reads in About's dialog, and sends only it", async () => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    const named = { ...contact, first_name: "Dana", last_name: "Buyer" };
    installFetchStub({
      "GET /me": meRoute({ contact: ["read", "update"] }, { seat: "full" }),
      "PATCH /contacts/p-1": (body) => {
        sent.push(body);
        return jsonResponse({ ...named, first_name: "Daniela" });
      },
    });
    render(
      <StoryProviders>
        <Column page={{ ...view, contact: named }} />
      </StoryProviders>,
    );
    const about = await screen.findByRole("region", {
      name: en["record.details.about"],
    });
    expect(within(about).getByText("Dana")).toBeTruthy();
    expect(within(about).getByText("Buyer")).toBeTruthy();
    await user.click(await screen.findByRole("button", { name: "Edit About" }));
    const dialog = within(await screen.findByRole("dialog", { name: "About" }));
    const input = dialog.getByRole("textbox", { name: en["create.firstName"] });
    await user.clear(input);
    await user.type(input, "Daniela");
    await user.click(dialog.getByRole("button", { name: "Save" }));
    // The name parts are one group, so the unchanged last name rides along;
    // the server reads a restated part as no change.
    await waitFor(() =>
      expect(sent).toEqual([{ first_name: "Daniela", last_name: "Buyer" }]),
    );
  });
});

describe("the tags in the About group", () => {
  function About({ page }: Readonly<{ page: typeof view }>) {
    const about = useContactAboutSections(page);
    return (
      <>
        <RecordDetails sections={about.sections} />
        {about.element}
      </>
    );
  }

  it("folds an untagged contact's Tags row and asks for no tags", async () => {
    let asked = 0;
    installFetchStub({
      "GET /me": meRoute({ contact: ["read", "update"] }, { seat: "full" }),
      "GET /records/contact/p-1/tags": () => {
        asked += 1;
        return jsonResponse({ data: [], withheld: false });
      },
    });
    render(
      <StoryProviders>
        <About page={{ ...view, contact: { ...contact, tags: [] } }} />
      </StoryProviders>,
    );
    const about = await screen.findByRole("region", {
      name: en["record.details.about"],
    });
    expect(within(about).queryByText(en["tags.panelTitle"])).toBeNull();
    expect(asked).toBe(0);
  });

  it("shows the tags the contact carries and the verb to add one", async () => {
    installFetchStub({
      "GET /me": meRoute({ contact: ["read", "update"] }, { seat: "full" }),
      "GET /records/contact/p-1/tags": () =>
        jsonResponse({
          data: [
            {
              tag_id: "t-1",
              name: "Champion",
              color: "teal",
              archived: false,
              assigned_at: "2026-03-03T10:00:00Z",
            },
          ],
          withheld: false,
        }),
    });
    render(
      <StoryProviders>
        <About
          page={{
            ...view,
            contact: {
              ...contact,
              tags: [{ tag_id: "t-1", name: "Champion", color: "teal" }],
            },
          }}
        />
      </StoryProviders>,
    );
    const about = await screen.findByRole("region", {
      name: en["record.details.about"],
    });
    await within(about).findByText("Champion");
    expect(within(about).getByText(en["tags.panelTitle"])).toBeTruthy();
    expect(
      within(about).getByRole("button", { name: en["tags.add"] }),
    ).toBeTruthy();
  });

  it("keeps the row and the focus in it when the last tag is removed", async () => {
    const user = userEvent.setup();
    let held = [{ tag_id: "t-1", name: "Champion", archived: false }];
    installFetchStub({
      "GET /me": meRoute({ contact: ["read", "update"] }, { seat: "full" }),
      "GET /records/contact/p-1/tags": () =>
        jsonResponse({ data: held, withheld: false }),
      "DELETE /tags/t-1/apply": () => {
        held = [];
        return jsonResponse({ restore: "r-1" });
      },
    });
    const tagged = { ...view, contact: { ...contact, tags: [held[0]] } };
    const { rerender } = render(
      <StoryProviders>
        <About page={tagged} />
      </StoryProviders>,
    );
    await user.click(
      await screen.findByRole("button", { name: "Remove Champion" }),
    );
    await waitFor(() => expect(screen.queryByText("Champion")).toBeNull());
    rerender(
      <StoryProviders>
        <About page={{ ...view, contact: { ...contact, tags: [] } }} />
      </StoryProviders>,
    );
    const about = screen.getByRole("region", {
      name: en["record.details.about"],
    });
    expect(within(about).getByText(en["tags.panelTitle"])).toBeTruthy();
    expect(about.contains(document.activeElement)).toBe(true);
  });
});
