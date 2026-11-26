// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../../api/schema";
import type { Locale } from "../../i18n";
import { listsMe } from "../../testing/lists.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { LeadScreen } from "./leads";
import { SIGNAL_CATALOG } from "./leadsignals.fixture";

// The linked company's facts sit in a group named for that company. While the
// company holds no industry, the lead's own industry stands in.

type Lead = components["schemas"]["Lead"];

const meta: Meta = {
  title: "Records/Leads/Details",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const base: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  company_name: "Nordwind Logistik",
  status: "contacted",
  score: 72,
  source: "manual",
  captured_by: "human:u1",
  writable: true,
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const linked: Lead = {
  ...base,
  company_id: "c-7",
  company_facts: {
    industry: "logistics_transport",
    country: "DE",
    size_band: "51-200",
  },
};

function page(
  lead: Lead,
  locale: Locale = "en",
  companyName?: string,
  listsOn = false,
  allow: Parameters<typeof meRoute>[0] = {
    lead: ["read", "update"],
    company: ["read"],
  },
  tags: unknown = { data: [] },
) {
  return () => {
    installFetchStub({
      "GET /records/lead/l-1/tags": () => jsonResponse(tags),
      "GET /records/lead/l-1/lists": () =>
        jsonResponse({ data: [{ id: "s-1", name: "Launch references" }] }),
      "GET /users": () =>
        jsonResponse({
          data: [
            { id: "u-2", display_name: "Dana Fischer" },
            { id: "u-3", display_name: "Mai Nguyen" },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      "GET /users/names": () =>
        jsonResponse({ data: [{ id: "u-2", display_name: "Dana Fischer" }] }),
      "GET /lead-signal-catalog": () => jsonResponse(SIGNAL_CATALOG),
      "GET /leads/l-1/consent/stops": () => jsonResponse({ stops: [] }),
      "GET /leads/l-1": () => jsonResponse(lead),
      "GET /companies/c-7": () =>
        jsonResponse({
          id: "c-7",
          display_name: companyName ?? "Nordwind Holding SE",
        }),
      "GET /me": listsOn
        ? listsMe(true, [], {
            roles: ["rep"],
            allow: {
              lead: ["read", "update"],
              company: ["read"],
              list: ["read", "create", "update"],
            },
          })
        : meRoute(allow, { roles: ["rep"] }),
    });
    return (
      <StoryProviders locale={locale}>
        <LeadScreen id="l-1" />
      </StoryProviders>
    );
  };
}

/** A linked company that holds all three facts. */
export const CompanyFacts: Story = { render: page(linked) };

/** The lead typed "Nordwind Logistik"; the company was renamed since, and the row reads its current name. */
export const RenamedCompany: Story = {
  render: page(linked, "en", "Nordwind Group AG"),
};

/** The company holds only its size, so the lead's own industry stays editable. */
export const SomeFactsMissing: Story = {
  render: page({
    ...linked,
    industry: "software",
    company_facts: { industry: null, country: null, size_band: "1-10" },
  }),
};

/** No linked company: the industry a rep typed. */
export const TypedIndustry: Story = {
  render: page({ ...base, industry: "manufacturing" }),
};

/** A reader who may not open the company gets no facts from it. */
export const NoCompanyRead: Story = {
  render: page({ ...base, company_id: "c-7", industry: "software" }),
};

/** A company name long enough to wrap the group's heading. */
export const LongCompanyName: Story = {
  render: page(
    linked,
    "de",
    "Sindelfingen Werke für Fahrzeugtechnik und Systemintegration GmbH & Co. KG",
  ),
};

export const CompanyFactsGerman: Story = { render: page(linked, "de") };

export const CompanyFactsVietnamese: Story = { render: page(linked, "vi") };

export const CompanyFactsDark: Story = {
  globals: { theme: "dark" },
  render: page(linked),
};

export const CompanyFactsPhone: Story = {
  tags: ["uat-phone"],
  render: page(linked),
};

/** On one Shortlist, for a seat that may change lists: the chip, Add to Shortlist and the way to every list. */
export const Lists: Story = {
  render: page(base, "en", undefined, true),
  play: async () => {
    const lists = within(await screen.findByRole("region", { name: "Lists" }));
    const add = await lists.findByRole("button", { name: "Add to Shortlist" });
    // The record page arrives with a fade from opacity 0.
    await waitFor(() => expect(add).toBeVisible());
  },
};

/** Ownership's one change is the owner, so its Edit opens the owner list at once: the pick is the save. */
export const OwnerList: Story = {
  render: page({ ...base, owner_id: "u-2" }),
  play: async () => {
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Edit Ownership" }),
    );
    const list = within(await screen.findByRole("dialog"));
    await expect(
      await list.findByRole("option", { name: "Dana Fischer" }),
    ).toHaveAttribute("aria-selected", "true");
  },
};

export const OwnerListPhone: Story = { ...OwnerList, tags: ["uat-phone"] };

/** About's Edit opens one dialog with the fields the section shows; a linked company's name is fixed, with why. */
export const AboutEdit: Story = {
  render: page({ ...linked, title: "Head of fleet" }),
  play: async () => {
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Edit About" }));
    const about = within(await screen.findByRole("dialog", { name: "About" }));
    await expect(about.getByRole("textbox", { name: "Job title" })).toHaveValue(
      "Head of fleet",
    );
    await waitFor(() =>
      expect(about.getByRole("group", { name: "Company" })).toBeVisible(),
    );
  },
};

export const AboutEditDark: Story = {
  ...AboutEdit,
  globals: { theme: "dark" },
};

export const AboutEditPhone: Story = { ...AboutEdit, tags: ["uat-phone"] };

/** Email and phone holds the address, the number and the profile; the profile shows locked, with its reason. */
export const ReachEdit: Story = {
  render: page({ ...base, phone: "+49301234567" }),
  play: async () => {
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Edit Email and phone" }),
    );
    const reach = within(
      await screen.findByRole("dialog", { name: "Email and phone" }),
    );
    await expect(reach.getByRole("textbox", { name: "Email" })).toHaveValue(
      "jonas@nordwind.example",
    );
    await waitFor(() =>
      expect(reach.getByRole("group", { name: "LinkedIn URL" })).toBeVisible(),
    );
  },
};

export const ReachEditPhone: Story = { ...ReachEdit, tags: ["uat-phone"] };

/** The project row leaves the dialog for the project search, which stands alone. */
export const ProjectFlow: Story = {
  render: page(base),
  play: async () => {
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Edit About" }));
    const about = within(await screen.findByRole("dialog", { name: "About" }));
    await user.click(about.getByRole("button", { name: "Change Project" }));
    const search = await screen.findByRole("dialog", {
      name: "Attach project",
    });
    await waitFor(() => expect(search).toBeVisible());
  },
};

/** A reader who may only read the lead gets the readings and no Edit. */
export const ReadOnly: Story = {
  render: page(base, "en", undefined, false, {
    lead: ["read"],
    company: ["read"],
  }),
  play: async () => {
    await screen.findByRole("region", { name: "About" });
    await expect(screen.queryByRole("button", { name: /^Edit / })).toBeNull();
  },
};

/** A reader the vocabulary is withheld from reads that, not an empty Tags row. */
export const TagsWithheld: Story = {
  render: page(
    base,
    "en",
    undefined,
    false,
    { lead: ["read"], company: ["read"] },
    { data: [], withheld: true },
  ),
  play: async () => {
    const registration = within(
      await screen.findByRole("region", { name: "Registration" }),
    );
    const note = await registration.findByText("Hidden for your role");
    // The page fades in as it arrives, so the note is visible once that ends.
    await waitFor(() => expect(note).toBeVisible());
  },
};
