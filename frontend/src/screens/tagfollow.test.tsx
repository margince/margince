// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import {
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
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";
import { TagsPanel } from "./tagspanel";

// Applying a tag on a contact offers it to their company; applying one on a
// company offers it to its contacts. Neither writes before the reader presses.

const CONTACT = "01a06151-0000-7000-8000-0000000000c1";
const COMPANY = "01a06151-0000-7000-8000-0000000000a1";
const EMPLOYER = { company_id: COMPANY, company_name: "Demo GmbH" };
const VOCABULARY = {
  data: [{ id: "t-1", workspace_id: "w", name: "Product X" }],
  page: { has_more: false, next_cursor: null },
};
const PRODUCT_X = {
  tag_id: "t-1",
  name: "Product X",
  archived: false,
  assigned_at: "2026-06-01T09:00:00Z",
};
const offerTitle = en["tags.offerCompanyTitle"]
  .replace("{company}", "Demo GmbH")
  .replace("{tag}", "Product X");
const acceptName = en["tags.offerCompanyAccept"].replace(
  "{company}",
  "Demo GmbH",
);

type Writes = { path: string; body: unknown }[];

function mountContact({
  withEmployer = true,
  companyTagged = false,
  companyWritable = true,
} = {}) {
  const writes: Writes = [];
  const companyRead = { tags: 0, row: 0 };
  let companyTags = companyTagged ? [PRODUCT_X] : [];
  installFetchStub({
    "GET /me": meRoute({ company: ["update"], contact: ["update"] } as never),
    "GET /tags": () => jsonResponse(VOCABULARY),
    [`GET /records/contact/${CONTACT}/tags`]: () =>
      jsonResponse({ data: [], withheld: false }),
    [`GET /records/company/${COMPANY}/tags`]: () => {
      companyRead.tags += 1;
      return jsonResponse({ data: companyTags, withheld: false });
    },
    [`GET /companies/${COMPANY}`]: () => {
      companyRead.row += 1;
      return jsonResponse({
        id: COMPANY,
        name: "Demo GmbH",
        writable: companyWritable,
      });
    },
    "POST /tags/t-1/apply": (body) => {
      writes.push({ path: "apply", body });
      if ((body as { entity_type: string }).entity_type === "company") {
        companyTags = [PRODUCT_X];
      }
      return new Response(null, { status: 204 });
    },
  });
  render(
    <StoryProviders>
      <ToastProvider>
        <TagsPanel
          entityType="contact"
          entityID={CONTACT}
          canEdit
          employer={withEmployer ? EMPLOYER : undefined}
        />
        <ToastRegion />
      </ToastProvider>
    </StoryProviders>,
  );
  return { writes, companyRead };
}

async function applyProductX(user: ReturnType<typeof userEvent.setup>) {
  await user.click(await screen.findByRole("button", { name: en["tags.add"] }));
  const picker = within(await screen.findByRole("dialog"));
  await user.click(await picker.findByRole("option", { name: "Product X" }));
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("tagging a contact offers the tag to their company", () => {
  it("applies it to the company on one press", async () => {
    const user = userEvent.setup();
    const { writes } = mountContact();
    await applyProductX(user);

    await user.click(await screen.findByRole("button", { name: acceptName }));

    expect(
      await screen.findByText("Product X added to Demo GmbH"),
    ).toBeInTheDocument();
    expect(writes.map((write) => write.body)).toEqual([
      { entity_type: "contact", entity_id: CONTACT },
      { entity_type: "company", entity_id: COMPANY },
    ]);
    expect(screen.queryByText(offerTitle)).toBeNull();
  });

  it("writes nothing more when the reader dismisses it", async () => {
    const user = userEvent.setup();
    const { writes } = mountContact();
    await applyProductX(user);

    await screen.findByText(offerTitle);
    await user.click(
      screen.getByRole("button", { name: en["tags.offerDismiss"] }),
    );

    expect(screen.queryByText(offerTitle)).toBeNull();
    expect(writes).toHaveLength(1);
  });

  it("stays silent when the contact has no visible employer", async () => {
    const user = userEvent.setup();
    const { writes, companyRead } = mountContact({ withEmployer: false });
    await applyProductX(user);

    await screen.findByRole("button", { name: en["tags.add"] });
    expect(writes).toHaveLength(1);
    expect(companyRead).toEqual({ tags: 0, row: 0 });
    expect(screen.queryByText(offerTitle)).toBeNull();
  });

  it("stays silent when the company already carries the tag", async () => {
    const user = userEvent.setup();
    const { companyRead } = mountContact({ companyTagged: true });
    await applyProductX(user);

    // Both company reads answered, so the silence is a decision, not a wait.
    await waitFor(() => expect(companyRead).toEqual({ tags: 1, row: 1 }));
    await screen.findByRole("button", { name: en["tags.add"] });
    expect(screen.queryByText(offerTitle)).toBeNull();
  });

  it("stays silent when the reader may not change the company", async () => {
    const user = userEvent.setup();
    const { companyRead } = mountContact({ companyWritable: false });
    await applyProductX(user);

    // Both company reads answered, so the silence is a decision, not a wait.
    await waitFor(() => expect(companyRead).toEqual({ tags: 1, row: 1 }));
    await screen.findByRole("button", { name: en["tags.add"] });
    expect(screen.queryByText(offerTitle)).toBeNull();
  });
});

describe("tagging a company offers the tag to its contacts", () => {
  const contacts = [
    { id: "c-1", full_name: "Anna Weber", version: 3, tags: [] },
    { id: "c-2", full_name: "Ben Ott", version: 7, tags: [] },
    {
      id: "c-3",
      full_name: "Clara Ruiz",
      version: 2,
      tags: [{ tag_id: "t-1", name: "Product X" }],
    },
  ];

  function mountCompany(contactGrant: string[] = ["update"]) {
    const sent: { path: string; body: unknown }[] = [];
    const annaTagReads = { count: 0 };
    installFetchStub({
      "GET /me": meRoute({
        company: ["update"],
        contact: contactGrant,
      } as never),
      "GET /records/contact/c-1/tags": () => {
        annaTagReads.count += 1;
        return jsonResponse({ data: [], withheld: false });
      },
      "GET /tags": () => jsonResponse(VOCABULARY),
      [`GET /records/company/${COMPANY}/tags`]: () =>
        jsonResponse({ data: [], withheld: false }),
      "POST /tags/t-1/apply": () => new Response(null, { status: 204 }),
      "GET /contacts": () =>
        jsonResponse({
          data: contacts,
          page: { has_more: false, next_cursor: null },
        }),
      "POST /bulk/preview": (body) => {
        sent.push({ path: "preview", body });
        return jsonResponse({
          record_type: "contact",
          verb: "add_tag",
          count: 1,
          affected: ["c-1"],
          excluded: [{ id: "c-2", reason: "not_writable" }],
          sample: [],
          requires_confirmation: false,
          confirm_token: "tok-1",
        });
      },
      "POST /bulk/execute": (body) => {
        sent.push({ path: "execute", body });
        return jsonResponse({
          batch_id: "b-1",
          changed: 1,
          skipped: [{ id: "c-2", reason: "not_writable" }],
        });
      },
    });
    render(
      <StoryProviders>
        <ToastProvider>
          <TagsPanel entityType="company" entityID={COMPANY} canEdit />
          {/* Anna's own panel, which the bulk change must refresh. */}
          <TagsPanel entityType="contact" entityID="c-1" canEdit={false} />
          <ToastRegion />
        </ToastProvider>
      </StoryProviders>,
    );
    return { sent, annaTagReads };
  }

  // Eleven waits in sequence, one second each, so it states its own ceiling.
  it("lists current contacts, marks who carries it, and tags the ticked ones in bulk", async () => {
    const user = userEvent.setup();
    const { sent, annaTagReads } = mountCompany();
    await applyProductX(user);
    await waitFor(() => expect(annaTagReads.count).toBe(1));

    await user.click(
      await screen.findByRole("button", {
        name: en["tags.offerContactsAccept"],
      }),
    );
    const chooser = within(await screen.findByRole("dialog"));
    const clara = await chooser.findByRole("checkbox", {
      name: "Clara Ruiz (already tagged)",
    });
    expect(clara).toBeChecked();
    expect(clara).toBeDisabled();
    await user.click(chooser.getByRole("checkbox", { name: "Anna Weber" }));
    await user.click(chooser.getByRole("checkbox", { name: "Ben Ott" }));
    await user.click(
      chooser.getByRole("button", { name: en["tags.contactsContinue"] }),
    );

    // The bulk preview names the contact this reader may not edit.
    const review = within(await screen.findByRole("dialog"));
    expect(
      await review.findByText(en["bulk.reason.not_writable"]),
    ).toBeInTheDocument();
    expect(review.getByText("Ben Ott")).toBeInTheDocument();
    await user.click(
      review.getByRole("button", { name: en["bulk.confirmAddTag"] }),
    );

    await screen.findByText(/1 contact/);
    // The tagged contact's own panel reads its tags again.
    await waitFor(() => expect(annaTagReads.count).toBe(2));
    const items = [
      { id: "c-1", version: 3 },
      { id: "c-2", version: 7 },
    ];
    expect(sent).toEqual([
      {
        path: "preview",
        body: {
          record_type: "contact",
          verb: "add_tag",
          items,
          tag_id: "t-1",
        },
      },
      {
        path: "execute",
        body: {
          record_type: "contact",
          verb: "add_tag",
          items,
          tag_id: "t-1",
          confirm_token: "tok-1",
        },
      },
    ]);
  }, 12_000);

  it("writes nothing to the contacts when the reader dismisses the offer", async () => {
    const user = userEvent.setup();
    const { sent } = mountCompany();
    await applyProductX(user);

    await user.click(
      await screen.findByRole("button", { name: en["tags.offerDismiss"] }),
    );

    expect(
      screen.queryByRole("button", { name: en["tags.offerContactsAccept"] }),
    ).toBeNull();
    expect(sent).toEqual([]);
  });

  // Tagging a contact is a write to the contact, which /bulk/preview refuses
  // without contact.update, so the offer is not made.
  it("offers nothing to a reader who may change the company but not contacts", async () => {
    const user = userEvent.setup();
    mountCompany(["read"]);
    await applyProductX(user);

    await screen.findByRole("button", { name: en["tags.add"] });
    expect(
      screen.queryByRole("button", { name: en["tags.offerContactsAccept"] }),
    ).toBeNull();
    expect(
      screen.queryByText(
        en["tags.offerContactsTitle"].replace("{tag}", "Product X"),
      ),
    ).toBeNull();
  });
});
