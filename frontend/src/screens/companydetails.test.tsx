// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { CompanyDetails } from "./companydetails";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

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
  status: "customer",
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
  installFetchStub({
    "GET /me": meRoute({ company: ["update"] }),
    "GET /records/company/co-1/tags": () =>
      jsonResponse({ data: [], withheld: false }),
  });
  render(
    <StoryProviders>
      <CompanyDetails company={company} />
    </StoryProviders>,
  );
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
