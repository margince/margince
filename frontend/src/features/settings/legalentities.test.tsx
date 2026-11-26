/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { en } from "../../i18n/en";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { jsonResponse, render } from "../../testing/settings.testkit";
import { LegalEntitiesCard } from "./legalentities";

// Settings › Company › Legal entities: every seat reads the list, a seat
// holding legal_entity:create is offered a new one, :update edits one and
// :delete archives one.

const ADMIN: GrantSpec = {
  legal_entity: ["read", "create", "update", "delete"],
};
const READER: GrantSpec = { legal_entity: ["read"] };

function entity(id: string, extra: object = {}) {
  return {
    id,
    is_default: false,
    display_name: "",
    address: {},
    member_count: 0,
    version: 1,
    created_at: "2026-10-01T08:00:00Z",
    updated_at: "2026-10-01T08:00:00Z",
    ...extra,
  };
}

const DEFAULT_ONLY = [
  entity("le-default", { is_default: true, display_name: "Acme" }),
];

const VIENNA = entity("le-vienna", {
  display_name: "Acme Wien",
  legal_name: "Acme Wien GmbH",
  country: "at",
  jurisdiction: "at",
  version: 4,
  address: { city: "Wien" },
});

type Call = {
  method: string;
  url: string;
  body: unknown;
  ifMatch: string | null;
};

// `refuse` answers a write to a path ending in its key with that problem.
// A logo write waits for `logoAnswer`, and like the server it refuses one
// whose If-Match is not the stored version.
function backend(
  allow: GrantSpec,
  rows: object[],
  calls: Call[] = [],
  refuse: Record<string, object> = {},
  logoAnswer: Promise<void> = Promise.resolve(),
) {
  let logoVersion = 4;
  // The typed client hands fetch a Request; a file upload, a path and init.
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request =
      input instanceof Request ? input : new Request(String(input), init);
    const raw =
      request.method === "GET" || request.url.endsWith("/logo")
        ? ""
        : await request.text();
    calls.push({
      method: request.method,
      url: request.url,
      body: raw ? JSON.parse(raw) : undefined,
      ifMatch: request.headers.get("If-Match"),
    });
    if (request.url.endsWith("/me")) {
      return jsonResponse(meFixture({ allow }));
    }
    if (request.url.endsWith("/installation/jurisdiction")) {
      return jsonResponse({
        selected: "de",
        available: [{ code: "at" }, { code: "de" }],
        deadlines: [],
        documents: [],
      });
    }
    const refusal = Object.entries(refuse).find(([path]) =>
      request.url.endsWith(path),
    );
    if (refusal && request.method !== "GET") {
      const answer = refusal[1];
      const status =
        "status" in answer && typeof answer.status === "number"
          ? answer.status
          : 409;
      return jsonResponse(answer, status);
    }
    if (request.method === "POST" && request.url.endsWith("/legal-entities")) {
      return jsonResponse(entity("le-new", JSON.parse(raw)), 201);
    }
    if (request.url.endsWith("/logo")) {
      await logoAnswer;
      if (request.headers.get("If-Match") !== String(logoVersion)) {
        return jsonResponse(
          { title: "Conflict", status: 409, code: "version_skew" },
          409,
        );
      }
      logoVersion += 1;
      return jsonResponse(
        entity("le-vienna", { version: logoVersion, logo_url: "/v1/logo.png" }),
      );
    }
    if (request.method === "PATCH" || request.method === "POST") {
      return jsonResponse(entity("le-saved"));
    }
    return jsonResponse({ data: rows });
  });
}

async function rowVerb(
  user: ReturnType<typeof userEvent.setup>,
  name: string,
  verb: string,
) {
  await user.click(
    await screen.findByRole("button", { name: `Actions for ${name}` }),
  );
  await user.click(await screen.findByRole("button", { name: verb }));
}

beforeEach(() => {
  localStorage.setItem("margince.workspaceSlug", "acme");
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
});

it("lists the default entity alone, named after the company and marked default", async () => {
  vi.stubGlobal("fetch", backend(READER, DEFAULT_ONLY));
  render(<LegalEntitiesCard />);
  const row = await screen.findByTestId("legal-entity-le-default");
  expect(within(row).getByText("Acme")).toBeInTheDocument();
  expect(
    within(row).getByText(en["legalEntities.defaultBadge"]),
  ).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: en["legalEntities.addOpen"] }),
  ).toBeNull();
});

it("shows an admin a further entity that names no jurisdiction, and not the default one", async () => {
  vi.stubGlobal(
    "fetch",
    backend(READER, [
      ...DEFAULT_ONLY,
      VIENNA,
      entity("le-branch", { display_name: "Acme Branch" }),
    ]),
  );
  render(<LegalEntitiesCard />);
  const gap = en["legalEntities.jurisdictionMissing"];
  const branch = await screen.findByTestId("legal-entity-le-branch");
  expect(within(branch).getByText(gap)).toBeInTheDocument();
  expect(
    within(screen.getByTestId("legal-entity-le-default")).queryByText(gap),
  ).toBeNull();
  const vienna = screen.getByTestId("legal-entity-le-vienna");
  expect(within(vienna).queryByText(gap)).toBeNull();
});

it("flags a jurisdiction the build offers no pack for, as a copied address country can be", async () => {
  vi.stubGlobal(
    "fetch",
    backend(READER, [
      ...DEFAULT_ONLY,
      entity("le-paris", { display_name: "Acme Paris", jurisdiction: "fr" }),
    ]),
  );
  render(<LegalEntitiesCard />);
  const paris = await screen.findByTestId("legal-entity-le-paris");
  expect(
    await within(paris).findByText(en["legalEntities.jurisdictionMissing"]),
  ).toBeInTheDocument();
});

it("lists each entity's jurisdiction beside its country, the default's being the installation's", async () => {
  vi.stubGlobal(
    "fetch",
    backend(READER, [
      ...DEFAULT_ONLY,
      { ...VIENNA, country: "fr", jurisdiction: "at" },
    ]),
  );
  render(<LegalEntitiesCard />);
  expect(
    await screen.findByRole("columnheader", {
      name: en["legalEntities.colJurisdiction"],
    }),
  ).toBeInTheDocument();
  const vienna = await screen.findByTestId("legal-entity-le-vienna");
  expect(within(vienna).getByText("France")).toBeInTheDocument();
  expect(within(vienna).getByText("Austria")).toBeInTheDocument();
  expect(within(vienna).queryByText("FR")).toBeNull();
  expect(
    await within(screen.getByTestId("legal-entity-le-default")).findByText(
      "Germany",
    ),
  ).toBeInTheDocument();
});

it("names an entity's country, not its code", async () => {
  vi.stubGlobal(
    "fetch",
    backend(READER, [
      ...DEFAULT_ONLY,
      entity("le-saigon", { display_name: "Saigon Branch", country: "vn" }),
    ]),
  );
  render(<LegalEntitiesCard />);
  const saigon = await screen.findByTestId("legal-entity-le-saigon");
  expect(within(saigon).getByText("Vietnam")).toBeInTheDocument();
  expect(within(saigon).queryByText("VN")).toBeNull();
});

it("says where the default's legal name and country are set while it has none", async () => {
  vi.stubGlobal("fetch", backend(READER, [...DEFAULT_ONLY, VIENNA]));
  render(<LegalEntitiesCard />);
  const source = en["legalEntities.fromCompanyProfile"];
  const row = await screen.findByTestId("legal-entity-le-default");
  expect(within(row).getAllByText(source)).toHaveLength(2);
  // A further entity keeps its own values, and the note is not on its row.
  const vienna = screen.getByTestId("legal-entity-le-vienna");
  expect(within(vienna).queryByText(source)).toBeNull();
  // The sentence above the list points to the same place as the cells.
  expect(screen.getByText(en["legalEntities.sub"])).toHaveTextContent(
    /from the company profile\.$/,
  );
});

it("shows the default's legal name once the company profile holds one", async () => {
  vi.stubGlobal(
    "fetch",
    backend(READER, [
      entity("le-default", {
        is_default: true,
        display_name: "Acme",
        legal_name: "Acme GmbH",
      }),
    ]),
  );
  render(<LegalEntitiesCard />);
  const row = await screen.findByTestId("legal-entity-le-default");
  expect(within(row).getByText("Acme GmbH")).toBeInTheDocument();
  expect(
    within(row).getAllByText(en["legalEntities.fromCompanyProfile"]),
  ).toHaveLength(1);
});

it("names the default entity of an undescribed company in words", async () => {
  vi.stubGlobal(
    "fetch",
    backend(READER, [entity("le-default", { is_default: true })]),
  );
  render(<LegalEntitiesCard />);
  const row = await screen.findByTestId("legal-entity-le-default");
  expect(
    within(row).getByText(en["legalEntities.defaultUnnamed"]),
  ).toBeInTheDocument();
});

it("describes a further entity with its name, legal name, country and jurisdiction", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal("fetch", backend(ADMIN, DEFAULT_ONLY, calls));
  render(<LegalEntitiesCard />);
  await user.click(
    await screen.findByRole("button", { name: en["legalEntities.addOpen"] }),
  );
  const dialog = await screen.findByRole("dialog");
  const [name, legal] = within(dialog).getAllByRole("textbox");
  await user.type(name as HTMLElement, "Wien");
  await user.type(legal as HTMLElement, "Wien GmbH");
  await pickOption(
    user,
    within(dialog).getByRole("combobox", { name: /Country/ }),
    "Austria",
  );
  await pickOption(
    user,
    within(dialog).getByRole("combobox", { name: /Jurisdiction/ }),
    "Germany",
  );
  await user.click(
    within(dialog).getByRole("button", {
      name: en["legalEntities.addConfirm"],
    }),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(
    calls.some(
      (call) =>
        call.method === "POST" &&
        call.url.endsWith("/legal-entities") &&
        JSON.stringify(call.body) ===
          JSON.stringify({
            display_name: "Wien",
            legal_name: "Wien GmbH",
            country: "at",
            jurisdiction: "de",
          }),
    ),
  ).toBe(true);
});

it("asks before Escape drops a typed entity, and closes an empty one at once", async () => {
  const user = userEvent.setup();
  vi.stubGlobal("fetch", backend(ADMIN, DEFAULT_ONLY));
  render(<LegalEntitiesCard />);
  const add = await screen.findByRole("button", {
    name: en["legalEntities.addOpen"],
  });
  await user.click(add);
  await user.keyboard("{Escape}");
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());

  await user.click(add);
  const [name] = within(await screen.findByRole("dialog")).getAllByRole(
    "textbox",
  );
  await user.type(name as HTMLElement, "Wien");
  await user.keyboard("{Escape}");
  expect(
    screen.getByRole("dialog", { name: en["unsaved.title"] }),
  ).toBeInTheDocument();
});

it("saves only the fields an edit changed, on the version it read", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal("fetch", backend(ADMIN, [...DEFAULT_ONLY, VIENNA], calls));
  render(<LegalEntitiesCard />);
  await rowVerb(user, "Acme Wien", en["legalEntities.edit"]);
  const dialog = await screen.findByRole("dialog");
  const city = within(dialog).getByRole("textbox", {
    name: en["legalEntities.city"],
  });
  await user.clear(city);
  await user.type(city, "Graz");
  await user.type(
    within(dialog).getByRole("textbox", { name: en["legalEntities.iban"] }),
    "AT61 1904 3002 3457 3201",
  );
  await user.type(
    within(dialog).getByRole("textbox", {
      name: en["legalEntities.signatureTemplate"],
    }),
    "<p>Acme Wien</p>",
  );
  await user.click(
    within(dialog).getByRole("button", { name: en["legalEntities.save"] }),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  const patch = calls.find((call) => call.method === "PATCH");
  expect(patch?.url).toMatch(/\/legal-entities\/le-vienna$/);
  expect(patch?.ifMatch).toBe("4");
  expect(patch?.body).toEqual({
    iban: "AT61 1904 3002 3457 3201",
    signature_template: "<p>Acme Wien</p>",
    address: { city: "Graz" },
  });
});

it("sends a changed jurisdiction and nothing else", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal("fetch", backend(ADMIN, [...DEFAULT_ONLY, VIENNA], calls));
  render(<LegalEntitiesCard />);
  await rowVerb(user, "Acme Wien", en["legalEntities.edit"]);
  const dialog = await screen.findByRole("dialog");
  await pickOption(
    user,
    await within(dialog).findByRole("combobox", { name: /Jurisdiction/ }),
    "Germany",
  );
  await user.click(
    within(dialog).getByRole("button", { name: en["legalEntities.save"] }),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(calls.find((call) => call.method === "PATCH")?.body).toEqual({
    jurisdiction: "de",
  });
});

it("offers the default only the fields it keeps itself", async () => {
  const user = userEvent.setup();
  vi.stubGlobal("fetch", backend(ADMIN, DEFAULT_ONLY));
  render(<LegalEntitiesCard />);
  await rowVerb(user, "Acme", en["legalEntities.edit"]);
  const dialog = await screen.findByRole("dialog");
  expect(
    within(dialog).getByText(en["legalEntities.defaultFromCompany"]),
  ).toBeInTheDocument();
  expect(
    within(dialog).queryByRole("textbox", {
      name: en["legalEntities.displayName"],
    }),
  ).toBeNull();
  expect(
    within(dialog).getByRole("textbox", { name: en["legalEntities.iban"] }),
  ).toBeInTheDocument();
  // It signs with the workspace template, which has its own card.
  expect(
    within(dialog).queryByRole("textbox", {
      name: en["legalEntities.signatureTemplate"],
    }),
  ).toBeNull();
  // Its law is the installation's jurisdiction, shown and never edited here.
  expect(
    within(dialog).queryByRole("combobox", { name: /Country/ }),
  ).toBeNull();
  expect(
    within(dialog).queryByRole("combobox", { name: /Jurisdiction/ }),
  ).toBeNull();
  expect(
    await within(dialog).findByText(/The default company follows/),
  ).toBeInTheDocument();
});

it("never offers to archive the default entity", async () => {
  const user = userEvent.setup();
  vi.stubGlobal("fetch", backend(ADMIN, DEFAULT_ONLY));
  render(<LegalEntitiesCard />);
  await user.click(
    await screen.findByRole("button", { name: "Actions for Acme" }),
  );
  await screen.findByRole("button", { name: en["legalEntities.edit"] });
  expect(
    screen.queryByRole("button", { name: en["legalEntities.archive"] }),
  ).toBeNull();
});

it("says in words why an entity with members cannot be archived", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    backend(ADMIN, [...DEFAULT_ONLY, VIENNA], calls, {
      "/archive": {
        type: "about:blank",
        title: "Conflict",
        status: 409,
        code: "legal_entity_has_members",
        detail: "3 users still work for this legal entity",
      },
    }),
  );
  render(<LegalEntitiesCard />);
  await rowVerb(user, "Acme Wien", en["legalEntities.archive"]);
  const confirm = await screen.findByRole("dialog");
  await user.click(
    within(confirm).getByRole("button", { name: en["legalEntities.archive"] }),
  );
  expect(
    await within(confirm).findByText(en["legalEntities.hasMembers"]),
  ).toBeInTheDocument();
  expect(calls.find((call) => call.url.endsWith("/archive"))?.ifMatch).toBe(
    "4",
  );
});

it("offers a reader no verbs on a row", async () => {
  vi.stubGlobal("fetch", backend(READER, [...DEFAULT_ONLY, VIENNA]));
  render(<LegalEntitiesCard />);
  await screen.findByTestId("legal-entity-le-vienna");
  expect(
    screen.queryByRole("button", { name: "Actions for Acme Wien" }),
  ).toBeNull();
});

// The logo slot's drop target, named by the slot it sits in.
function logoPicker(dialog: HTMLElement) {
  const slot = within(dialog).getByRole("region", {
    name: en["legalEntities.sectionLogo"],
  });
  return within(slot).getByLabelText(en["legalEntities.sectionLogo"]);
}

it("stores a chosen logo at once, and saves the next edit on the version it left", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal("fetch", backend(ADMIN, [...DEFAULT_ONLY, VIENNA], calls));
  render(<LegalEntitiesCard />);
  await rowVerb(user, "Acme Wien", en["legalEntities.edit"]);
  const dialog = await screen.findByRole("dialog");
  await user.upload(
    logoPicker(dialog),
    new File(["png"], "logo.png", { type: "image/png" }),
  );
  await user.click(
    within(dialog).getByRole("button", { name: en["legalEntities.logoEdit"] }),
  );
  await screen.findByRole("button", { name: en["legalEntities.logoRemove"] });
  await user.keyboard("{Escape}");
  const upload = calls.find(
    (call) =>
      call.method === "POST" &&
      call.url.endsWith("/legal-entities/le-vienna/logo"),
  );
  expect(upload?.ifMatch).toBe("4");
  await user.type(
    within(dialog).getByRole("textbox", { name: en["legalEntities.bic"] }),
    "BKAUATWW",
  );
  await user.click(
    within(dialog).getByRole("button", { name: en["legalEntities.save"] }),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(calls.find((call) => call.method === "PATCH")?.ifMatch).toBe("5");
});

it("holds Save and the close while a logo write is out", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  let answer = () => {};
  const logoAnswer = new Promise<void>((resolve) => {
    answer = resolve;
  });
  vi.stubGlobal(
    "fetch",
    backend(ADMIN, [...DEFAULT_ONLY, VIENNA], calls, {}, logoAnswer),
  );
  render(<LegalEntitiesCard />);
  await rowVerb(user, "Acme Wien", en["legalEntities.edit"]);
  const dialog = await screen.findByRole("dialog");
  await user.type(
    within(dialog).getByRole("textbox", { name: en["legalEntities.bic"] }),
    "BKAUATWW",
  );
  await user.upload(
    logoPicker(dialog),
    new File(["png"], "logo.png", { type: "image/png" }),
  );
  const save = within(dialog).getByRole("button", {
    name: en["legalEntities.save"],
  });
  await waitFor(() => expect(save).toBeDisabled());
  expect(
    within(dialog).getByRole("button", { name: en["deals.cancel"] }),
  ).toBeDisabled();
  await user.keyboard("{Escape}");
  expect(screen.getByRole("dialog")).toBe(dialog);
  expect(calls.some((call) => call.method === "PATCH")).toBe(false);

  answer();
  await waitFor(() => expect(save).toBeEnabled());
  await user.click(save);
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(calls.find((call) => call.method === "PATCH")?.ifMatch).toBe("5");
});

function fieldFault(field: string, code: string, extra: object = {}) {
  return {
    title: "Validation failed",
    status: 422,
    code: "validation_error",
    detail: `${field} is wrong`,
    details: {
      errors: [{ field, code, message: `${field} is wrong`, ...extra }],
    },
  };
}

async function openNewEntity(user: ReturnType<typeof userEvent.setup>) {
  await user.click(
    await screen.findByRole("button", { name: en["legalEntities.addOpen"] }),
  );
  return screen.findByRole("dialog");
}

it("says on each blank required field what it needs, focuses the first and sends nothing", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal("fetch", backend(ADMIN, DEFAULT_ONLY, calls));
  render(<LegalEntitiesCard />);
  const dialog = await openNewEntity(user);
  const [name, legal] = within(dialog).getAllByRole("textbox");
  await user.type(name as HTMLElement, "   ");
  await user.type(legal as HTMLElement, "Wien GmbH");
  await user.click(
    within(dialog).getByRole("button", {
      name: en["legalEntities.addConfirm"],
    }),
  );
  expect(await within(dialog).findByText("Name is required.")).toBeVisible();
  expect(within(dialog).getByText("Country is required.")).toBeVisible();
  expect(within(dialog).getByText("Jurisdiction is required.")).toBeVisible();
  await waitFor(() => expect(name).toHaveFocus());
  expect(name).toHaveAttribute("aria-invalid", "true");
  expect(legal).not.toHaveAttribute("aria-invalid");
  expect(calls.some((call) => call.method === "POST")).toBe(false);

  await user.type(name as HTMLElement, "Wien");
  expect(within(dialog).queryByText("Name is required.")).toBeNull();
  expect(within(dialog).getByText("Country is required.")).toBeVisible();
});

it("puts a refusal the server names on its field, by the field's label", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    backend(ADMIN, DEFAULT_ONLY, [], {
      "/legal-entities": fieldFault("legal_name", "too_long", { max: 200 }),
    }),
  );
  render(<LegalEntitiesCard />);
  const dialog = await openNewEntity(user);
  const [name, legal] = within(dialog).getAllByRole("textbox");
  await user.type(name as HTMLElement, "Wien");
  await user.type(legal as HTMLElement, "Wien GmbH");
  await pickOption(
    user,
    within(dialog).getByRole("combobox", { name: /Country/ }),
    "Austria",
  );
  await pickOption(
    user,
    within(dialog).getByRole("combobox", { name: /Jurisdiction/ }),
    "Germany",
  );
  await user.click(
    within(dialog).getByRole("button", {
      name: en["legalEntities.addConfirm"],
    }),
  );
  expect(
    await within(dialog).findByText("Legal name: at most 200 characters."),
  ).toBeVisible();
  await waitFor(() => expect(legal).toHaveFocus());
  expect(within(dialog).queryByText("legal_name is wrong")).toBeNull();
});

it("says on the name field that another entity has the name", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    backend(ADMIN, DEFAULT_ONLY, [], {
      "/legal-entities": {
        title: "Conflict",
        status: 409,
        code: "legal_entity_name_taken",
        detail: "another live legal entity already has this name",
      },
    }),
  );
  render(<LegalEntitiesCard />);
  const dialog = await openNewEntity(user);
  const [name, legal] = within(dialog).getAllByRole("textbox");
  await user.type(name as HTMLElement, "Acme");
  await user.type(legal as HTMLElement, "Acme GmbH");
  await pickOption(
    user,
    within(dialog).getByRole("combobox", { name: /Country/ }),
    "Austria",
  );
  await pickOption(
    user,
    within(dialog).getByRole("combobox", { name: /Jurisdiction/ }),
    "Germany",
  );
  await user.click(
    within(dialog).getByRole("button", {
      name: en["legalEntities.addConfirm"],
    }),
  );
  expect(
    await within(dialog).findByText(en["legalEntities.nameTaken"]),
  ).toBeVisible();
  expect(name).toHaveAttribute("aria-invalid", "true");
  await waitFor(() => expect(name).toHaveFocus());
});

it("puts an edit's refused IBAN on the IBAN field", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    backend(ADMIN, [...DEFAULT_ONLY, VIENNA], [], {
      "/legal-entities/le-vienna": fieldFault("iban", "invalid_iban"),
    }),
  );
  render(<LegalEntitiesCard />);
  await rowVerb(user, "Acme Wien", en["legalEntities.edit"]);
  const dialog = await screen.findByRole("dialog");
  const iban = within(dialog).getByRole("textbox", {
    name: en["legalEntities.iban"],
  });
  await user.type(iban, "AT00");
  await user.click(
    within(dialog).getByRole("button", { name: en["legalEntities.save"] }),
  );
  expect(
    await within(dialog).findByText(en["legalEntities.invalidIban"]),
  ).toBeVisible();
  await waitFor(() => expect(iban).toHaveFocus());
});

it("closes an unchanged entity whose stored name is blank, and sends nothing", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal(
    "fetch",
    backend(
      ADMIN,
      [...DEFAULT_ONLY, entity("le-legacy", { legal_name: "Legacy GmbH" })],
      calls,
    ),
  );
  render(<LegalEntitiesCard />);
  await rowVerb(
    user,
    en["legalEntities.defaultUnnamed"],
    en["legalEntities.edit"],
  );
  const dialog = await screen.findByRole("dialog");
  await user.click(
    within(dialog).getByRole("button", { name: en["legalEntities.save"] }),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(calls.some((call) => call.method === "PATCH")).toBe(false);
});

it("puts a refusal the decoder names under offering on its field", async () => {
  const user = userEvent.setup();
  vi.stubGlobal(
    "fetch",
    backend(ADMIN, [...DEFAULT_ONLY, VIENNA], [], {
      "/legal-entities/le-vienna": fieldFault("offering.icp", "too_long", {
        max: 2000,
      }),
    }),
  );
  render(<LegalEntitiesCard />);
  await rowVerb(user, "Acme Wien", en["legalEntities.edit"]);
  const dialog = await screen.findByRole("dialog");
  const icp = within(dialog).getByRole("textbox", {
    name: en["legalEntities.icp"],
  });
  await user.type(icp, "Mid-size utilities");
  await user.click(
    within(dialog).getByRole("button", { name: en["legalEntities.save"] }),
  );
  expect(
    await within(dialog).findByText(
      `${en["legalEntities.icp"]}: at most 2,000 characters.`,
    ),
  ).toBeVisible();
  await waitFor(() => expect(icp).toHaveFocus());
});
