// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { CompanyDetails } from "./companydetails";

// CompanyDetails routes its address group through the same `groups` config
// and the same `postalLines` reader as the contact's own Details card
// (recordfieldvalues.ts). companyrail.test.tsx's own coverage of this row
// matches on NORMALIZED text, which collapses a real newline to a single
// space and would pass just as well if the parts were joined by a space
// instead — so it cannot tell postalLines' actual line breaks from a
// flattened summary. This reads the value cell's raw textContent, where the
// two are different strings.

type Company = components["schemas"]["Company"];

const COMPANY: Company = {
  id: "co-1",
  workspace_id: "w-1",
  display_name: "Brandt Automotive GmbH",
  writable: true,
  lifecycle: "customer",
  owner_id: null,
  address: {
    line1: "Leopoldstrasse 154",
    postal_code: "80804",
    city: "Munich",
    country: "DE",
  },
  domains: [],
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

function mount(company: Company) {
  const calls = { me: 0, patch: 0 };
  const session = meRoute({ company: ["update"] });
  installFetchStub({
    "GET /me": () => {
      calls.me += 1;
      return session();
    },
    "GET /records/company/co-1/tags": () =>
      jsonResponse({ data: [], withheld: false }),
    "GET /companies/co-1": () => jsonResponse(company),
    "PATCH /companies/co-1": () => {
      calls.patch += 1;
      return jsonResponse({ ...company, version: (company.version ?? 0) + 1 });
    },
  });
  render(
    <StoryProviders>
      <CompanyDetails company={company} />
    </StoryProviders>,
  );
  return calls;
}

afterEach(() => {
  cleanup();
});

describe("CompanyDetails' address row", () => {
  it("keeps the street on its own line above the postal code, ahead of country", async () => {
    mount(COMPANY);
    const value = (await screen.findByText("Address")).nextElementSibling;
    expect(value?.textContent).toBe("Leopoldstrasse 154\n80804 Munich\nDE");
  });
});

describe("CompanyDetails' rename", () => {
  it.each([
    { is_anchor: true, refetched: true },
    { is_anchor: false, refetched: false },
  ])(
    "reads /me again after a save only when is_anchor is $is_anchor",
    async ({ is_anchor, refetched }) => {
      const user = userEvent.setup();
      const calls = mount({ ...COMPANY, is_anchor });
      await user.click(
        await screen.findByRole("button", { name: "Change Company name" }),
      );
      const before = calls.me;
      await user.type(
        screen.getByRole("textbox", { name: "Company name" }),
        "!{Enter}",
      );

      await waitFor(() =>
        expect(
          screen.queryByRole("textbox", { name: "Company name" }),
        ).toBeNull(),
      );
      expect(calls.patch).toBe(1);
      expect(calls.me > before).toBe(refetched);
    },
  );
});
