/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  company,
  emptyPage,
  jsonResponse,
  renderRail,
  stub,
  view,
} from "./companyrail.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("CompanyRail", () => {
  it("draws the details grid from the fields the record actually carries", async () => {
    stub();
    renderRail();
    expect(
      screen.getAllByText("Brandt Automotive GmbH")[0],
    ).toBeInTheDocument();
    expect(screen.getByText("Automotive")).toBeInTheDocument();
    expect(screen.getByText("51-200")).toBeInTheDocument();
    // Address draws as postal lines, city then country on their own line
    // (recordfieldvalues.ts's `postalLines`), rather than one combined
    // "Munich, DE" summary. The default text normalizer collapses that line
    // break to a single space, so the two parts still read as one match.
    expect(screen.getByText("Munich DE")).toBeInTheDocument();
    expect(screen.getByText("brandt.example")).toBeInTheDocument();
    // The owner cell resolves through the roster read, same as EntityRef
    // does everywhere else: not shown until the read lands.
    await waitFor(() =>
      expect(screen.getByText("Mira Voss")).toBeInTheDocument(),
    );
  });

  it("still draws every known row when the record carries no value for it", async () => {
    const user = userEvent.setup();
    stub();
    const bare = {
      ...company,
      legal_name: null,
      industry: null,
      size_band: null,
      linkedin_url: null,
      address: undefined,
      domains: [],
      owner_id: null,
    };
    renderRail({ view: view({ company: bare }) });
    // The unset rows wait behind one link, and every one of them comes back
    // when the reader opens it. Industry (InlineText) and Company
    // size (InlineChoice with `hideLabel`) draw no visible label of their own.
    // Both get it from FieldRow's label column only, so it is checked directly.
    // Address is six rows, one per part, and City stands in for the other five.
    expect(screen.queryByText("Industry")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /empty field/ }));
    expect(screen.getByText("Industry")).toBeInTheDocument();
    expect(screen.getByText("Company size")).toBeInTheDocument();
    expect(screen.getByText("Address")).toBeInTheDocument();
    // No /me grant in this stub, so every field renders its read-only
    // fallback, not a control. Owner and every address part share the grid's
    // "Not set"/"Unassigned" absence text.
    expect(screen.getAllByText("Not set").length).toBeGreaterThan(0);
  });

  it("opens and saves the LinkedIn URL inline even when one is already set", async () => {
    let patchBody: unknown;
    stub({
      "/me": () =>
        jsonResponse({
          user: { id: "u-1", display_name: "Mira Voss" },
          authorization: {
            objects: { company: { update: true } },
            // A full seat: the licensing ceiling is checked before RBAC, and the
            // grid's controls issue a PATCH.
            seat_type: "full",
          },
        }),
      "/companies/o-1": async (request) => {
        if (request.method === "PATCH") {
          patchBody = await request.json();
          return jsonResponse({ ...company, version: 2 });
        }
        return jsonResponse(company);
      },
    });
    renderRail();
    // The value is already set. In that case the row must still offer a
    // control, not a bare link with nothing to open.
    // Waits for /me's grant to resolve first: the button exists only once
    // `canEdit` turns true, same as every other inline control here.
    await userEvent.click(
      await screen.findByRole("button", { name: "Change LinkedIn URL" }),
    );
    const input = screen.getByLabelText("LinkedIn URL");
    await userEvent.clear(input);
    // No Save button when editing in place: Enter is the commit.
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

  it("edits owner through the same roster-backed control the header uses", async () => {
    let patchBody: unknown;
    stub({
      "/me": () =>
        jsonResponse({
          user: { id: "u-1", display_name: "Mira Voss" },
          authorization: {
            objects: { company: { update: true } },
            // A full seat: the licensing ceiling is checked before RBAC, and the
            // grid's controls issue a PATCH.
            seat_type: "full",
          },
        }),
      "/users": () =>
        jsonResponse({
          data: [
            { id: "u-1", display_name: "Mira Voss" },
            { id: "u-2", display_name: "Ravi Shah" },
          ],
          page: emptyPage,
        }),
      "/companies/o-1": async (request) => {
        if (request.method === "PATCH") {
          patchBody = await request.json();
          return jsonResponse({ ...company, version: 2 });
        }
        return jsonResponse(company);
      },
    });
    renderRail();
    await waitFor(() =>
      expect(screen.getByText("Mira Voss")).toBeInTheDocument(),
    );
    // The click that starts editing already opens the picker, with no second
    // click on the combobox first.
    await userEvent.click(screen.getByRole("button", { name: "Change Owner" }));
    await userEvent.click(screen.getByRole("option", { name: "Ravi Shah" }));
    await waitFor(() => expect(patchBody).toMatchObject({ owner_id: "u-2" }));
  });

  it("surfaces the server's refusal on the owner control rather than swallowing it", async () => {
    // A stale roster entry (a user removed between load and save) is how an
    // accepted-looking choice still fails. The FK on company.owner_id rejects it.
    // The owner control must show that sentence next to itself, through the
    // refusal path design-system/inlinechoice.test.tsx proves for InlineChoice.
    stub({
      "/me": () =>
        jsonResponse({
          user: { id: "u-1", display_name: "Mira Voss" },
          authorization: {
            objects: { company: { update: true } },
            // A full seat: the licensing ceiling is checked before RBAC, and the
            // grid's controls issue a PATCH.
            seat_type: "full",
          },
        }),
      "/users": () =>
        jsonResponse({
          data: [
            { id: "u-1", display_name: "Mira Voss" },
            { id: "u-2", display_name: "Ravi Shah" },
          ],
          page: emptyPage,
        }),
      "/companies/o-1": (request) => {
        if (request.method === "PATCH") {
          return jsonResponse(
            {
              type: "about:blank",
              title: "Unprocessable Entity",
              status: 422,
              code: "reference_not_found",
              detail: "The referenced record was not found.",
            },
            422,
          );
        }
        return jsonResponse(company);
      },
    });
    renderRail();
    await waitFor(() =>
      expect(screen.getByText("Mira Voss")).toBeInTheDocument(),
    );
    await userEvent.click(screen.getByRole("button", { name: "Change Owner" }));
    await userEvent.click(screen.getByRole("option", { name: "Ravi Shah" }));
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toContain(
        "The referenced record was not found.",
      ),
    );
    // The picker stays open on the reader's choice rather than snapping back
    // to the old owner, the same rule InlineChoice keeps for every field.
    expect(screen.getByRole("combobox")).toBeInTheDocument();
  });

  it("edits lifecycle in the grid through the header's own control", async () => {
    // Only the rail renders in this suite. The header's copy of this control,
    // and the claim of one implementation in two mount points, belong to
    // company360.test.tsx, where both mount together.
    let patchBody: unknown;
    stub({
      "/me": () =>
        jsonResponse({
          user: { id: "u-1", display_name: "Mira Voss" },
          authorization: {
            objects: { company: { update: true } },
            // A full seat: the licensing ceiling is checked before RBAC, and the
            // grid's controls issue a PATCH.
            seat_type: "full",
          },
        }),
      "/companies/o-1": async (request) => {
        if (request.method === "PATCH") {
          patchBody = await request.json();
          return jsonResponse({ ...company, version: 2 });
        }
        return jsonResponse(company);
      },
    });
    renderRail();
    await userEvent.click(
      await screen.findByRole("button", { name: "Change Lifecycle" }),
    );
    // The fixture is already "customer", so pick a different value, or the
    // no-op guard skips the write.
    await userEvent.click(screen.getByRole("option", { name: "Prospect" }));
    await waitFor(() =>
      expect(patchBody).toMatchObject({ lifecycle: "prospect" }),
    );
  });

  it("renames the primary domain while preserving every other domain on the account", async () => {
    let patchBody: unknown;
    const threeDomains = {
      ...company,
      domains: [
        { domain: "brandt.example", is_primary: true, source: "manual" },
        { domain: "brandt.de", is_primary: false, source: "manual" },
        {
          domain: "brandt-automotive.com",
          is_primary: false,
          source: "manual",
        },
      ],
    };
    stub({
      "/me": () =>
        jsonResponse({
          user: { id: "u-1", display_name: "Mira Voss" },
          authorization: {
            objects: { company: { update: true } },
            // A full seat: the licensing ceiling is checked before RBAC, and the
            // grid's controls issue a PATCH.
            seat_type: "full",
          },
        }),
      "/companies/o-1": async (request) => {
        if (request.method === "PATCH") {
          patchBody = await request.json();
          return jsonResponse({ ...threeDomains, version: 2 });
        }
        return jsonResponse(threeDomains);
      },
    });
    renderRail({ view: view({ company: threeDomains }) });
    await userEvent.click(
      await screen.findByRole("button", { name: "Change Domains" }),
    );
    const input = screen.getByDisplayValue("brandt.example");
    await userEvent.clear(input);
    await userEvent.type(input, "brandt-gmbh.example");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(patchBody).toBeTruthy());
    // The other two domains survive, and only the renamed primary changed.
    // Sending only the edited entry would drop them: the replace-set trap.
    expect(patchBody).toMatchObject({
      domains: expect.arrayContaining([
        expect.objectContaining({ domain: "brandt.de", is_primary: false }),
        expect.objectContaining({
          domain: "brandt-automotive.com",
          is_primary: false,
        }),
        expect.objectContaining({
          domain: "brandt-gmbh.example",
          is_primary: true,
        }),
      ]),
    });
    expect((patchBody as { domains: unknown[] }).domains).toHaveLength(3);
  });

  it("removes the final domain only through the grouped replace-set save", async () => {
    const onSave = vi.fn();
    stub({
      "/me": () =>
        jsonResponse({
          user: { id: "u-1", display_name: "Mira Voss" },
          authorization: {
            objects: { company: { update: true } },
            // A full seat: the licensing ceiling is checked before RBAC, and the
            // grid's controls issue a PATCH.
            seat_type: "full",
          },
        }),
      "/companies/o-1": async (request) => {
        if (request.method === "PATCH") {
          onSave(await request.json());
        }
        return jsonResponse(company);
      },
    });
    renderRail();
    await userEvent.click(
      await screen.findByRole("button", { name: "Change Domains" }),
    );
    await userEvent.click(screen.getByRole("button", { name: /^Remove row/ }));
    expect(onSave).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(onSave).toHaveBeenCalledWith({ domains: [] }));
  });

  it("edits an address part by sending the whole address back with only that part changed", async () => {
    let patchBody: unknown;
    stub({
      "/me": () =>
        jsonResponse({
          user: { id: "u-1", display_name: "Mira Voss" },
          authorization: {
            objects: { company: { update: true } },
            // A full seat: the licensing ceiling is checked before RBAC, and the
            // grid's controls issue a PATCH.
            seat_type: "full",
          },
        }),
      "/companies/o-1": async (request) => {
        if (request.method === "PATCH") {
          patchBody = await request.json();
          return jsonResponse({ ...company, version: 2 });
        }
        return jsonResponse(company);
      },
    });
    renderRail();
    await userEvent.click(
      await screen.findByRole("button", { name: "Change Address" }),
    );
    const input = screen.getByLabelText("City");
    await userEvent.clear(input);
    await userEvent.type(input, "Stuttgart");
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(patchBody).toMatchObject({
        // `country` survives from the existing address though this edit never
        // touched it. `address` replaces the whole object, so omitting it blanks it.
        address: { city: "Stuttgart", country: "DE" },
      }),
    );
  });
});
