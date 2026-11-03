/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { CompanyProfileDetails } from "./companyprofiledetails";
import {
  company,
  emptyPage,
  jsonResponse,
  render,
  stub,
} from "./companyrail.testkit";

// The fields the details column does not draw live here, all visible and all
// editable. Nothing on the record is reachable only by being folded.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const EDITABLE = { canEdit: true } as const;
const NO_FIELDS = { fields: [], fieldsLoaded: true } as const;

const writableSeat = () =>
  jsonResponse({
    user: { id: "u-1", display_name: "Mira Voss" },
    authorization: {
      objects: { company: { update: true } },
      seat_type: "full",
    },
  });

describe("CompanyProfileDetails", () => {
  it("draws the eight fields the details column leaves out, set or not", async () => {
    stub();
    render(
      <CompanyProfileDetails
        company={{
          ...company,
          linkedin_url: null,
          legal_name: null,
          phone: null,
          email: null,
        }}
        edit={EDITABLE}
        {...NO_FIELDS}
      />,
    );

    const panel = screen
      .getByRole("heading", { name: en["co.details.title"] })
      .closest("section");
    expect(panel).not.toBeNull();
    for (const label of [
      en["create.legalName"],
      en["company.relationshipTypes"],
      en["history.field.parent_company_id"],
      en["create.linkedinUrl"],
      en["create.phone"],
      en["create.email"],
      en["co.profileField.registered_address"],
      en["tags.panelTitle"],
    ]) {
      expect(panel && within(panel).getByText(label)).toBeVisible();
    }
  });

  it("keeps the LinkedIn URL a link, and edits it from the bubble beside it", async () => {
    let patchBody: unknown;
    stub({
      "/me": writableSeat,
      "/companies/o-1": async (request) => {
        if (request.method === "PATCH") {
          patchBody = await request.json();
          return jsonResponse({ ...company, version: 2 });
        }
        return jsonResponse(company);
      },
    });
    render(
      <CompanyProfileDetails
        company={company}
        edit={EDITABLE}
        {...NO_FIELDS}
      />,
    );

    expect(
      screen.getByRole("link", { name: "https://linkedin.com/company/brandt" }),
    ).toHaveAttribute("href", "https://linkedin.com/company/brandt");
    await userEvent.click(
      await screen.findByRole("button", { name: "Change LinkedIn URL" }),
    );
    const input = screen.getByRole("textbox", { name: "LinkedIn URL" });
    await userEvent.clear(input);
    await userEvent.type(
      input,
      "https://linkedin.com/company/brandt-gmbh{Enter}",
    );
    await waitFor(() =>
      expect(patchBody).toMatchObject({
        linkedin_url: "https://linkedin.com/company/brandt-gmbh",
      }),
    );
  });

  it("keeps the parent company a link to its record, and edits it from the bubble", async () => {
    const user = userEvent.setup();
    stub({
      "/me": writableSeat,
      "/companies": () =>
        jsonResponse({
          data: [{ ...company, id: "o-9", display_name: "Brandt Holding" }],
          page: emptyPage,
        }),
    });
    render(
      <CompanyProfileDetails
        company={{ ...company, parent_company_id: "o-9" }}
        edit={EDITABLE}
        {...NO_FIELDS}
      />,
    );

    const parent = await screen.findByRole("link", { name: "Brandt Holding" });
    const bubble = screen.getByRole("button", {
      name: `Change ${en["history.field.parent_company_id"]}`,
    });
    expect(bubble.closest(".editable-value")).toContainElement(parent);
    await user.click(bubble);
    expect(
      await screen.findByRole("dialog", {
        name: en["history.field.parent_company_id"],
      }),
    ).toBeVisible();
  });

  it("saves the company's general address as its email", async () => {
    let patchBody: unknown;
    stub({
      "/me": writableSeat,
      "/companies/o-1": async (request) => {
        if (request.method === "PATCH") {
          patchBody = await request.json();
          return jsonResponse({ ...company, version: 2 });
        }
        return jsonResponse(company);
      },
    });
    render(
      <CompanyProfileDetails
        company={{ ...company, email: null }}
        edit={EDITABLE}
        {...NO_FIELDS}
      />,
    );

    await userEvent.click(
      await screen.findByRole("button", {
        name: new RegExp(`^Change ${en["create.email"]}`),
      }),
    );
    await userEvent.type(
      screen.getByRole("textbox", { name: en["create.email"] }),
      "info@brandt.example{Enter}",
    );
    await waitFor(() =>
      expect(patchBody).toMatchObject({ email: "info@brandt.example" }),
    );
  });

  // No register answers for a postal address, so the VAT verdict mark belongs
  // to the VAT row alone.
  it("reads back a registered address the crawl found, with no verdict on it", async () => {
    stub();
    render(
      <CompanyProfileDetails
        company={company}
        edit={EDITABLE}
        fieldsLoaded
        fields={[
          {
            id: "pf-registered_address",
            field: "registered_address",
            value: "Kaiserdamm 1, 14057 Berlin",
            source: "site_read",
            captured_by: "agent:deepread",
            updated_at: "2026-06-01T08:00:00Z",
            version: 4,
          },
        ]}
      />,
    );

    const value = await screen.findByText("Kaiserdamm 1, 14057 Berlin");
    expect(value).toBeVisible();
    expect(
      value.closest("span")?.querySelector('[class*="vatmark"]'),
    ).toBeNull();
  });
});
