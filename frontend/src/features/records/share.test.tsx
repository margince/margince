/** @vitest-environment happy-dom */
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { RosterSubject } from "../../components/share.parts";
import { pickOption } from "../../design-system/select-testing";
import { en } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import { ShareScreen } from "./share";
import { pickedToGrant } from "./share.roster";
import {
  existingGrant,
  grantEndpoint,
  installBaseFetch,
  jsonResponse,
  ownedAccess,
  PICK_MOR,
  render,
  SHARER,
  writeGrant,
} from "./share.testkit";

// AS-3/4/5 — the record-share screen (A52/ADR-0039): list who has manual
// access to this one record, grant a new user/team subject, revoke an
// existing grant. Mirrors relationships.test.tsx's fetch-stub convention.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("ShareScreen", () => {
  it("defaults a new team share to lasting write access and a colleague share to seven days of read access", async () => {
    installBaseFetch();
    render(<ShareScreen recordType="deal" recordId="d-1" />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: /Deal Desk/ }));
    expect(
      screen.getByRole("button", { name: "Write", pressed: true }),
    ).toBeTruthy();
    expect(screen.getByLabelText("Expiry").textContent).toContain("No expiry");
    await user.clear(screen.getByLabelText("Person or team"));
    await user.click(await screen.findByRole("button", { name: /Priya Shah/ }));
    expect(
      screen.getByRole("button", { name: "Read", pressed: true }),
    ).toBeTruthy();
    expect(screen.getByLabelText("Expiry").textContent).toContain("7 days");
  });

  it("shares a company's current contacts through the shared grant form and reports both counts", async () => {
    const posts: Record<string, unknown>[] = [];
    installBaseFetch({
      "/companies/co-1/share-contacts": async (request) => {
        posts.push(await request.json());
        return jsonResponse({ shared: 3, skipped: 1 });
      },
    });
    render(<ShareScreen recordType="company-contacts" recordId="co-1" />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: /Deal Desk/ }));
    await user.click(screen.getByTestId("share-grant-submit"));
    const confirm = await screen.findByRole("dialog", {
      name: "Share these contacts?",
    });
    expect(posts).toEqual([]);
    await user.click(
      within(confirm).getByRole("button", { name: "Grant access" }),
    );
    expect(await screen.findByRole("status")).toHaveProperty(
      "textContent",
      "3 shared · 1 skipped",
    );
    expect(posts).toEqual([
      { subject_type: "team", subject_id: "t-1", access: "write" },
    ]);
    expect(screen.queryByTestId("share-acl-list")).toBeNull();
  });

  it("says so when an edit share is saved as read because of the colleague's role", async () => {
    const posts: Record<string, unknown>[] = [];
    installBaseFetch({
      "/record-grants": (request) => {
        if (request.method === "POST") {
          return request.json().then((body: Record<string, unknown>) => {
            posts.push(body);
            return jsonResponse(
              {
                ...existingGrant,
                ...body,
                access: "read",
                limited_by_role: true,
              },
              201,
            );
          });
        }
        return jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        });
      },
    });
    render(<ShareScreen recordType="deal" recordId="d-1" />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: /Priya Shah/ }));
    await user.click(screen.getByRole("button", { name: "Write" }));
    await user.click(screen.getByTestId("share-grant-submit"));

    const notice = await screen.findByTestId("share-limited-by-role");
    expect(notice.textContent).toMatch(
      /^Saved as read access\. Priya Shah cannot edit .+ with their role\.$/,
    );
    expect(posts).toMatchObject([{ access: "write" }]);
    expect(screen.queryByText(/now has/)).toBeNull();
  });

  it("says so when company contacts shared for editing are saved as read", async () => {
    installBaseFetch({
      "/companies/co-1/share-contacts": () =>
        jsonResponse({ shared: 2, skipped: 0, limited_by_role: true }),
    });
    render(<ShareScreen recordType="company-contacts" recordId="co-1" />);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: /Deal Desk/ }));
    await user.clear(screen.getByLabelText("Person or team"));
    await user.click(await screen.findByRole("button", { name: /Priya Shah/ }));
    await user.click(screen.getByRole("button", { name: "Write" }));
    await user.click(screen.getByTestId("share-grant-submit"));
    const confirm = await screen.findByRole("dialog", {
      name: "Share these contacts?",
    });
    await user.click(
      within(confirm).getByRole("button", { name: "Grant access" }),
    );

    expect(
      (await screen.findByTestId("share-limited-by-role")).textContent,
    ).toBe(
      "Saved as read access. Priya Shah cannot edit contacts with their role.",
    );
  });

  it("renders who-has-access, resolving subject and granter names via the roster", async () => {
    installBaseFetch();
    render(<ShareScreen recordType="deal" recordId="d-1" />);

    const aclList = await screen.findByTestId("share-acl-list");
    await waitFor(() =>
      expect(within(aclList).getByText("Mor Adler")).toBeTruthy(),
    );
    expect(within(aclList).getByText("compliance review")).toBeTruthy();
    await waitFor(() =>
      expect(within(aclList).getByText("Priya Shah")).toBeTruthy(),
    );
  });

  it("picking a seeded user + Read + submitting POSTs the grant body", async () => {
    let posted: unknown = null;
    installBaseFetch({
      "/record-grants": (request) => {
        if (request.method === "POST") {
          return request.json().then((body) => {
            posted = body;
            return jsonResponse(
              {
                id: "g-2",
                record_type: "deal",
                record_id: "d-1",
                subject_type: "user",
                subject_id: "u-1",
                access: "read",
                granted_by: "u-1",
                reason: body.reason ?? null,
                expires_at: body.expires_at ?? null,
                created_at: "2026-07-14T00:00:00Z",
                version: 1,
              },
              201,
            );
          });
        }
        return jsonResponse({
          data: [existingGrant],
          page: { next_cursor: null, has_more: false },
        });
      },
    });
    render(<ShareScreen recordType="deal" recordId="d-1" />);

    const pick = await screen.findByRole("button", { name: /Priya Shah/ });
    await userEvent.click(pick);

    const reasonBox = screen.getByLabelText(/reason/i);
    await userEvent.type(reasonBox, "Deal-desk review");

    const submit = screen.getByTestId("share-grant-submit");
    await userEvent.click(submit);

    await waitFor(() => expect(posted).not.toBeNull());
    expect(posted).toMatchObject({
      record_type: "deal",
      record_id: "d-1",
      subject_type: "user",
      subject_id: "u-1",
      access: "read",
      reason: "Deal-desk review",
    });
    expect((posted as Record<string, unknown>).expires_at).toBeDefined();
  });

  // The expiry control converts between a DAY COUNT (the screen's state) and the
  // strings a listbox speaks, and the wire carries neither: it carries a timestamp,
  // or nothing at all for a grant that lasts until it is revoked. Both directions
  // of that conversion are asserted, because the two failures look identical on
  // screen — a grant that silently never expires, and one that expires today.
  it("turns the chosen expiry into a timestamp, and no expiry into none", async () => {
    // Collected in an ARRAY rather than into a `let`: a variable only ever
    // assigned inside a `.then()` still reads as its initial `null` to the
    // compiler, so every field access off it narrows to `never`. An array keeps
    // its element type, and the two grants are two entries.
    const posts: Record<string, unknown>[] = [];
    const grant = (body: Record<string, unknown>) =>
      jsonResponse(
        {
          id: "g-3",
          record_type: "deal",
          record_id: "d-1",
          subject_type: "user",
          subject_id: "u-1",
          access: "read",
          granted_by: "u-1",
          reason: body.reason ?? null,
          expires_at: body.expires_at ?? null,
          created_at: "2026-07-14T00:00:00Z",
          version: 1,
        },
        201,
      );
    installBaseFetch({
      "/record-grants": (request) => {
        if (request.method === "POST") {
          return request.json().then((body) => {
            posts.push(body);
            return grant(body);
          });
        }
        return jsonResponse({
          data: [existingGrant],
          page: { next_cursor: null, has_more: false },
        });
      },
    });
    render(<ShareScreen recordType="deal" recordId="d-1" />);
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /Priya Shah/ }));
    await pickOption(
      user,
      screen.getByLabelText("Expiry"),
      "Expires in 7 days",
    );
    await user.click(screen.getByTestId("share-grant-submit"));

    await waitFor(() => expect(posts).toHaveLength(1));
    // A real instant, seven days out — asserted as a distance rather than as a
    // literal, because the clock is the one thing a test cannot pin.
    const days =
      (Date.parse(String(posts[0].expires_at)) - Date.now()) / 86_400_000;
    expect(days).toBeGreaterThan(6.9);
    expect(days).toBeLessThan(7.1);

    // A granted subject is picked again for the second half: a successful grant
    // clears the form, which is what the reader sees, so the case has to walk the
    // same path they would rather than reusing a form that is no longer filled in.
    await user.click(await screen.findByRole("button", { name: /Priya Shah/ }));
    await pickOption(
      user,
      screen.getByLabelText("Expiry"),
      "No expiry (until revoked)",
    );
    await user.click(screen.getByTestId("share-grant-submit"));

    await waitFor(() => expect(posts).toHaveLength(2));
    // Absent, not an empty string and not epoch zero: a grant with no expiry is
    // one the wire says nothing about.
    expect(posts[1].expires_at).toBeUndefined();
  });

  it("excludes agent seats from the subject picker (spec §2.1)", async () => {
    installBaseFetch();
    render(<ShareScreen recordType="deal" recordId="d-1" />);

    // A human roster member is a selectable subject…
    await screen.findByRole("button", { name: /Priya Shah/ });
    // …but the agent seat (is_agent) is never offered as one.
    expect(screen.queryByRole("button", { name: /SDR Bot/ })).toBeNull();
    expect(screen.queryByText("SDR Bot")).toBeNull();
  });

  it("leaves a contact's owner out of the colleagues to share it with", async () => {
    installBaseFetch({ "/contacts/c-1/access": () => ownedAccess("u-1") });
    render(<ShareScreen recordType="contact" recordId="c-1" />);

    await screen.findByRole("button", { name: PICK_MOR });
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: /Priya Shah/ })).toBeNull(),
    );
  });

  it("offers no colleague until the contact's owner is known", async () => {
    let answerAccess: (response: Response) => void = () => {};
    installBaseFetch({
      "/contacts/c-1/access": () =>
        new Promise<Response>((resolve) => {
          answerAccess = resolve;
        }),
    });
    render(<ShareScreen recordType="contact" recordId="c-1" />);

    await screen.findByRole("button", { name: /Deal Desk/ });
    expect(screen.queryByRole("button", { name: /Priya Shah/ })).toBeNull();
    expect(screen.queryByRole("button", { name: PICK_MOR })).toBeNull();

    answerAccess(ownedAccess("u-1"));
    await screen.findByRole("button", { name: PICK_MOR });
    expect(screen.queryByRole("button", { name: /Priya Shah/ })).toBeNull();
  });

  it("offers no colleague after the owner lookup failed, until its Retry answers", async () => {
    let reads = 0;
    installBaseFetch({
      "/contacts/c-1/access": () => {
        reads += 1;
        return reads === 1
          ? jsonResponse({ code: "internal", status: 500 }, 500)
          : ownedAccess("u-1");
      },
    });
    render(<ShareScreen recordType="contact" recordId="c-1" />);
    const user = userEvent.setup();

    const retry = await screen.findByRole("button", { name: "Retry" });
    await screen.findByRole("button", { name: /Deal Desk/ });
    expect(screen.queryByRole("button", { name: PICK_MOR })).toBeNull();
    await user.click(retry);
    await screen.findByRole("button", { name: PICK_MOR });
    expect(screen.queryByRole("button", { name: /Priya Shah/ })).toBeNull();
  });

  // POST /record-grants is idempotent on
  // (record_type, record_id, subject_type, subject_id) and a re-assert
  // RESTATES the grant, so an already-granted subject is offered rather than
  // refused. The four cases below are the four things that press can mean.

  it("offers a subject who already has a grant, and says which level they hold", async () => {
    installBaseFetch();
    render(<ShareScreen recordType="deal" recordId="d-1" />);

    const row = await screen.findByRole("button", { name: PICK_MOR });
    // Selectable: the only way to change a level used to be revoke-and-share.
    // Refused in either spelling counts: `disabled` is what the row used to
    // carry, `aria-disabled` is how Button refuses a press without dropping
    // focus, and a row wearing either one is not pickable.
    expect(row.hasAttribute("disabled")).toBe(false);
    expect(row.getAttribute("aria-disabled")).toBeNull();
    // And the row says WHICH level, so the next press is not a blind
    // overwrite — "already has a grant" never said read from write.
    expect(within(row).getByText("Has read")).toBeTruthy();
  });

  it("submits an upgrade for an already-granted subject without asking first", async () => {
    const posts: Record<string, unknown>[] = [];
    installBaseFetch({ "/record-grants": grantEndpoint(posts, existingGrant) });
    render(<ShareScreen recordType="deal" recordId="d-1" />);
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: PICK_MOR }));
    await user.click(screen.getByRole("button", { name: "Write" }));
    await user.click(screen.getByTestId("share-grant-submit"));

    await waitFor(() => expect(posts).toHaveLength(1));
    expect(posts[0]).toMatchObject({ subject_id: "u-2", access: "write" });
    // Widening what a colleague can do is not the direction that needs a
    // second look, so nothing interrupts it.
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("asks before reducing a level, then submits the reduced one on confirm", async () => {
    const posts: Record<string, unknown>[] = [];
    installBaseFetch({ "/record-grants": grantEndpoint(posts, writeGrant) });
    render(<ShareScreen recordType="deal" recordId="d-1" />);
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: PICK_MOR }));
    // Picking opens the form on what they hold (write); Read is the reduction.
    await user.click(screen.getByRole("button", { name: "Read" }));
    await user.click(screen.getByTestId("share-grant-submit"));

    const dialog = await screen.findByRole("dialog");
    const asked = within(dialog).getByTestId(
      "share-downgrade-body",
    ).textContent;
    expect(asked).toContain("Mor Adler");
    expect(asked).toContain("Write");
    expect(asked).toContain("Read");
    // Nothing has been sent while the question is still open.
    expect(posts).toEqual([]);

    await user.click(
      within(dialog).getByRole("button", { name: "Reduce to Read" }),
    );

    await waitFor(() => expect(posts).toHaveLength(1));
    expect(posts[0]).toMatchObject({ subject_id: "u-2", access: "read" });
  });

  it("drops an open reduction once a refreshed access read names its colleague as owner", async () => {
    const posts: Record<string, unknown>[] = [];
    let accessReads = 0;
    installBaseFetch({
      "/record-grants": grantEndpoint(posts, writeGrant),
      "/contacts/c-1/access": () => {
        accessReads += 1;
        return ownedAccess(accessReads === 1 ? "u-1" : "u-2");
      },
    });
    render(<ShareScreen recordType="contact" recordId="c-1" />);
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: PICK_MOR }));
    await user.click(screen.getByRole("button", { name: "Read" }));
    await user.click(screen.getByTestId("share-grant-submit"));
    await screen.findByRole("dialog");
    act(() => {
      globalThis.dispatchEvent(new Event("visibilitychange"));
    });

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(posts).toEqual([]);
  });

  it("returns focus to the subject field after a reduction, whose trigger is gone", async () => {
    const posts: Record<string, unknown>[] = [];
    installBaseFetch({ "/record-grants": grantEndpoint(posts, writeGrant) });
    render(<ShareScreen recordType="deal" recordId="d-1" />);
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: PICK_MOR }));
    await user.click(screen.getByRole("button", { name: "Read" }));
    await user.click(screen.getByTestId("share-grant-submit"));

    const dialog = await screen.findByRole("dialog");
    await user.click(
      within(dialog).getByRole("button", { name: "Reduce to Read" }),
    );

    // The press that opened this dialog cleared the picker on success, so the
    // control the reader came from no longer takes focus. Landing on the body
    // would restart the surface for anyone not using a pointer.
    await waitFor(() =>
      expect(document.activeElement).toBe(
        screen.getByLabelText("Person or team"),
      ),
    );
  });

  it("sends nothing when the reduction is cancelled", async () => {
    const posts: Record<string, unknown>[] = [];
    installBaseFetch({ "/record-grants": grantEndpoint(posts, writeGrant) });
    render(<ShareScreen recordType="deal" recordId="d-1" />);
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: PICK_MOR }));
    await user.click(screen.getByRole("button", { name: "Read" }));
    await user.click(screen.getByTestId("share-grant-submit"));

    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Cancel" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(posts).toEqual([]);
  });

  it("reports that nothing changed when the re-assert restates the same grant", async () => {
    const posts: Record<string, unknown>[] = [];
    installBaseFetch({ "/record-grants": grantEndpoint(posts, existingGrant) });
    render(<ShareScreen recordType="deal" recordId="d-1" />);
    const user = userEvent.setup();

    // Picked and submitted untouched: same level, same (absent) expiry, same
    // reason. The list below looks identical afterwards, so silence here would
    // read as a change that landed.
    await user.click(await screen.findByRole("button", { name: PICK_MOR }));
    await user.click(screen.getByTestId("share-grant-submit"));

    await waitFor(() => expect(posts).toHaveLength(1));
    expect(posts[0]).toMatchObject({ access: "read" });
    const notice = await screen.findByTestId("share-unchanged");
    expect(notice.textContent).toContain("Nothing changed");
    expect(notice.textContent).toContain("Mor Adler");
  });

  it("shows only the limited notice when re-saving a write share the colleague's role cannot use", async () => {
    const posts: Record<string, unknown>[] = [];
    const listed = grantEndpoint(posts, writeGrant);
    installBaseFetch({
      "/record-grants": async (request) => {
        const answer = await listed(request);
        if (request.method !== "POST") {
          return answer;
        }
        const saved = await answer.json();
        return jsonResponse(
          { ...saved, access: "read", limited_by_role: true },
          201,
        );
      },
    });
    render(<ShareScreen recordType="deal" recordId="d-1" />);
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: PICK_MOR }));
    await user.click(screen.getByTestId("share-grant-submit"));

    await waitFor(() => expect(posts).toHaveLength(1));
    expect(posts[0]).toMatchObject({ access: "write" });
    expect(
      (await screen.findByTestId("share-limited-by-role")).textContent,
    ).toContain("Mor Adler");
    expect(screen.queryByTestId("share-unchanged")).toBeNull();
  });

  it("names the recipient's seat for a 403 seat_tier_insufficient", async () => {
    installBaseFetch({
      "/record-grants": (request) => {
        if (request.method === "POST") {
          return jsonResponse(
            {
              type: "about:blank",
              title: "Forbidden",
              status: 403,
              code: "seat_tier_insufficient",
              detail: "seat tier does not admit this action",
            },
            403,
          );
        }
        return jsonResponse({
          data: [existingGrant],
          page: { next_cursor: null, has_more: false },
        });
      },
    });
    render(<ShareScreen recordType="deal" recordId="d-1" />);
    const user = userEvent.setup();

    await user.click(await screen.findByRole("button", { name: /Priya Shah/ }));
    await user.click(screen.getByRole("button", { name: "Write" }));
    await user.click(screen.getByTestId("share-grant-submit"));

    // The seat ceiling binds the RECIPIENT's licence, not the actor's
    // permission, and the server's own wording sends a reader looking at
    // their own role instead.
    expect(
      await screen.findByText(/read-only seat cannot hold write access/),
    ).toBeTruthy();
    expect(
      screen.queryByText("seat tier does not admit this action"),
    ).toBeNull();
  });

  async function confirmRevoke() {
    await userEvent.click(await screen.findByTestId("revoke-grant"));
    const dialog = await screen.findByRole("dialog");
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Revoke" }),
    );
    return dialog;
  }

  // Every row has a Revoke, so the button and its question name whose access
  // to which record a press ends.
  it("names the member and the record on Revoke and in its question", async () => {
    installBaseFetch();
    render(<ShareScreen recordType="deal" recordId="d-1" />);
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("button", {
        name: "Revoke access to Acme renewal for Mor Adler",
      }),
    );

    expect((await screen.findByRole("dialog")).textContent).toContain(
      "Revoke Read access to Acme renewal for Mor Adler?",
    );
  });

  // A colleague who left is not on the live roster, and is still named by id.
  it("names a grantee the roster does not list, on Revoke and in its question", async () => {
    const departed = { ...existingGrant, subject_id: "u-gone" };
    installBaseFetch({
      "/users/names": () =>
        jsonResponse({
          data: [
            { id: "u-gone", display_name: "Lee Former", departed: true },
            { id: "u-1", display_name: "Priya Shah" },
          ],
        }),
      "/record-grants": () =>
        jsonResponse({
          data: [departed],
          page: { next_cursor: null, has_more: false },
        }),
    });
    render(<ShareScreen recordType="deal" recordId="d-1" />);
    const user = userEvent.setup();

    await user.click(
      await screen.findByRole("button", {
        name: "Revoke access to Acme renewal for Lee Former (left)",
      }),
    );

    expect((await screen.findByRole("dialog")).textContent).toContain(
      "Revoke Read access to Acme renewal for Lee Former (left)?",
    );
  });

  it("confirms a grant by naming who can now see the record", async () => {
    const posts: Record<string, unknown>[] = [];
    installBaseFetch({ "/record-grants": grantEndpoint(posts, existingGrant) });
    render(<ShareScreen recordType="deal" recordId="d-1" />);
    const user = userEvent.setup();
    await screen.findByRole("button", {
      name: "Revoke access to Acme renewal for Mor Adler",
    });

    await user.click(await screen.findByRole("button", { name: /Priya Shah/ }));
    await user.click(screen.getByTestId("share-grant-submit"));

    expect(
      await screen.findByText(
        "Priya Shah now has Read access to Acme renewal.",
      ),
    ).toBeTruthy();
    expect(posts).toHaveLength(1);
  });

  it("leads back to the record it shares", async () => {
    installBaseFetch();
    render(<ShareScreen recordType="deal" recordId="d-1" />);

    const back = await screen.findByRole("link", {
      name: "Back to Acme renewal",
    });
    expect(back.getAttribute("href")).toMatch(/deals\/d-1$/);
  });

  it("labels a share's end date and writes it the way the record page does", async () => {
    const ending = { ...existingGrant, expires_at: "2026-11-23T12:00:00Z" };
    installBaseFetch({
      "/record-grants": () =>
        jsonResponse({
          data: [ending],
          page: { next_cursor: null, has_more: false },
        }),
    });
    render(<ShareScreen recordType="deal" recordId="d-1" />);

    const aclList = await screen.findByTestId("share-acl-list");
    expect(within(aclList).getByText(/^Until /).textContent).toBe(
      "Until 23 Nov 2026",
    );
  });

  // A reason is free text: one long word has to break inside its row.
  it("lets a long reason break inside its row", async () => {
    installBaseFetch();
    render(<ShareScreen recordType="deal" recordId="d-1" />);

    const reason = await screen.findByText("compliance review");
    expect(reason.classList.contains("share-acl-reason")).toBe(true);
    // jsdom loads no stylesheet, so the rule is read off the file.
    const sheet = readFileSync(
      join(dirname(fileURLToPath(import.meta.url)), "share.css"),
      "utf8",
    );
    expect(sheet).toMatch(
      /\.share-acl-reason\s*\{[^}]*overflow-wrap\s*:\s*anywhere\s*[;}]/,
    );
  });

  it("says a search matched nobody, not that there is nobody to share with", async () => {
    installBaseFetch();
    render(<ShareScreen recordType="deal" recordId="d-1" />);
    const user = userEvent.setup();
    await screen.findByRole("button", { name: /Priya Shah/ });

    await user.type(screen.getByLabelText("Person or team"), "zzzz");

    expect(screen.getByTestId("share-roster-empty").textContent).toBe(
      "No member or team matches your search.",
    );
  });

  it("revoke on a row, confirmed, fires DELETE /record-grants/{id} behind the version it showed", async () => {
    let deletedIfMatch: string | null = null;
    installBaseFetch({
      "/record-grants/g-1": (request) => {
        if (request.method === "DELETE") {
          deletedIfMatch = request.headers.get("If-Match");
          return new Response(null, { status: 204 });
        }
        return jsonResponse({}, 404);
      },
    });
    render(<ShareScreen recordType="deal" recordId="d-1" />);

    await confirmRevoke();

    await waitFor(() => expect(deletedIfMatch).toBe("1"));
  });

  it("a revoke refused as version_skew says the access changed and re-reads the list", async () => {
    let listReads = 0;
    installBaseFetch({
      "/record-grants/g-1": () =>
        jsonResponse(
          { title: "Conflict", status: 409, code: "version_skew" },
          409,
        ),
      "/record-grants": () => {
        listReads += 1;
        return jsonResponse({
          data: [existingGrant],
          page: { next_cursor: null, has_more: false },
        });
      },
    });
    render(<ShareScreen recordType="deal" recordId="d-1" />);
    await screen.findByTestId("revoke-grant");
    const readsBefore = listReads;

    const dialog = await confirmRevoke();

    expect(
      await within(dialog).findByText(
        /changed this access after you opened it/,
      ),
    ).toBeTruthy();
    await waitFor(() => expect(listReads).toBeGreaterThan(readsBefore));
  });

  it("a revoke confirmed again after version_skew carries the version the list re-read", async () => {
    const ifMatches: (string | null)[] = [];
    let refreshed = false;
    installBaseFetch({
      "/record-grants/g-1": (request) => {
        ifMatches.push(request.headers.get("If-Match"));
        if (ifMatches.length === 1) {
          refreshed = true;
          return jsonResponse(
            { title: "Conflict", status: 409, code: "version_skew" },
            409,
          );
        }
        return new Response(null, { status: 204 });
      },
      "/record-grants": () =>
        jsonResponse({
          data: [{ ...existingGrant, version: refreshed ? 2 : 1 }],
          page: { next_cursor: null, has_more: false },
        }),
    });
    render(<ShareScreen recordType="deal" recordId="d-1" />);

    const dialog = await confirmRevoke();
    await within(dialog).findByText(/changed this access after you opened it/);
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Revoke" }),
    );

    await waitFor(() => expect(ifMatches).toEqual(["1", "2"]));
  });

  it("renders honest copy (not a raw string) for a 403 approval_required grant response", async () => {
    installBaseFetch({
      "/record-grants": (request) => {
        if (request.method === "POST") {
          return jsonResponse(
            {
              type: "about:blank",
              title: "Forbidden",
              status: 403,
              code: "approval_required",
              detail: "queued behind the approval gate",
            },
            403,
          );
        }
        return jsonResponse({
          data: [existingGrant],
          page: { next_cursor: null, has_more: false },
        });
      },
    });
    render(<ShareScreen recordType="deal" recordId="d-1" />);

    const pick = await screen.findByRole("button", { name: /Priya Shah/ });
    await userEvent.click(pick);
    await userEvent.click(screen.getByTestId("share-grant-submit"));

    await waitFor(() =>
      expect(screen.queryByText(/approval_required/)).toBeNull(),
    );
    expect(await screen.findByText(/held for approval|approval/i)).toBeTruthy();
  });

  it("renders honest copy (not a raw string) for a 422 validation error", async () => {
    installBaseFetch({
      "/record-grants": (request) => {
        if (request.method === "POST") {
          return jsonResponse(
            {
              type: "about:blank",
              title: "Unprocessable",
              status: 422,
              code: "validation_error",
              detail: "expires_at must be in the future",
            },
            422,
          );
        }
        return jsonResponse({
          data: [existingGrant],
          page: { next_cursor: null, has_more: false },
        });
      },
    });
    render(<ShareScreen recordType="deal" recordId="d-1" />);

    const pick = await screen.findByRole("button", { name: /Priya Shah/ });
    await userEvent.click(pick);
    await userEvent.click(screen.getByTestId("share-grant-submit"));

    expect(
      await screen.findByText("expires_at must be in the future"),
    ).toBeTruthy();
    expect(screen.queryByText("[object Object]")).toBeNull();
  });

  it("a project is a shareable record: the share form mounts rather than the unknown-kind plate", async () => {
    installBaseFetch();
    render(<ShareScreen recordType="project" recordId="pr-1" />);

    expect(await screen.findByTestId("share-acl-list")).toBeTruthy();
    expect(screen.queryByText("This record cannot be shared.")).toBeNull();
  });
});

// Sharing asks the record's own write authority (EnsureCanGrant), the answer
// the server gives as `writable`. A reader without it sees who can see the
// record and why they cannot change that, and no verb the server would refuse.
describe("ShareScreen for a reader who may not share", () => {
  const viewer = meFixture(SHARER).user?.id ?? "";

  it("offers no grant and no revoke on a record the reader may not change", async () => {
    installBaseFetch({
      "/v1/deals/d-1": () => jsonResponse({ id: "d-1", writable: false }),
      "/record-grants": () =>
        jsonResponse({
          data: [
            existingGrant,
            { ...existingGrant, id: "g-own", subject_id: viewer },
          ],
          page: { next_cursor: null, has_more: false },
        }),
    });
    render(<ShareScreen recordType="deal" recordId="d-1" />);

    expect(await screen.findByText(en["deal.notYoursToChange"])).toBeTruthy();
    expect(
      screen.queryByRole("heading", { name: en["share.grantAccess"] }),
    ).toBeNull();
    expect(screen.queryByTestId("share-grant-submit")).toBeNull();
    // The one share such a reader may take away is the one made to them.
    await waitFor(() =>
      expect(screen.getAllByTestId("revoke-grant")).toHaveLength(1),
    );
  });

  // Revoking asks the record's write authority on a row that need not be live
  // (grants.go mayRevoke). An archived record keeps its Revoke for a seat with
  // the update grant, while sharing anew stays refused.
  it("keeps Revoke on an archived record for a seat that may change it", async () => {
    installBaseFetch({
      "/v1/deals/d-1": () =>
        jsonResponse({
          id: "d-1",
          writable: false,
          archived_at: "2026-01-02T00:00:00Z",
        }),
    });
    render(<ShareScreen recordType="deal" recordId="d-1" />);

    expect(await screen.findByText(en["deal.archivedReadOnly"])).toBeTruthy();
    expect(screen.queryByTestId("share-grant-submit")).toBeNull();
    expect(await screen.findByTestId("revoke-grant")).toBeTruthy();
  });

  // A /me that failed leaves every grant denied: the screen shows that rather
  // than waiting, and the server decides any write.
  it("does not wait forever when /me fails", async () => {
    installBaseFetch({
      "/v1/me": () => jsonResponse({ title: "Forbidden", status: 403 }, 403),
    });
    render(<ShareScreen recordType="deal" recordId="d-1" />);

    expect(await screen.findByText(en["deal.notYoursToChange"])).toBeTruthy();
    expect(await screen.findByTestId("share-acl-list")).toBeTruthy();
  });

  it("says a record the reader cannot see is not there, and offers nothing", async () => {
    installBaseFetch({
      "/v1/contacts/c-9": () =>
        jsonResponse({ title: "Not Found", status: 404 }, 404),
    });
    render(<ShareScreen recordType="contact" recordId="c-9" />);

    expect(await screen.findByText(en["share.missingRecord"])).toBeTruthy();
    expect(screen.queryByText("c-9")).toBeNull();
    expect(screen.queryByTestId("share-grant-submit")).toBeNull();
  });
});

describe("the subject a Grant press sends", () => {
  const priya: RosterSubject = {
    id: "u-1",
    name: "Priya Shah",
    note: "",
    kind: "user",
  };
  const desk: RosterSubject = {
    id: "t-1",
    name: "Deal Desk",
    note: "",
    kind: "team",
  };

  it("is dropped once the access answer names the picked colleague as owner", () => {
    expect(pickedToGrant(priya, "u-1")).toBeNull();
  });

  it("is no colleague while the owner is not known yet", () => {
    expect(pickedToGrant(priya, undefined)).toBeNull();
  });

  it("keeps a team, and a colleague on a record with no owner", () => {
    expect(pickedToGrant(desk, undefined)).toBe(desk);
    expect(pickedToGrant(priya, null)).toBe(priya);
  });
});
