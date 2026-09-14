/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { writeMessage } from "../design-system/richtext-testing";
import { pickOption } from "../design-system/select-testing";
import { LocaleProvider } from "../i18n";
import { ComposeModal } from "./compose";
import {
  allowedPreview,
  isPreviewDoor,
  previewedAddresses,
} from "./sendpermission.testkit";

// The head of the message: who it is to, and the two things a rep could not
// reach from here before — a blind copy, and the paper the mail is about.
//
// Both were on the wire already. The backend has long carried `bcc` and
// `attachment_ids` (ADR-0086/A131), and no surface in the product named
// either: a rep who wanted to send the offer they were writing about had to
// leave the drawer, file it on the record, and start the mail again.

type Sent = { key: string; body: unknown };

const CONTACT = {
  as_of: "2026-08-15T09:00:00Z",
  contact: {
    id: "p-1",
    full_name: "Dana Brandt",
    emails: [
      { email: "dana@brandt.example", is_primary: true },
      // A second address, because the primary is OFFERED into the To line and a
      // value the field already holds stops being suggested — which is the rule
      // the offer is built on, not an accident of this fixture.
      { email: "d.brandt@nordwand.example", is_primary: false },
    ],
  },
  sections_omitted: [],
};

const FILES = {
  data: [
    {
      id: "att-1",
      entity_type: "contact",
      entity_id: "p-1",
      filename: "Offer_Nordwand_v3.pdf",
      byte_size: 412_000,
    },
    {
      id: "att-2",
      entity_type: "contact",
      entity_id: "p-1",
      filename: "Site_survey.jpg",
      byte_size: 2_100_000,
    },
  ],
  page: { has_more: false },
};

const SENT_ACTIVITY = {
  id: "act-9",
  kind: "email",
  occurred_at: "2026-08-15T09:00:00Z",
  is_done: false,
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-08-15T09:00:00Z",
  updated_at: "2026-08-15T09:00:00Z",
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function stubRoutes(
  overrides: Record<string, () => Response | Promise<Response>> = {},
) {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      let body: unknown = null;
      if (method !== "GET") {
        try {
          body = request
            ? await request.clone().json()
            : JSON.parse(String(init?.body));
        } catch {
          body = null;
        }
      }
      sent.push({ key, body });
      const override = overrides[key];
      if (override) return override();
      if (key === "GET /contacts/p-1/360") return jsonResponse(CONTACT);
      if (key === "GET /attachments") return jsonResponse(FILES);
      if (key === "POST /emails") return jsonResponse(SENT_ACTIVITY, 202);
      if (isPreviewDoor(url.pathname)) {
        return jsonResponse(allowedPreview(previewedAddresses(body)));
      }
      return jsonResponse({ data: [] });
    }),
  );
  return sent;
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function drawer() {
  return (
    <ComposeModal
      entityType="contact"
      entityId="p-1"
      contactId="p-1"
      recordAddress="dana@brandt.example"
      open
      onClose={vi.fn()}
    />
  );
}

/** Everything a send needs beyond the head the test under study is about.
 *
 *  The "why" is answered because these compose an UNANCHORED message: no anchor
 *  read is stubbed, so the composer has nothing to derive the category from and
 *  asks — which is the behaviour compose.test.tsx pins on its own. */
async function fillAndSend(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText("Subject"), "The offer");
  writeMessage("Body", "Attached.");
  await pickOption(
    user,
    screen.getByLabelText("Why are you writing?"),
    "They asked me to get in touch",
  );
  await user.click(screen.getByRole("button", { name: "Send" }));
}

function sentMail(sent: Sent[]) {
  return sent.find((r) => r.key === "POST /emails")?.body as
    | Record<string, unknown>
    | undefined;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("the recipient lines", () => {
  // A reader remembers a colleague's name and not their address. The offer is
  // read off the 360 the drawer already holds, so it costs no request and
  // cannot disagree with the page behind it.
  it("offers the record's own contacts by name", async () => {
    const user = userEvent.setup();
    stubRoutes();
    render(drawer());

    await user.click(await screen.findByLabelText("To"));
    await user.keyboard("Dana");

    const offered = await screen.findAllByRole("option", {
      name: /Dana Brandt/,
    });
    // Her OTHER address: the one already in the To line is not on offer.
    expect(offered).toHaveLength(1);
  });

  // The list is HELP, never a constraint: a first message to somebody nobody
  // has on file is the ordinary case, and a field that refused it would be
  // worse than the plain box it replaced.
  it("still takes an address nobody has on file", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes();
    render(drawer());

    await user.type(
      await screen.findByLabelText("To"),
      "stranger@example.com{Enter}",
    );
    await fillAndSend(user);

    await waitFor(() => expect(sentMail(sent)).toBeTruthy());
    // Beside the record's own address, which the drawer offered into the empty
    // field — what the reader typed is added, never substituted.
    expect(sentMail(sent)?.to).toContain("stranger@example.com");
  });
});

describe("the blind copy", () => {
  // A blind copy is the rare half of addressing, and an always-drawn third row
  // made every ordinary mail read as a form with an empty field in it. It is
  // also the one field that reaches somebody the recipients cannot see, which
  // is worth an explicit press.
  it("is a button until it is asked for", async () => {
    const user = userEvent.setup();
    stubRoutes();
    render(drawer());

    await screen.findByLabelText("Cc");
    expect(screen.queryByLabelText("Bcc")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Bcc" }));
    expect(await screen.findByLabelText("Bcc")).toBeTruthy();
  });

  it("travels on the send once the reader fills it", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes();
    render(drawer());

    await user.click(await screen.findByRole("button", { name: "Bcc" }));
    await user.type(
      screen.getByLabelText("Bcc"),
      "records@brandt.example{Enter}",
    );
    await fillAndSend(user);

    await waitFor(() => expect(sentMail(sent)).toBeTruthy());
    expect(sentMail(sent)?.bcc).toEqual(["records@brandt.example"]);
  });

  // A mail nobody blind-copied sends no key at all, rather than an empty list.
  it("sends no key when nobody was blind-copied", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes();
    render(drawer());

    await screen.findByLabelText("To");
    await fillAndSend(user);

    await waitFor(() => expect(sentMail(sent)).toBeTruthy());
    expect(sentMail(sent)).not.toHaveProperty("bcc");
  });
});

describe("what the message carries", () => {
  it("attaches paper already filed on the record", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes();
    render(drawer());

    await user.click(await screen.findByRole("button", { name: /Attach/ }));
    await user.click(
      await screen.findByRole("button", { name: /^Offer_Nordwand_v3\.pdf/ }),
    );
    await fillAndSend(user);

    await waitFor(() => expect(sentMail(sent)).toBeTruthy());
    // NAMED BY ID, never uploaded here: the send snapshots each file, so what
    // travels is a set of references.
    expect(sentMail(sent)?.attachment_ids).toEqual(["att-1"]);
  });

  it("takes a file back off before the send", async () => {
    const user = userEvent.setup();
    const sent = stubRoutes();
    render(drawer());

    await user.click(await screen.findByRole("button", { name: /Attach/ }));
    await user.click(
      await screen.findByRole("button", { name: /^Offer_Nordwand_v3\.pdf/ }),
    );
    await user.click(
      await screen.findByRole("button", { name: /Do not send/ }),
    );
    await fillAndSend(user);

    await waitFor(() => expect(sentMail(sent)).toBeTruthy());
    expect(sentMail(sent)).not.toHaveProperty("attachment_ids");
  });

  // A file already on the message is not on offer: the row refuses rather than
  // adding a second copy of one document, which is not something a message can
  // mean — the recipient would see the same paper twice.
  it("refuses a file the message already carries", async () => {
    const user = userEvent.setup();
    stubRoutes();
    render(drawer());

    await user.click(await screen.findByRole("button", { name: /Attach/ }));
    const row = await screen.findByRole("button", {
      name: /^Offer_Nordwand_v3\.pdf/,
    });
    await user.click(row);

    await waitFor(() =>
      expect(
        screen
          .getByRole("button", { name: /^Offer_Nordwand_v3\.pdf/ })
          .hasAttribute("disabled"),
      ).toBe(true),
    );
  });
});

// --- Where a message files, per record kind --------------------------------
//
// The picker is drawn only where a project is actually connected: a dropdown
// over nothing asks a reader to open a menu to discover there is no choice.
// What differs by record is where the connected set COMES from, and two of the
// five reached none — a message written from a contact or from a project landed
// unfiled while the composer showed no way to say otherwise.

const PROJECT = {
  id: "proj-1",
  name: "Nordwand retrofit",
  key: "NW-12",
  phase: "delivering",
  company_id: "company-1",
  version: 1,
};

const COMPANY_360 = {
  company: { id: "company-1", name: "Nordwand GmbH" },
  contacts: {
    data: [
      {
        contact_id: "per-2",
        full_name: "Milo Fenn",
        primary_email: "milo@nordwand.example",
      },
    ],
  },
  projects: [PROJECT],
  sections_omitted: [],
};

function fileStubs(
  overrides: Record<string, () => Response | Promise<Response>> = {},
) {
  return stubRoutes({
    "GET /projects/proj-1": () => jsonResponse(PROJECT),
    // The endpoint answers the 360 itself; the ready/overlay wrapper is the
    // hook's own reading of it, not something the wire carries.
    "GET /companies/company-1/360": () => jsonResponse(COMPANY_360),
    ...overrides,
  });
}

describe("the project a message files under", () => {
  // A contact page reached NO project set at all: the composer offered a
  // filing it could never make, so a mail about the retrofit landed on the
  // contact and nowhere else.
  it("offers the contact's own projects on a contact", async () => {
    stubRoutes({
      "GET /contacts/p-1/360": () =>
        jsonResponse({ ...CONTACT, projects: [PROJECT] }),
    });
    render(drawer());

    expect(await screen.findByLabelText("Project")).toBeTruthy();
  });

  // Nothing connected is nothing to ask about. The fixture at the top of this
  // file carries no projects, which is the ordinary contact.
  it("draws no picker when no project is connected", async () => {
    stubRoutes();
    render(drawer());

    await screen.findByLabelText("To");
    expect(screen.queryByLabelText("Project")).toBeNull();
  });

  // A message written FROM a project is about that project. There is no other
  // filing it could mean, and deriving one from a thread would leave the
  // record's own mail unfiled against the record it was written from.
  it("files a project's own mail under it, chosen and visible", async () => {
    fileStubs();
    render(
      <ComposeModal
        entityType="project"
        entityId="proj-1"
        open
        onClose={vi.fn()}
      />,
    );

    // The sole live project is the default, SHOWN in the picker rather than
    // sent silently.
    expect(await screen.findByText("Scoped to NW-12")).toBeTruthy();
  });

  it("sends a project's mail linked to the project", async () => {
    const user = userEvent.setup();
    const sent = fileStubs();
    render(
      <ComposeModal
        entityType="project"
        entityId="proj-1"
        open
        onClose={vi.fn()}
      />,
    );

    await user.type(
      await screen.findByLabelText("To"),
      "milo@nordwand.example{Enter}",
    );
    await fillAndSend(user);

    await waitFor(() => expect(sentMail(sent)).toBeTruthy());
    expect(sentMail(sent)?.links).toEqual([
      { entity_type: "project", entity_id: "proj-1" },
    ]);
  });

  // The account behind the project is where its contacts come from: a project is
  // not a contact and has no roster of its own.
  it("offers the account's contacts on a project", async () => {
    const user = userEvent.setup();
    fileStubs();
    render(
      <ComposeModal
        entityType="project"
        entityId="proj-1"
        open
        onClose={vi.fn()}
      />,
    );

    await user.click(await screen.findByLabelText("To"));

    expect(
      await screen.findByRole("option", { name: /Milo Fenn/ }),
    ).toBeTruthy();
  });

  // A closed project is history, and a message written today is not about
  // history — so the picker stands down exactly as it does with none connected.
  it("draws no picker for a closed project", async () => {
    fileStubs({
      "GET /projects/proj-1": () =>
        jsonResponse({ ...PROJECT, phase: "closed" }),
    });
    render(
      <ComposeModal
        entityType="project"
        entityId="proj-1"
        open
        onClose={vi.fn()}
      />,
    );

    await screen.findByLabelText("To");
    expect(screen.queryByLabelText("Project")).toBeNull();
  });
});
