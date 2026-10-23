/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { CompanyProfileDetails } from "./companyprofiledetails";
import { company, jsonResponse, render, stub } from "./companyrail.testkit";

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
  it("draws the six fields the details column leaves out, set or not", async () => {
    stub();
    render(
      <CompanyProfileDetails
        company={{ ...company, linkedin_url: null, legal_name: null }}
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
      en["co.profileField.registered_address"],
      en["tags.panelTitle"],
    ]) {
      expect(panel && within(panel).getByText(label)).toBeVisible();
    }
  });

  it("opens and saves the LinkedIn URL inline even when one is already set", async () => {
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

    await userEvent.click(
      await screen.findByRole("button", { name: "Change LinkedIn URL" }),
    );
    const input = screen.getByLabelText("LinkedIn URL");
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
