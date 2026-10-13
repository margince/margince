/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { RecordShell } from "../app/testing/recordshell.testkit";
import { meshOf, meshStyle } from "../design-system/avatarmesh";
import { LocaleProvider } from "../i18n";
import { CompaniesScreen, CompanyScreen } from "./companies";
import { company, company360, jsonResponse } from "./company.fixtures";
import { stubFetch } from "./company.testkit";
import { mount, view } from "./contactpage.testkit";
import { WriteToHost } from "./writeto";

// One record, one mark: the list a reader found it in, the card another record
// lists it on, its own page and a message it sent all key the chip on the
// record's id. Each surface is held against the mesh of that id, and one side
// carries a different name, so a chip keyed on its name fails here.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <WriteToHost>
          <RecordShell>{ui}</RecordShell>
        </WriteToHost>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The inline numbers the chip beside `name` draws its mesh from.
function meshBeside(name: string, holder: string): string {
  const chip = screen
    .getAllByText(name)
    .map((node) => node.closest(holder)?.querySelector(".avatar-mesh"))
    .find((node) => node != null);
  const mesh = chip?.getAttribute("style");
  if (!mesh?.includes("--avatar-hue-a")) {
    throw new Error(`no mesh beside ${name} in ${holder}`);
  }
  return mesh;
}

// The inline numbers a chip keyed on `key` carries, set as React sets them.
function meshKeyedOn(key: string): string {
  const probe = document.createElement("span");
  for (const [name, value] of Object.entries(meshStyle(meshOf(key)))) {
    probe.style.setProperty(name, String(value));
  }
  return probe.getAttribute("style") ?? "";
}

const headMesh = () =>
  meshBeside(
    screen.getByRole("heading", { level: 1 }).textContent ?? "",
    ".record-head",
  );

const noMore = { next_cursor: null, has_more: false };

const dana = {
  contact_id: "p-1",
  full_name: "Dana Buyer",
  title: "VP Procurement",
  strength: { score: 40, bucket: "moderate", factors: {}, inbound_90d: 1 },
  deal_roles: [],
  consent: {},
  routes: { top: [], remainder: 0, untried: false },
};

describe("a record's mark", () => {
  it("is the same in the companies list and on the company's page", async () => {
    // Listed under the name it had before a rename the page already shows.
    const listedAs = "Brandt Automotive";
    stubFetch(async (url) =>
      new URL(url).pathname.endsWith("/companies")
        ? jsonResponse({
            data: [{ ...company, display_name: listedAs }],
            page: noMore,
          })
        : jsonResponse(url.includes("/activities") ? { data: [] } : company),
    );
    render(<CompaniesScreen />);
    await screen.findByText(listedAs);
    expect(meshBeside(listedAs, ".avatar-row")).toBe(meshKeyedOn(company.id));
    cleanup();

    render(<CompanyScreen id={company.id} />);
    await screen.findByRole("heading", {
      level: 1,
      name: company.display_name,
    });
    expect(headMesh()).toBe(meshKeyedOn(company.id));
  });

  it("is the same on the card another record lists it on and on its own page", async () => {
    stubFetch(
      async (url) =>
        jsonResponse(url.includes("/activities") ? { data: [] } : company),
      {
        company360: {
          ...company360,
          contacts: { data: [danaCarded], page: noMore },
        },
      },
    );
    render(<CompanyScreen id={company.id} />);
    await screen.findByRole("link", { name: danaCarded.full_name });
    expect(meshBeside(danaCarded.full_name, ".record-card")).toBe(
      meshKeyedOn(dana.contact_id),
    );
    cleanup();
    vi.unstubAllGlobals();

    mount("overview");
    await screen.findByRole("heading", {
      level: 1,
      name: view.contact.full_name,
    });
    expect(headMesh()).toBe(meshKeyedOn(dana.contact_id));
  });

  it("is the same on a contact's page and on the message that contact sent", async () => {
    const faces = await facesOnThread(view.contact.full_name, view.contact.id);
    expect(faces).toEqual([
      meshKeyedOn(view.contact.id),
      meshKeyedOn(view.contact.id),
    ]);
    expect(headMesh()).toBe(meshKeyedOn(view.contact.id));
  });

  // A CONTACT RENAMED SINCE CAPTURE keeps one face. The phrase is frozen at the
  // words the message carried, so matching it against the contact's name today
  // finds nothing and draws a second colour for the same human. The id the
  // server resolved does not go stale.
  it("keys on the contact when the phrase holds the name they had then", async () => {
    const faces = await facesOnThread("Dana Lang", view.contact.id);
    expect(faces).toEqual([
      meshKeyedOn(view.contact.id),
      meshKeyedOn(view.contact.id),
    ]);
  });

  // Filed against this contact, sent by somebody else: the phrase names the
  // sender, so the face is theirs and never drawn in this contact's colour.
  it("is the sender's own on a message filed here that somebody else sent", async () => {
    const faces = await facesOnThread("Bob Stranger");
    expect(faces).toEqual([
      meshKeyedOn("Bob Stranger"),
      meshKeyedOn("Bob Stranger"),
    ]);
  });
});

// The same contact a card lists under the name it had before a rename.
const danaCarded = { ...dana, full_name: "Dana Buyer-Lang" };

type Activity = components["schemas"]["Activity"];

// Two inbound messages filed against the page's contact, one conversation,
// each naming its sender with the server's phrase AND the contact that phrase
// resolved to. The card draws the face from the id: a sender who is nobody here
// carries none, and the phrase is then all there is to key on.
const thread = (sender: string, senderContactId?: string): Activity[] =>
  ["m-2", "m-1"].map((id, at) => ({
    id,
    kind: "email",
    subject: "Fleet renewal",
    occurred_at: `2026-08-1${2 - at}T12:00:00Z`,
    direction: "inbound",
    thread_key: "t-fleet",
    is_done: false,
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-08-10T12:00:00Z",
    updated_at: "2026-08-10T12:00:00Z",
    links: [{ entity_type: "contact", entity_id: view.contact.id }],
    email_summary: {
      activity_id: id,
      occurred_at: `2026-08-1${2 - at}T12:00:00Z`,
      version: 1,
      subject: "Fleet renewal",
      preview: "Can you hold the price?",
      counterparty: sender,
      counterparty_contact_id: senderContactId,
      direction: "inbound",
      display_status: "team",
      move: "needs_reply",
      attachment_count: 0,
    },
  }));

async function facesOnThread(
  sender: string,
  senderContactId?: string,
): Promise<string[]> {
  mount("timeline", {
    ...view,
    activities: { data: thread(sender, senderContactId), page: noMore },
  });
  await screen.findByRole("heading", {
    level: 1,
    name: view.contact.full_name,
  });
  await screen.findAllByText("Can you hold the price?");
  return [...document.querySelectorAll(".tl-msg .avatar-mesh")].map(
    (face) => face.getAttribute("style") ?? "",
  );
}
