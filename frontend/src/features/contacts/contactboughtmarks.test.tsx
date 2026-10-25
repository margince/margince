/** @vitest-environment happy-dom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactDetails } from "./contactdetails";
import { Employers } from "./contactemployers";
import { ContactRail } from "./contactrail";
import { ContactSubtitle } from "./contactsubtitle";

type Contact360 = components["schemas"]["Contact360"];

const CAPTURED = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
} as const;
const BOUGHT = {
  source: "surfe",
  captured_by: "connector:surfe",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
} as const;
const MARCH = "2026-03-02T12:00:00Z";
const JUNE = "2026-06-02T12:00:00Z";

// Two addresses and two numbers, one of each bought, so a mark on the wrong row
// cannot pass for a mark on the right one.
const view: Contact360 = {
  as_of: "2026-08-13T09:00:00Z",
  contact: {
    id: "p-1",
    full_name: "Dana Buyer",
    title: "Head of Revenue",
    writable: true,
    version: 1,
    social: { linkedin: "https://www.linkedin.com/in/dana" },
    primary_email: "dana@bought.example",
    emails: [
      {
        id: "e-bought",
        email: "dana@bought.example",
        email_type: "work",
        is_primary: true,
        position: 0,
        ...BOUGHT,
      },
      {
        id: "e-typed",
        email: "dana@typed.example",
        email_type: "personal",
        is_primary: false,
        position: 1,
        ...CAPTURED,
      },
    ],
    phones: [
      {
        id: "ph-bought",
        phone: "+4915112345678",
        phone_type: "mobile",
        is_primary: true,
        position: 0,
        ...BOUGHT,
      },
      {
        id: "ph-typed",
        phone: "+4930123456",
        phone_type: "work",
        is_primary: false,
        position: 1,
        ...CAPTURED,
      },
    ],
    bought_fields: [
      {
        target: "title",
        provider: "surfe",
        applied_at: JUNE,
        paid: true,
        trigger: "manual",
      },
      {
        target: "email:e-bought",
        provider: "surfe",
        applied_at: JUNE,
        paid: true,
        trigger: "manual",
      },
      {
        target: "phone:ph-bought",
        provider: "surfe",
        applied_at: JUNE,
        paid: true,
        trigger: "manual",
      },
      {
        target: "employment:rel-1",
        provider: "surfe",
        applied_at: JUNE,
        paid: true,
        trigger: "manual",
      },
    ],
    ...CAPTURED,
  },
  sections_omitted: [],
  employments: {
    data: [
      {
        relationship_id: "rel-1",
        company_id: "o-1",
        company_name: "Bought Employer",
        employment_status: "current",
        is_current_primary: true,
      },
    ],
    page: { has_more: false },
  },
  deal_roles: { data: [], page: { has_more: false } },
  profile_fields: [],
  provider_profiles: [
    {
      provider: "surfe",
      state: "completed",
      categories_not_requested: [],
      emails: [],
      mobile_phones: [],
      job_history: [],
      departments: ["Sales", "Finance"],
      seniorities: [],
      location: "Munich, Germany",
      attributes: [
        { kind: "department", value: "Finance", retrieved_at: MARCH },
        { kind: "location", value: "Munich, Germany", retrieved_at: JUNE },
        { kind: "department", value: "Sales", retrieved_at: JUNE },
      ],
    },
  ],
};

// A value that is plain text is underlined in place and named after itself; a
// value that is a control gets the word "bought" beside it, named word first.
function mark(subject: string) {
  return en["evidence.explain"].replace("{value}", subject);
}
function beside(subject: string) {
  return en["evidence.explainBeside"]
    .replace("{label}", en["evidence.bought"])
    .replace("{value}", subject);
}

function mount(shown: Contact360 = view) {
  render(
    <StoryProviders>
      <ContactSubtitle view={shown} />
      <ContactRail
        view={shown}
        consent={{
          guard: undefined,
          loading: false,
          failed: false,
          onRetry: () => undefined,
        }}
        writeRefusalId="write-refused"
      />
      <Employers view={shown} />
    </StoryProviders>,
  );
}

// What a provider sold beyond the fields is drawn with the full details on the
// Research tab, not in the left column.
function mountDetails() {
  render(
    <StoryProviders>
      <ContactDetails
        contact={view.contact}
        profiles={view.provider_profiles}
      />
    </StoryProviders>,
  );
}

beforeEach(() => {
  installFetchStub({
    "GET /me": meRoute({ contact: ["read", "update"] }, { seat: "full" }),
    "GET /companies/o-1": () =>
      jsonResponse({ id: "o-1", display_name: "Bought Employer" }),
  });
});
afterEach(cleanup);

describe("bought values on the contact page", () => {
  it("marks exactly the values the server lists as bought", async () => {
    mount();
    await screen.findByRole("button", { name: beside("dana@bought.example") });
    expect(
      screen.getByRole("button", { name: beside("+4915112345678") }),
    ).toBeTruthy();
    // The title twice: underlined in the header, marked beside the details field.
    expect(
      screen.getByRole("button", { name: mark("Head of Revenue") }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: beside("Head of Revenue") }),
    ).toBeTruthy();
    // Once in the header, once on the Employers row.
    expect(
      screen.getAllByRole("button", { name: beside("Bought Employer") }),
    ).toHaveLength(2);

    expect(
      screen.queryByRole("button", { name: beside("dana@typed.example") }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: beside("+4930123456") }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", {
        name: beside("https://www.linkedin.com/in/dana"),
      }),
    ).toBeNull();
  });

  it("never puts a mark inside another control", async () => {
    mount();
    await screen.findByRole("button", { name: beside("dana@bought.example") });
    // The Employers rows mount after their own reads: wait for the row's text.
    await waitFor(() =>
      expect(screen.getAllByText("Bought Employer").length).toBeGreaterThan(1),
    );
    const nested = document.querySelectorAll(
      "button button, a button, button a, a a, [role='button'] button",
    );
    expect(nested).toHaveLength(0);
  });

  it("shows nothing as bought once the server stops listing it", async () => {
    mount({
      ...view,
      contact: { ...view.contact, bought_fields: [] },
      provider_profiles: [],
    });
    await screen.findByText("dana@bought.example");
    expect(screen.queryAllByRole("button", { name: /came from/ })).toHaveLength(
      0,
    );
  });

  it("dates each bought location and department by the run that last reported it", async () => {
    const user = userEvent.setup();
    mountDetails();
    await screen.findByText("Bought from Surfe");
    // The month, in words, is what tells the two runs apart in any zone.
    for (const [value, at] of [
      ["Munich, Germany", "Jun"],
      ["Sales", "Jun"],
      ["Finance", "Mar"],
    ] as const) {
      await user.click(screen.getByRole("button", { name: mark(value) }));
      const receipt = await screen.findByRole("region", { name: mark(value) });
      expect(receipt.textContent).toContain(at);
    }
  });

  it("calls a free automatic run's values from the provider and names the sweep", async () => {
    const sweep = {
      ...view.contact,
      bought_fields: (view.contact.bought_fields ?? []).map((f) => ({
        ...f,
        paid: false,
        trigger: "automatic_backfill" as const,
      })),
    };
    render(
      <StoryProviders>
        <ContactSubtitle view={{ ...view, contact: sweep }} />
        <ContactDetails contact={sweep} profiles={view.provider_profiles} />
      </StoryProviders>,
    );
    await screen.findByText("From Surfe");
    expect(screen.queryByText("Bought from Surfe")).toBeNull();
    expect(screen.getByText("Added by the catch-up sweep")).toBeTruthy();
    const freeChip = en["evidence.explainBeside"]
      .replace("{label}", en["evidence.fromProvider"])
      .replace("{value}", "Head of Revenue");
    expect(screen.getByRole("button", { name: freeChip })).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: beside("Head of Revenue") }),
    ).toBeNull();
  });

  it("says a person requested a paid run and keeps the bought wording", async () => {
    const requested = {
      ...view.contact,
      bought_fields: (view.contact.bought_fields ?? []).map((f) => ({
        ...f,
        requested_by: "u-1",
      })),
    };
    render(
      <StoryProviders>
        <ContactDetails contact={requested} profiles={view.provider_profiles} />
      </StoryProviders>,
    );
    await screen.findByText("Bought from Surfe");
    expect(screen.getByText(/^Requested by/)).toBeTruthy();
  });
});
