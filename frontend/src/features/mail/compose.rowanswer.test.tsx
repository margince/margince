/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ChannelReplyAction } from "./compose";
import {
  type Activity,
  activity202,
  jsonResponse,
  render,
  stubRoutes,
} from "./compose.testkit";
import type { RelinkKind } from "./composerelink";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function person(id: string, address: string | null) {
  return {
    id,
    full_name: `Contact ${id}`,
    primary_email: address,
    reachability: [],
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  };
}

const onContact = (id: string) => ({
  entity_type: "contact" as const,
  entity_id: id,
});

function row(over: Partial<Activity> & Pick<Activity, "id" | "kind">) {
  return {
    ...activity202,
    occurred_at: "2026-07-01T09:30:00Z",
    links: [onContact("p-1")],
    ...over,
  } as Activity;
}

function Verb({
  activity,
  page,
}: Readonly<{
  activity: Activity;
  page: { type: RelinkKind; id: string; contactId?: string };
}>) {
  return (
    <ChannelReplyAction
      activityId={activity.id}
      kind={activity.kind}
      links={activity.links}
      automaticKind={activity.automatic_kind}
      entityType={page.type}
      entityId={page.id}
      contactId={page.contactId}
    />
  );
}

const CONTACT_PAGE = { type: "contact", id: "p-1", contactId: "p-1" } as const;
const DEAL_PAGE = { type: "deal", id: "d-1" } as const;
const EMAIL = row({ id: "a-mail", kind: "email", subject: "Angebot" });

function draw(
  rows: Activity[],
  page: { type: RelinkKind; id: string; contactId?: string },
  people = [person("p-1", "anna@example.test")],
  routes: Record<string, () => Response> = {},
) {
  stubRoutes({
    ...Object.fromEntries(
      people.map((one) => [`GET /contacts/${one.id}`, () => jsonResponse(one)]),
    ),
    ...Object.fromEntries(
      rows.map((one) => [`GET /activities/${one.id}`, () => jsonResponse(one)]),
    ),
    ...routes,
  });
  return render(
    <>
      {rows.map((one) => (
        <Verb key={one.id} activity={one} page={page} />
      ))}
    </>,
  );
}

// Absence is asserted only once the contact read has answered, so a gate that
// always offers the verb cannot pass by being early.
async function contactRead(
  drawn: ReturnType<typeof draw>,
  id: string,
): Promise<void> {
  await waitFor(() =>
    expect(drawn.queryClient.getQueryState(["contact", id])?.status).toBe(
      "success",
    ),
  );
}

describe("what a row offers to write", () => {
  it.each([
    row({ id: "a-call", kind: "call", subject: "Voicemail" }),
    row({ id: "a-note", kind: "note", subject: "Budget moved to Q4" }),
    row({ id: "a-meet", kind: "meeting", subject: "Site visit" }),
    row({ id: "a-task", kind: "task", subject: "Send the quote" }),
  ])(
    "opens a new email about a $kind row, never a reply to it",
    async (activity) => {
      const user = userEvent.setup();
      draw([activity], CONTACT_PAGE);

      await user.click(
        await screen.findByRole("button", { name: "Write email" }),
      );
      expect(screen.queryByRole("button", { name: "Reply" })).toBeNull();

      const dialog = await screen.findByRole("dialog", { name: /^New email/ });
      expect(
        await within(dialog).findByText(`About: ${activity.subject}`),
      ).toBeTruthy();
      expect(within(dialog).getByText(/^· .*2026/)).toBeTruthy();
      expect(within(dialog).queryByText(/Replying to/)).toBeNull();
      expect(within(dialog).queryByText(/Following up on/)).toBeNull();
      expect(
        within(dialog).getByRole("button", { name: "Draft email" }),
      ).toBeTruthy();
      expect(
        within(dialog).getByRole("textbox", { name: "Purpose of the email" }),
      ).toBeTruthy();
    },
  );

  it("keeps an email row a reply, and the dialog says what it answers", async () => {
    const user = userEvent.setup();
    draw([{ ...EMAIL, direction: "inbound" }], CONTACT_PAGE);

    await user.click(await screen.findByRole("button", { name: "Reply" }));
    expect(screen.queryByRole("button", { name: "Write email" })).toBeNull();

    const dialog = await screen.findByRole("dialog", { name: /^Re: Angebot/ });
    expect(
      await within(dialog).findByText(/^Replying to “Angebot” · /),
    ).toBeTruthy();
    expect(within(dialog).queryByText(/^About: /)).toBeNull();
    expect(
      within(dialog).getByRole("button", { name: "Draft reply" }),
    ).toBeTruthy();
  });

  it("offers nothing on a note when the contact has no address", async () => {
    const drawn = draw(
      [row({ id: "a-note", kind: "note", subject: "Called back" })],
      CONTACT_PAGE,
      [person("p-1", null)],
    );

    await contactRead(drawn, "p-1");
    expect(screen.queryByRole("button", { name: "Write email" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Reply" })).toBeNull();
  });

  it("writes to the row's own contact on a deal timeline", async () => {
    draw(
      [
        row({
          id: "a-note",
          kind: "note",
          links: [
            { entity_type: "deal" as const, entity_id: "d-1" },
            onContact("p-2"),
          ],
        }),
      ],
      DEAL_PAGE,
      [person("p-2", "ben@example.test")],
    );

    expect(
      await screen.findByRole("button", { name: "Write email" }),
    ).toBeTruthy();
  });

  it("offers nothing on a deal note the row links to no contact", async () => {
    draw(
      [
        EMAIL,
        row({
          id: "a-note",
          kind: "note",
          links: [{ entity_type: "deal" as const, entity_id: "d-1" }],
        }),
      ],
      DEAL_PAGE,
    );

    // The email row's Reply is the positive control in the same render.
    expect(await screen.findByRole("button", { name: "Reply" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Write email" })).toBeNull();
  });

  it.each([
    row({
      id: "t-system",
      kind: "task",
      subject: "1 forecast input needs attention",
      captured_by: "system",
      automatic_kind: "product",
    }),
    row({
      id: "t-two",
      kind: "task",
      subject: "Check the contract",
      links: [onContact("p-1"), onContact("p-2")],
    }),
  ])("offers no email on a task: $subject", async (task) => {
    const note = row({ id: "a-note", kind: "note", subject: "Called back" });
    draw([note, task], DEAL_PAGE, [
      person("p-1", "anna@example.test"),
      person("p-2", "ben@example.test"),
    ]);

    // The note's verb waits on the same contact read the task would.
    await screen.findByRole("button", { name: "Write email" });
    expect(screen.getAllByRole("button", { name: "Write email" })).toHaveLength(
      1,
    );
  });

  it("hands focus back to the verb when the composer closes", async () => {
    const user = userEvent.setup();
    draw(
      [row({ id: "a-call", kind: "call", subject: "Voicemail" })],
      CONTACT_PAGE,
    );

    const verb = await screen.findByRole("button", { name: "Write email" });
    await user.click(verb);
    const dialog = await screen.findByRole("dialog", { name: /^New email/ });
    await user.click(within(dialog).getByRole("button", { name: "Close" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).toBe(verb);
  });

  it("titles the email New email while the subject holds only the project key", async () => {
    const user = userEvent.setup();
    const call = row({
      id: "a-call",
      kind: "call",
      subject: "Voicemail",
      links: [
        { entity_type: "deal" as const, entity_id: "d-1" },
        onContact("p-1"),
      ],
    });
    draw([call], DEAL_PAGE, [person("p-1", "anna@example.test")], {
      "GET /deals/d-1": () =>
        jsonResponse({
          id: "d-1",
          name: "netcare",
          company_id: "o-1",
          project_id: "pr-9",
        }),
      "GET /companies/o-1/360": () =>
        jsonResponse({
          as_of: "2026-08-09T09:00:00Z",
          company: { id: "o-1", display_name: "netcare" },
          sections_omitted: [],
          contacts: { data: [], page: { next_cursor: null } },
          deals: { data: [], page: { next_cursor: null } },
          projects: [
            {
              project_id: "pr-9",
              name: "Netcare 2 project",
              key: "N2P-1",
              phase: "delivering",
            },
          ],
        }),
      "GET /projects/pr-9": () =>
        jsonResponse({ id: "pr-9", name: "Netcare 2 project", key: "N2P-1" }),
    });

    await user.click(
      await screen.findByRole("button", { name: "Write email" }),
    );
    const subject = (await screen.findByLabelText(
      "Subject",
    )) as HTMLInputElement;
    await waitFor(() => expect(subject.value).toBe("[N2P-1]"));
    expect(screen.getByRole("dialog", { name: /^New email/ })).toBeTruthy();
    expect(screen.queryByRole("dialog", { name: "[N2P-1]" })).toBeNull();
  });
});
