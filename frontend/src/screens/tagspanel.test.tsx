// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import {
  act,
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ToastProvider, ToastRegion } from "../design-system/toast";
import { en } from "../i18n/en";
import { CompanyTagsSection } from "./companyrailtags";
import { ContactTagsSection } from "./contactrail";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";
import { TagsPanel } from "./tagspanel";

// What the panel SAYS in each state. Three of them answer 200 with a list, so
// the wire cannot tell them apart — only the words on screen can, which is
// what these assert.

const COMPANY = "01a06151-0000-7000-8000-000000000001";

type PanelTag = {
  tag_id: string;
  name: string;
  color?: string;
  archived: boolean;
  assigned_at: string;
  assigned_by?: { display_name: string; kind: string };
};

function mount(tags: PanelTag[], withheld = false, canEdit = true) {
  installFetchStub({
    [`GET /records/company/${COMPANY}/tags`]: () =>
      jsonResponse({ data: tags, withheld }),
  });
  render(
    <StoryProviders>
      <TagsPanel entityType="company" entityID={COMPANY} canEdit={canEdit} />
    </StoryProviders>,
  );
}

const KEY_ACCOUNT: PanelTag = {
  tag_id: "t-1",
  name: "Key Account",
  color: "amber",
  archived: false,
  assigned_at: "2026-03-03T10:00:00Z",
  assigned_by: { display_name: "Lena Fischer", kind: "human" },
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the tags panel", () => {
  // A malformed answer must not take the RECORD PAGE down with it. This one
  // is what a stub or a server disagreeing with the contract sends, and
  // reading .slice off the missing list crashed every company screen test
  // until the default landed.
  it("survives an answer that carries no list at all", async () => {
    installFetchStub({
      [`GET /records/company/${COMPANY}/tags`]: () => jsonResponse({}),
    });
    render(
      <StoryProviders>
        <TagsPanel entityType="company" entityID={COMPANY} canEdit />
      </StoryProviders>,
    );
    // The panel draws its empty state rather than throwing.
    expect(await screen.findByText(en["tags.emptyTitle"])).toBeInTheDocument();
  });

  it("draws the words a record carries", async () => {
    mount([KEY_ACCOUNT]);
    expect(await screen.findByText("Key Account")).toBeInTheDocument();
  });

  // The distinction the whole read exists to carry: a caller who may see the
  // record and not the vocabulary must not be told the record has no tags.
  it("says the words are withheld rather than claiming there are none", async () => {
    mount([], true);
    expect(await screen.findByText(en["tags.withheld"])).toBeInTheDocument();
    expect(screen.queryByText(en["tags.emptyTitle"])).toBeNull();
  });

  it("teaches what tags are for when a record carries none", async () => {
    mount([]);
    expect(await screen.findByText(en["tags.emptyTitle"])).toBeInTheDocument();
    expect(screen.queryByText(en["tags.withheld"])).toBeNull();
  });

  // Past four the rest fold away, and the reader can open them.
  it("folds the words past the fourth behind an expander", async () => {
    const user = userEvent.setup();
    mount(
      ["A", "B", "C", "D", "E", "F"].map((name, i) => ({
        ...KEY_ACCOUNT,
        tag_id: `t-${i}`,
        name,
      })),
    );
    expect(await screen.findByText("A")).toBeInTheDocument();
    expect(screen.queryByText("F")).toBeNull();

    await user.click(screen.getByRole("button", { name: /\+2/ }));
    await waitFor(() => expect(screen.getByText("F")).toBeInTheDocument());
  });

  // Applying a tag writes to the RECORD, so a reader who may only look at one
  // sees the words and no verb to change them.
  it("offers no remove verb to a reader who may not edit the record", async () => {
    mount([KEY_ACCOUNT], false, false);
    await screen.findByText("Key Account");

    expect(
      screen.queryByRole("button", {
        name: en["tags.removeTag"].replace("{name}", "Key Account"),
      }),
    ).toBeNull();
  });

  it("names who applied a tag, and when, on the tag itself", async () => {
    mount([KEY_ACCOUNT]);
    const link = await screen.findByRole("link", { name: /Key Account/ });

    act(() => link.focus());

    const tip = await screen.findByRole("tooltip");
    expect(tip).toHaveTextContent(/Added by Lena Fischer/);
    expect(link).toHaveAttribute("aria-describedby", tip.id);
  });

  // An assignment written before the product recorded WHO has nobody to
  // credit, and inventing a name would put a choice on somebody.
  it("shows the date alone when the assignment names nobody", async () => {
    mount([{ ...KEY_ACCOUNT, assigned_by: undefined }]);
    const link = await screen.findByRole("link", { name: /Key Account/ });

    act(() => link.focus());

    const tip = await screen.findByRole("tooltip");
    expect(tip).toHaveTextContent(/^Added /);
    expect(tip).not.toHaveTextContent(/Lena Fischer/);
  });
});

describe("taking a tag off a record", () => {
  const AUDIT = "0199a000-0000-7000-8000-0000000000a1";
  const REMOVE = `DELETE /tags/${KEY_ACCOUNT.tag_id}/apply`;
  const RESTORE = `POST /tags/${KEY_ACCOUNT.tag_id}/apply/restore`;
  const removeName = en["tags.removeTag"].replace("{name}", "Key Account");

  function serve(
    answers: Partial<Record<"remove" | "restore", () => Response>> = {},
  ) {
    let carried = [KEY_ACCOUNT];
    const sent: { remove: unknown[]; restore: unknown[] } = {
      remove: [],
      restore: [],
    };
    installFetchStub({
      [`GET /records/company/${COMPANY}/tags`]: () =>
        jsonResponse({ data: carried, withheld: false }),
      [REMOVE]: (body) => {
        sent.remove.push(body);
        const answer = answers.remove?.();
        if (answer) {
          return answer;
        }
        carried = [];
        return jsonResponse({ audit_id: AUDIT });
      },
      [RESTORE]: (body) => {
        sent.restore.push(body);
        const answer = answers.restore?.();
        if (answer) {
          return answer;
        }
        carried = [KEY_ACCOUNT];
        return jsonResponse({});
      },
    });
    render(
      <StoryProviders>
        <ToastProvider>
          <TagsPanel entityType="company" entityID={COMPANY} canEdit />
          <ToastRegion />
        </ToastProvider>
      </StoryProviders>,
    );
    return sent;
  }

  const refused = () =>
    jsonResponse({ detail: "The tag was applied again since." }, 409);

  it("removes at once, with no dialog, and offers Undo", async () => {
    const sent = serve();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: removeName }));

    const said = await screen.findByRole("status");
    await waitFor(() =>
      expect(said).toHaveTextContent(
        en["tags.removed"].replace("{name}", "Key Account"),
      ),
    );
    expect(sent.remove).toEqual([
      { entity_type: "company", entity_id: COMPANY },
    ]);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(
      within(said).getByRole("button", { name: en["common.undo"] }),
    ).toBeInTheDocument();
  });

  it("hands focus to the Add tag row once the pill is gone", async () => {
    serve();
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: removeName }));

    await waitFor(() => expect(screen.queryByText("Key Account")).toBeNull());
    expect(document.activeElement).not.toBe(document.body);
    expect(document.activeElement).toContainElement(
      screen.getByRole("button", { name: en["tags.add"] }),
    );
  });

  it("puts the tag back through Undo with the removal's audit id", async () => {
    const sent = serve();
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: removeName }));
    const said = await screen.findByRole("status");

    await user.click(
      await within(said).findByRole("button", { name: en["common.undo"] }),
    );

    await waitFor(() => expect(sent.restore).toEqual([{ audit_id: AUDIT }]));
    expect(await screen.findByText("Key Account")).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(
        en["tags.restored"].replace("{name}", "Key Account"),
      ),
    );
  });

  it("offers no Undo when the record no longer carried the tag", async () => {
    serve({ remove: () => new Response(null, { status: 204 }) });
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: removeName }));

    const said = await screen.findByRole("status");
    await waitFor(() =>
      expect(said).toHaveTextContent(
        en["tags.removed"].replace("{name}", "Key Account"),
      ),
    );
    expect(
      within(said).queryByRole("button", { name: en["common.undo"] }),
    ).toBeNull();
  });

  it("keeps a refused removal on screen as a danger toast", async () => {
    serve({ remove: refused });
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: removeName }));

    const said = await screen.findByRole("status");
    await waitFor(() =>
      expect(said).toHaveTextContent("The tag was applied again since."),
    );
    expect(said.querySelector(".toast-dot-danger")).not.toBeNull();
    expect(
      within(said).getByRole("button", { name: en["common.close"] }),
    ).toBeInTheDocument();
    expect(screen.getByText("Key Account")).toBeInTheDocument();
  });

  it("keeps a refused Undo on screen as a danger toast", async () => {
    serve({ restore: refused });
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: removeName }));
    const said = await screen.findByRole("status");

    await user.click(
      await within(said).findByRole("button", { name: en["common.undo"] }),
    );

    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent(
        "The tag was applied again since.",
      ),
    );
    expect(
      screen.getByRole("status").querySelector(".toast-dot-danger"),
    ).not.toBeNull();
  });
});

// The add-tag verb and the panel answer to ONE read. The panel draws nothing
// until it lands and says "hidden" when the vocabulary is withheld, so a button
// gated on permission alone floated above no panel and opened a picker whose
// apply the server refuses.
describe("the company mount's add-tag verb", () => {
  const COMPANY_ROW = {
    id: COMPANY,
    name: "Aurora GmbH",
    writable: true,
  };

  function mountCompany(
    withheld: boolean,
    grants: Record<string, string[]> = { company: ["update"] },
    row: { writable: boolean } = { writable: true },
    seat: "full" | "read" = "full",
  ) {
    installFetchStub({
      "GET /me": meRoute(grants as never, { seat }),
      [`GET /records/company/${COMPANY}/tags`]: () =>
        jsonResponse({ data: [], withheld }),
    });
    render(
      <StoryProviders>
        <CompanyTagsSection
          company={{ ...COMPANY_ROW, ...row } as never}
          companyId={COMPANY}
        />
      </StoryProviders>,
    );
  }

  // The control: without it, a verb that never renders would pass the test
  // below for the wrong reason.
  it("offers the verb once the read lands and the words are visible", async () => {
    mountCompany(false);
    expect(
      await screen.findByRole("button", { name: en["tags.add"] }),
    ).toBeInTheDocument();
  });

  it("offers no verb when the vocabulary is withheld", async () => {
    mountCompany(true);
    expect(await screen.findByText(en["tags.withheld"])).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: en["tags.add"] })).toBeNull();
  });

  // Applying writes to the RECORD, so the server asks for `company.update`
  // as well. A seat without it would be offered a picker whose apply is refused.
  // The row axis. A rep holding `company.update` on the OBJECT still may
  // not write a colleague's company, and the server stamps that as `writable`.
  it("offers no verb on a company this reader may not write", async () => {
    mountCompany(false, { company: ["update"] }, { writable: false });
    expect(await screen.findByText(en["tags.emptyTitle"])).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: en["tags.add"] })).toBeNull();
  });

  // The seat axis. A read seat is refused by the licensing middleware before
  // RBAC is consulted, so a verb offered to one cannot lead to a saved tag.
  it("offers no verb on a company to a read seat", async () => {
    mountCompany(false, { company: ["update"] }, { writable: true }, "read");
    expect(await screen.findByText(en["tags.emptyTitle"])).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: en["tags.add"] })).toBeNull();
  });

  it("offers no verb to a seat holding no update grant", async () => {
    mountCompany(false, {});
    expect(await screen.findByText(en["tags.emptyTitle"])).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: en["tags.add"] })).toBeNull();
  });
});

// The verb belongs to the PANEL, not to each host that mounts it. A contact page
// once shipped with tags a reader could see and no way to add one, because the
// add button lived in the company wrapper and the contact mount never placed it.
// These drive the REAL mount, so they fail if it stops computing `canEdit`
// correctly — a literal prop would only prove the panel obeys whatever it gets.
describe("the contact mount offers the verb the panel draws", () => {
  const CONTACT = "01a06151-0000-7000-8000-000000000002";

  function mountContact(
    contact: Record<string, unknown>,
    grants: Record<string, string[]> = { contact: ["update"] },
    seat: "full" | "read" = "full",
  ) {
    installFetchStub({
      "GET /me": meRoute(grants as never, { seat }),
      [`GET /records/contact/${CONTACT}/tags`]: () =>
        jsonResponse({ data: [], withheld: false }),
    });
    render(
      <StoryProviders>
        <ContactTagsSection
          view={{ contact: { id: CONTACT, ...contact } } as never}
        />
      </StoryProviders>,
    );
  }

  // The control: without it, a verb that never renders would pass every test
  // below for the wrong reason.
  it("offers the verb on a contact the seat may write", async () => {
    mountContact({ writable: true });
    expect(
      await screen.findByRole("button", { name: en["tags.add"] }),
    ).toBeInTheDocument();
  });

  // The row axis. A rep holding `contact.update` on the OBJECT still may not
  // write a colleague's contact, and the server stamps that as `writable`.
  it("offers no verb on a contact this reader may not write", async () => {
    mountContact({ writable: false });
    expect(await screen.findByText(en["tags.emptyTitle"])).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: en["tags.add"] })).toBeNull();
  });

  // The seat axis. A read seat is refused by the licensing middleware before
  // RBAC is consulted, so a verb offered to one is a control whose save cannot
  // succeed.
  it("offers no verb to a read seat", async () => {
    mountContact({ writable: true }, { contact: ["update"] }, "read");
    expect(await screen.findByText(en["tags.emptyTitle"])).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: en["tags.add"] })).toBeNull();
  });

  // The object axis, for completeness: all three are necessary and none of the
  // others would catch a mount that dropped this one.
  it("offers no verb to a seat holding no update grant", async () => {
    mountContact({ writable: true }, {});
    expect(await screen.findByText(en["tags.emptyTitle"])).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: en["tags.add"] })).toBeNull();
  });
});
