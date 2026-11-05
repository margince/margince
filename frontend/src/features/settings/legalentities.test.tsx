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
    const refusal = Object.entries(refuse).find(([path]) =>
      request.url.endsWith(path),
    );
    if (refusal && request.method !== "GET") {
      return jsonResponse(refusal[1], 409);
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

it("describes a further entity with its name, legal name and country", async () => {
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
  expect(
    within(dialog).getByRole("combobox", { name: /Country/ }),
  ).toHaveTextContent(en["legalEntities.countryInstallation"]);
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

it("stores a chosen logo at once, and saves the next edit on the version it left", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal("fetch", backend(ADMIN, [...DEFAULT_ONLY, VIENNA], calls));
  render(<LegalEntitiesCard />);
  await rowVerb(user, "Acme Wien", en["legalEntities.edit"]);
  const dialog = await screen.findByRole("dialog");
  await user.upload(
    within(dialog).getByLabelText(en["legalEntities.logoAdd"]),
    new File(["png"], "logo.png", { type: "image/png" }),
  );
  await within(dialog).findByRole("button", {
    name: en["legalEntities.logoRemove"],
  });
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
    within(dialog).getByLabelText(en["legalEntities.logoAdd"]),
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
