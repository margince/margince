/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { openChoiceDialog } from "../../testing/choicedialog.testkit";
import { meFixture } from "../../testing/mefixture";
import {
  company,
  emptyPage,
  jsonResponse,
  openSection,
  renderCards,
  renderRail,
  stub,
  view,
} from "./companyrail.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The owner is its own flow: Ownership's Edit opens the owner's dialog directly.
async function changeOwner(name: string) {
  const user = userEvent.setup();
  const { dialog, chooser } = await openChoiceDialog(
    user,
    "Owner",
    await screen.findByRole("button", { name: "Edit Ownership" }),
  );
  await pickOption(user, chooser, name);
  await user.click(within(dialog).getByRole("button", { name: "Save" }));
}

describe("the details' Edit links", () => {
  it("offer each section's Edit to a reader who may change it, and no pencil on a row", async () => {
    stub();
    renderRail();
    for (const section of ["About", "Ownership", "Registration"]) {
      expect(
        await screen.findByRole("button", { name: `Edit ${section}` }),
      ).toHaveTextContent(/^Edit$/);
    }
    expect(screen.queryByRole("button", { name: /^Change / })).toBeNull();
  });

  it("offer none to a reader without the grant to change accounts", async () => {
    stub({
      "/me": () => jsonResponse(meFixture({ allow: { company: ["read"] } })),
    });
    renderRail();
    expect(await screen.findByText("Mira Voss")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Edit / })).toBeNull();
  });

  it("open About with only About's fields", async () => {
    stub();
    renderRail();
    const about = await openSection(userEvent.setup(), "About");
    for (const name of [/^Industry/, /^Employees/, /^Lifecycle/]) {
      expect(within(about).getByRole("combobox", { name })).toBeVisible();
    }
    expect(within(about).getByLabelText("City")).toBeVisible();
    expect(
      within(about).queryByRole("combobox", { name: /^Owner/ }),
    ).toBeNull();
    expect(within(about).queryByLabelText(/Legal name/)).toBeNull();
  });

  it("save an About change through the account's conditional PATCH", async () => {
    let patch: { body: unknown; ifMatch: string | null } | undefined;
    stub({
      "/companies/o-1": async (request) => {
        if (request.method === "PATCH") {
          patch = {
            body: await request.json(),
            ifMatch: request.headers.get("If-Match"),
          };
          return jsonResponse({ ...company, version: 2 });
        }
        return jsonResponse(company);
      },
    });
    renderRail();
    const user = userEvent.setup();
    const about = await openSection(user, "About");
    await pickOption(
      user,
      within(about).getByRole("combobox", { name: /^Industry/ }),
      "Manufacturing",
    );
    await user.click(within(about).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(patch).toEqual({
        body: { industry: "manufacturing" },
        ifMatch: "1",
      }),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });
});

describe("CompanyRail", () => {
  it("draws the About rows from the fields the record actually carries", async () => {
    stub();
    renderRail();
    expect(screen.getByText("Automotive")).toBeInTheDocument();
    expect(screen.getByText("51–200 employees")).toBeInTheDocument();
    // One line per part; the query folds the newline into a space.
    expect(screen.getByText("Munich DE")).toBeInTheDocument();
    expect(screen.getByText("brandt.example")).toBeInTheDocument();
    // The owner resolves through the roster read: not shown until it lands.
    await waitFor(() =>
      expect(screen.getByText("Mira Voss")).toBeInTheDocument(),
    );
  });

  it("folds every empty field behind one link, and opens them in place", async () => {
    stub();
    const bare = {
      ...company,
      industry: null,
      size_band: null,
      address: undefined,
      domains: [],
    };
    renderRail({ view: view({ company: bare }) });
    expect(screen.queryByText("Industry")).toBeNull();
    expect(screen.queryByText("Employees")).toBeNull();

    await userEvent.click(
      screen.getByRole("button", { name: /^Show \d+ empty fields$/ }),
    );
    expect(screen.getByText("Industry")).toBeVisible();
    expect(screen.getByText("Employees")).toBeVisible();
    expect(screen.getByText("Address")).toBeVisible();
    expect(
      screen.getByRole("button", { name: "Hide empty fields" }),
    ).toBeInTheDocument();
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
    await changeOwner("Ravi Shah");
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
    await changeOwner("Ravi Shah");
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toContain(
        "The referenced record was not found.",
      ),
    );
    // The dialog stays open on the reader's choice rather than snapping back
    // to the old owner, the same rule InlineChoice keeps for every field.
    expect(screen.getByRole("combobox", { name: "Owner" })).toHaveTextContent(
      "Ravi Shah",
    );
  });

  it("edits lifecycle in the About dialog", async () => {
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
    // The fixture is already "customer", so pick a different value, or the
    // no-op guard skips the write.
    const user = userEvent.setup();
    const about = await openSection(user, "About");
    await pickOption(
      user,
      within(about).getByRole("combobox", { name: /^Lifecycle/ }),
      "Prospect",
    );
    await user.click(within(about).getByRole("button", { name: "Save" }));
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
    const user = userEvent.setup();
    const about = await openSection(user, "About");
    const input = within(about).getByDisplayValue("brandt.example");
    await user.clear(input);
    await user.type(input, "brandt-gmbh.example");
    await user.click(within(about).getByRole("button", { name: "Save" }));
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
    const user = userEvent.setup();
    const about = await openSection(user, "About");
    await user.click(
      within(about).getByRole("button", { name: /^Remove row/ }),
    );
    expect(onSave).not.toHaveBeenCalled();
    await user.click(within(about).getByRole("button", { name: "Save" }));
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
    const user = userEvent.setup();
    const about = await openSection(user, "About");
    const input = within(about).getByLabelText("City");
    await user.clear(input);
    await user.type(input, "Stuttgart");
    await user.click(within(about).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(patchBody).toMatchObject({
        // `country` survives from the existing address though this edit never
        // touched it. `address` replaces the whole object, so omitting it blanks it.
        address: { city: "Stuttgart", country: "DE" },
      }),
    );
  });

  it("marks a withheld section restricted instead of drawing it empty", () => {
    stub();
    renderCards({ view: view({ sections_omitted: ["contacts"] }) });
    const contacts = screen
      .getByRole("heading", { name: "Contacts" })
      .closest<HTMLElement>("section");
    expect(contacts).not.toBeNull();
    expect(
      contacts && within(contacts).getByText("Hidden for your role"),
    ).toBeTruthy();
  });
});

describe("the Address row", () => {
  it("writes the street and postal code, not the city alone", () => {
    stub();
    const aachen = {
      ...company,
      address: {
        line1: "Kaiser-Friedrich-Allee 51",
        postal_code: "52074",
        city: "Aachen",
      },
    };
    renderRail({ view: view({ company: aachen }) });
    expect(
      screen.getByText("Kaiser-Friedrich-Allee 51 52074 Aachen"),
    ).toBeInTheDocument();
  });
});

describe("a stored country that does not fit the postal code and city", () => {
  it("flags the address with the country they resolve to and keeps the stored one", () => {
    stub();
    const misfiled = {
      ...company,
      address: { postal_code: "10115", city: "Berlin", country: "AT" },
      address_country_mismatch: { resolved_country: "DE" },
    };
    renderRail({ view: view({ company: misfiled }) });
    expect(screen.getByText("10115 Berlin AT")).toBeInTheDocument();
    expect(
      screen.getByText("Postal code and city are in Germany"),
    ).toBeInTheDocument();
  });

  it("shows no flag when the server reports no mismatch", () => {
    stub();
    renderRail();
    expect(screen.getByText("Munich DE")).toBeInTheDocument();
    expect(screen.queryByText(/Postal code and city are in/)).toBeNull();
  });
});
