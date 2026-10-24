/** @vitest-environment happy-dom */
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
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

const onContact = (id: string) => ({
  entity_type: "contact" as const,
  entity_id: id,
});

// A row the server can address unless the case says otherwise.
function row(
  over: Partial<Activity> & Pick<Activity, "id" | "kind">,
): Activity {
  return {
    ...activity202,
    occurred_at: "2026-07-01T09:30:00Z",
    links: [onContact("p-1")],
    offers_email: over.kind !== "email" && over.kind !== "message",
    ...over,
  };
}

async function toField(dialog: HTMLElement): Promise<HTMLElement> {
  const input = await within(dialog).findByLabelText("To");
  const field = input.closest(".compose-address-field");
  if (!(field instanceof HTMLElement)) throw new Error("To has no field");
  return field;
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
      offersEmail={activity.offers_email}
      automaticKind={activity.automatic_kind}
      entityType={page.type}
      entityId={page.id}
      contactId={page.contactId}
      contentWithheld={activity.content_state === "withheld"}
    />
  );
}

const CONTACT_PAGE = { type: "contact", id: "p-1", contactId: "p-1" } as const;
const DEAL_PAGE = { type: "deal", id: "d-1" } as const;
const EMAIL = row({ id: "a-mail", kind: "email", subject: "Angebot" });
const ADDRESSED_CALL = row({
  id: "a-call",
  kind: "call",
  subject: "Voicemail",
});

// Each row's reply-recipient answers the address the server would fill To with.
function draw(
  rows: Activity[],
  page: { type: RelinkKind; id: string; contactId?: string },
  routes: Record<string, () => Response> = {},
) {
  stubRoutes({
    ...Object.fromEntries(
      rows.flatMap((one) => [
        [`GET /activities/${one.id}`, () => jsonResponse(one)],
        [
          `GET /activities/${one.id}/reply-recipient`,
          () =>
            jsonResponse({
              full_name: "Anna Weber",
              first_name: "Anna",
              address: "anna@example.test",
            }),
        ],
      ]),
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

describe("what a row offers to write", () => {
  it.each([
    [row({ id: "a-call", kind: "call", subject: "Voicemail" }), CONTACT_PAGE],
    [row({ id: "a-note", kind: "note", subject: "Budget moved" }), DEAL_PAGE],
    [
      row({ id: "a-meet", kind: "meeting", subject: "Site visit" }),
      CONTACT_PAGE,
    ],
    [
      row({ id: "a-task", kind: "task", subject: "Send the quote" }),
      CONTACT_PAGE,
    ],
  ] as const)(
    "opens a new email about a %# row, addressed, never a reply to it",
    async (activity, page) => {
      const user = userEvent.setup();
      draw([activity], page);

      await user.click(
        await screen.findByRole("button", { name: "Write email" }),
      );
      expect(screen.queryByRole("button", { name: "Reply" })).toBeNull();

      const dialog = await screen.findByRole("dialog", { name: /^New email/ });
      expect(
        await within(dialog).findByText(`About: ${activity.subject}`),
      ).toBeTruthy();
      expect(within(dialog).getByText(/^· .*2026/)).toBeTruthy();
      expect(
        await within(await toField(dialog)).findByText("anna@example.test"),
      ).toBeTruthy();
      expect(within(dialog).queryByText(/Replying to/)).toBeNull();
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

  // The addressed call beside each row is the positive control in one render.
  it.each([
    [
      "a row the server cannot address",
      row({ id: "n", kind: "note", offers_email: false }),
    ],
    [
      "a withheld note",
      row({
        id: "w",
        kind: "note",
        content_state: "withheld",
        offers_email: false,
      }),
    ],
    [
      "a task the product filed",
      row({ id: "t", kind: "task", automatic_kind: "product" }),
    ],
  ] as const)("offers nothing on %s", (_, activity) => {
    draw([ADDRESSED_CALL, activity], DEAL_PAGE);

    expect(screen.getAllByRole("button")).toHaveLength(1);
    expect(screen.getByRole("button", { name: "Write email" })).toBeTruthy();
  });

  // Withheld mail keeps its plain new email: no anchor, so nothing frames it.
  it("opens a plain new email from a withheld email row", async () => {
    const user = userEvent.setup();
    draw(
      [row({ id: "m", kind: "email", content_state: "withheld" })],
      CONTACT_PAGE,
    );

    await user.click(
      await screen.findByRole("button", { name: "Write email" }),
    );
    const dialog = await screen.findByRole("dialog", { name: /^New email/ });
    expect(await within(dialog).findByLabelText("Subject")).toBeTruthy();
    expect(within(dialog).queryByText(/^About: /)).toBeNull();
    expect(within(dialog).queryByText(/Replying to/)).toBeNull();
  });

  it("hands focus back to the verb when the composer closes", async () => {
    const user = userEvent.setup();
    draw([ADDRESSED_CALL], CONTACT_PAGE);

    const verb = await screen.findByRole("button", { name: "Write email" });
    await user.click(verb);
    const dialog = await screen.findByRole("dialog", { name: /^New email/ });
    await user.click(within(dialog).getByRole("button", { name: "Close" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).toBe(verb);
  });

  // Safari does not focus a button it clicks, so the press itself names the verb.
  it("hands focus back to a verb the click never focused", async () => {
    const user = userEvent.setup();
    draw([ADDRESSED_CALL], CONTACT_PAGE);

    const verb = await screen.findByRole("button", { name: "Write email" });
    fireEvent.click(verb);
    const dialog = await screen.findByRole("dialog", { name: /^New email/ });
    expect(document.activeElement).not.toBe(verb);
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
    draw([call], DEAL_PAGE, {
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
    await screen.findByDisplayValue("[N2P-1]");
    expect(screen.getByRole("dialog", { name: /^New email/ })).toBeTruthy();
    expect(screen.queryByRole("dialog", { name: "[N2P-1]" })).toBeNull();
  });
});
