/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import { RequestsPanel } from "../settings/settings.privacy";
import {
  choose,
  DSRS,
  draftClose,
  findDsrRow,
  jsonResponse,
  render,
  stubRoutes,
} from "./privacy.testkit";

beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe("opening a DSR (G-2)", () => {
  // An erasure fulfils by resolving subject_ref to a contact id. The server
  // refuses free text there (BE-2), so the form must not offer it.
  it("requires a picked contact for an erasure", async () => {
    stubRoutes();
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /new request/i }),
    );
    await choose(screen.getByLabelText(/kind/i), "Erasure");
    expect(screen.getByLabelText(/contact/i)).toBeInTheDocument();
    expect(
      screen.queryByLabelText(/subject reference/i),
    ).not.toBeInTheDocument();
  });

  it("allows a free-text subject for an access request — the subject may not be in the CRM", async () => {
    stubRoutes();
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /new request/i }),
    );
    await choose(screen.getByLabelText(/kind/i), "Access");
    expect(screen.getByLabelText(/subject reference/i)).toBeInTheDocument();
  });

  it("says an access request is fulfilled by hand — nothing is exported", async () => {
    stubRoutes();
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /new request/i }),
    );
    await choose(screen.getByLabelText(/kind/i), "Access");
    expect(screen.getByText(/fulfilled manually/i)).toBeInTheDocument();
  });

  // The erasure fulfiller resolves subject_ref to a contact id (BE-2). So an
  // erasure request must never name a subject the server cannot erase. The
  // form holds this only by its shape: a RecordPicker and no text input.
  // This proves the picked contact's uuid reaches the wire, not its name.
  it("sends the picked contact's uuid as subject_ref for an erasure request", async () => {
    // Pinned to a zone behind UTC, since the suite never controls the host's
    // own zone. due_at must mint at the end of the day in that zone, not at
    // UTC midnight. The two disagree on which calendar day Aug 1 is.
    vi.spyOn(Intl.DateTimeFormat.prototype, "resolvedOptions").mockReturnValue({
      timeZone: "America/New_York",
    } as Intl.ResolvedDateTimeFormatOptions);
    const sent = stubRoutes({
      "GET /contacts": () =>
        jsonResponse({
          data: [
            {
              id: "3fa85f64-5717-4562-b3fc-2c963f66afa6",
              full_name: "Anna Weber",
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      "POST /data-subject-requests": () =>
        jsonResponse(
          {
            id: "d3",
            kind: "erasure",
            subject_ref: "3fa85f64-5717-4562-b3fc-2c963f66afa6",
            status: "open",
            due_at: "2026-08-02T03:59:59.999Z",
            due_overridden: false,
            dispute_open: false,
            created_at: "2026-07-15T00:00:00Z",
          },
          201,
        ),
    });
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /new request/i }),
    );
    await choose(screen.getByLabelText(/kind/i), "Erasure");
    await userEvent.type(screen.getByLabelText(/contact/i), "anna");
    await userEvent.click(await screen.findByText("Anna Weber"));
    await screen.findByText(/GDPR, one calendar month/);
    await userEvent.click(
      screen.getByRole("button", { name: /open request/i }),
    );
    await waitFor(() =>
      expect(
        sent.filter((s) => s.key === "POST /data-subject-requests"),
      ).toHaveLength(1),
    );
    const posts = sent.filter((s) => s.key === "POST /data-subject-requests");
    // toEqual, not objectContaining: subject_ref is the picked uuid, never
    // "Anna Weber", and no due date rode along, because the server computes it.
    expect(posts[0]?.body).toEqual({
      kind: "erasure",
      subject_ref: "3fa85f64-5717-4562-b3fc-2c963f66afa6",
    });
  });

  // The other side of the test above: access and rectify keep the free-text
  // field. This proves the two kinds differ on the wire. A shared code path
  // must not reuse the contact picker's value.
  it("sends the typed free text as subject_ref for an access request", async () => {
    vi.spyOn(Intl.DateTimeFormat.prototype, "resolvedOptions").mockReturnValue({
      timeZone: "America/New_York",
    } as Intl.ResolvedDateTimeFormatOptions);
    const sent = stubRoutes({
      "POST /data-subject-requests": () =>
        jsonResponse(
          {
            id: "d4",
            kind: "access",
            subject_ref: "anna@acme.test",
            status: "open",
            due_at: "2026-08-02T03:59:59.999Z",
            due_overridden: false,
            dispute_open: false,
            created_at: "2026-07-15T00:00:00Z",
          },
          201,
        ),
    });
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /new request/i }),
    );
    await choose(screen.getByLabelText(/kind/i), "Access");
    await userEvent.type(
      screen.getByLabelText(/subject reference/i),
      "anna@acme.test",
    );
    await screen.findByText(/GDPR, one calendar month/);
    await userEvent.click(
      screen.getByRole("button", { name: /open request/i }),
    );
    await waitFor(() =>
      expect(
        sent.filter((s) => s.key === "POST /data-subject-requests"),
      ).toHaveLength(1),
    );
    const posts = sent.filter((s) => s.key === "POST /data-subject-requests");
    expect(posts[0]?.body).toEqual({
      kind: "access",
      subject_ref: "anna@acme.test",
    });
  });

  // I3: minting and rendering must agree, in a zone where the old
  // UTC-midnight minting silently rolled the picked day back by one. Picking
  // 15 Jul mints the UTC instant for 23:59:59.999 New York time (16 Jul UTC);
  // the row must still read the date back as 15 Jul, not 14.
  it("mints an earlier due date at end-of-day in the viewer's zone, matching what the row later shows", async () => {
    vi.spyOn(Intl.DateTimeFormat.prototype, "resolvedOptions").mockReturnValue({
      timeZone: "America/New_York",
    } as Intl.ResolvedDateTimeFormatOptions);
    const created = {
      id: "d5",
      kind: "access",
      subject_ref: "anna@acme.test",
      status: "open",
      due_at: "2026-07-16T03:59:59.999Z",
      due_overridden: false,
      dispute_open: false,
      created_at: "2026-07-15T00:00:00Z",
    };
    const sent = stubRoutes({
      "POST /data-subject-requests": () => jsonResponse(created, 201),
      "GET /data-subject-requests": () =>
        jsonResponse({
          data: [created],
          page: { next_cursor: null, has_more: false },
        }),
    });
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /new request/i }),
    );
    await choose(screen.getByLabelText(/kind/i), "Access");
    await userEvent.type(
      screen.getByLabelText(/subject reference/i),
      "anna@acme.test",
    );
    await screen.findByText(/GDPR, one calendar month/);
    await userEvent.type(
      screen.getByRole("textbox", { name: /earlier deadline/i }),
      "15/07/2026",
    );
    await userEvent.click(
      screen.getByRole("button", { name: /open request/i }),
    );
    await waitFor(() =>
      expect(
        sent.filter((s) => s.key === "POST /data-subject-requests"),
      ).toHaveLength(1),
    );
    const posts = sent.filter((s) => s.key === "POST /data-subject-requests");
    expect(posts[0]?.body).toEqual({
      kind: "access",
      subject_ref: "anna@acme.test",
      due_at: "2026-07-16T03:59:59.999Z",
    });
    expect(await screen.findByText(/15\/07\/2026/)).toBeInTheDocument();
    expect(screen.queryByText(/14\/07\/2026/)).not.toBeInTheDocument();
  });
});

describe("fulfilling an erasure", () => {
  // The whole label, so a dialog that stops showing the word fails here.
  function erasePhraseInput() {
    return screen.getByLabelText(`${en["privacy.typeErasePrompt"]} ERASE`);
  }

  it("holds the confirm until ERASE is typed", async () => {
    stubRoutes();
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /8f3a-contact-uuid/i }),
    );
    await draftClose("verified");
    // "Fulfil" also matches the facet bar's "Fulfilled" filter button. So
    // the query is scoped to the row under test, as findDsrRow's other
    // callers do.
    const row = await findDsrRow("8f3a-contact-uuid");
    await userEvent.click(within(row).getByRole("button", { name: /fulfil/i }));
    const confirm = await screen.findByRole("button", {
      name: /erase and suppress/i,
    });
    expect(confirm).toBeDisabled();
    await waitFor(() => expect(erasePhraseInput()).toHaveFocus());
    // Case-sensitive, like every typed confirmation: the word is shown in bold.
    await userEvent.type(erasePhraseInput(), "erase");
    expect(confirm).toBeDisabled();
    await userEvent.clear(erasePhraseInput());
    await userEvent.type(erasePhraseInput(), " ERASE ");
    expect(confirm).toBeEnabled();
    // The record's drawer stays open under the confirm, so scope to the confirm.
    const confirmDialog = confirm.closest("[role='dialog']");
    if (!(confirmDialog instanceof HTMLElement)) {
      throw new Error("the erase confirm is not inside a dialog");
    }
    expect(within(confirmDialog).getByRole("status")).toHaveTextContent(
      en["confirmPhrase.matches"],
    );
  });

  // gobd.html: retention wins for the statutory window (Art. 17(3)(b)). The
  // 409 is not a generic error; it is a documented outcome. The stub mirrors
  // the real wire shape: erasure.go wraps ErrConflict, and httperr.go maps it
  // by its fixed table. It has no retain_until. The legal-hold check is a
  // plain boolean column, not a retention timestamp, so the fixture has none.
  it("renders a legal hold as a blocked state, not a red toast", async () => {
    stubRoutes({
      "PATCH /data-subject-requests/d1": () =>
        jsonResponse(
          {
            type: "https://errors.gradion.com/conflict",
            title: "Conflict",
            status: 409,
            code: "conflict",
            detail: "erasing a contact under legal hold: conflict",
          },
          409,
        ),
    });
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /8f3a-contact-uuid/i }),
    );
    await draftClose("verified");
    const row = await findDsrRow("8f3a-contact-uuid");
    await userEvent.click(within(row).getByRole("button", { name: /fulfil/i }));
    await userEvent.type(erasePhraseInput(), "ERASE");
    const confirm = screen.getByRole("button", { name: /erase and suppress/i });
    await userEvent.click(confirm);
    expect(await screen.findByText(/legal hold/i)).toBeInTheDocument();
    expect(screen.getByText(/no override/i)).toBeInTheDocument();
    // The phrase still matches, and the hold keeps the confirm shut anyway.
    expect(confirm).toBeDisabled();
  });

  // Unlike the legal hold above, a stale-transition 422 is not a lawful
  // refusal of this confirm. It means someone else already decided the
  // request, so the same confirm could only get a 422 again. The modal must
  // disarm itself and the row list behind it must re-read. DsrRow's own
  // plain PATCH does the same for this race.
  it("disarms the confirm and re-reads when the fulfil moved on underneath us", async () => {
    const sent = stubRoutes({
      "PATCH /data-subject-requests/d1": () =>
        jsonResponse(
          {
            title: "Unprocessable Entity",
            detail: "open → fulfilled is not a legal transition",
            status: 422,
            code: "validation_error",
            details: {
              errors: [
                {
                  field: "status",
                  code: "invalid",
                  message: "open → fulfilled is not a legal transition",
                },
              ],
            },
          },
          422,
        ),
    });
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /8f3a-contact-uuid/i }),
    );
    await draftClose("verified");
    const row = await findDsrRow("8f3a-contact-uuid");
    await userEvent.click(within(row).getByRole("button", { name: /fulfil/i }));
    await userEvent.type(erasePhraseInput(), "ERASE");
    const confirm = screen.getByRole("button", { name: /erase and suppress/i });
    await userEvent.click(confirm);

    expect(await screen.findByText(/else decided this/i)).toBeInTheDocument();
    expect(confirm).toBeDisabled();
    // The refusal must cause the same re-read as the row's own PATCH race:
    // a second GET after the first page load. Then the row behind this modal
    // shows what the server did.
    await waitFor(() =>
      expect(
        sent.filter((s) => s.key === "GET /data-subject-requests").length,
      ).toBeGreaterThan(1),
    );
  });

  // The confirm modal's own mutation, not the row's plain PATCH above.
  // submitTransition never reaches it for an erasure fulfil. The server
  // checks a DSR update before it erases, and an erasure fulfil without a
  // resolution gets a 422. So the resolution the operator wrote in the row
  // must go into this PATCH too. The modal has no field to ask for it again.
  // The stub echoes what the server would store. An open fixture with no
  // resolution cannot get a 200 for a fulfil that sent none. So the response
  // includes the resolution just sent.
  it("sends {status: fulfilled, resolution, identity_check} on the erasure fulfil PATCH", async () => {
    const sent = stubRoutes({
      "PATCH /data-subject-requests/d1": () =>
        jsonResponse({
          ...DSRS.data[0],
          status: "fulfilled",
          resolution: "verified",
        }),
    });
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /8f3a-contact-uuid/i }),
    );
    await draftClose("verified");
    const row = await findDsrRow("8f3a-contact-uuid");
    await userEvent.click(within(row).getByRole("button", { name: /fulfil/i }));
    await userEvent.type(erasePhraseInput(), "ERASE");
    await userEvent.click(
      screen.getByRole("button", { name: /erase and suppress/i }),
    );
    await waitFor(() =>
      expect(
        sent.filter((s) => s.key === "PATCH /data-subject-requests/d1"),
      ).toHaveLength(1),
    );
    const patches = sent.filter(
      (s) => s.key === "PATCH /data-subject-requests/d1",
    );
    expect(patches[0]?.body).toEqual({
      status: "fulfilled",
      resolution: "verified",
      identity_check: "id_document",
    });
  });

  // A fulfilled request is terminal, so the verb the confirm was opened from is
  // gone once it succeeds. Focus lands on the drawer's title instead of <body>.
  it("returns focus to the request's title after the fulfil, never to the document", async () => {
    let fulfilled = false;
    const closed = { status: "fulfilled", resolution: "verified" };
    stubRoutes({
      // The queue the server answers with afterwards. Without it the row comes
      // back open and its button can take focus back. Then the case under test
      // never happens.
      "GET /data-subject-requests": () =>
        jsonResponse({
          ...DSRS,
          data: DSRS.data.map((dsr) =>
            dsr.id === "d1" && fulfilled ? { ...dsr, ...closed } : dsr,
          ),
        }),
      "PATCH /data-subject-requests/d1": () => {
        fulfilled = true;
        return jsonResponse({ ...DSRS.data[0], ...closed });
      },
    });
    render(<RequestsPanel />);
    const summary = await screen.findByRole("button", {
      name: /8f3a-contact-uuid/i,
    });
    await userEvent.click(summary);
    await draftClose("verified");
    const row = await findDsrRow("8f3a-contact-uuid");
    const opener = within(row).getByRole("button", { name: /fulfil/i });
    await userEvent.click(opener);
    await userEvent.type(erasePhraseInput(), "ERASE");
    await userEvent.click(
      screen.getByRole("button", { name: /erase and suppress/i }),
    );

    await waitFor(() => expect(opener.isConnected).toBe(false));
    const drawer = screen.getByRole("dialog", { name: "8f3a-contact-uuid" });
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(drawer).getByRole("heading", { name: "8f3a-contact-uuid" }),
      ),
    );
    expect(within(drawer).getByText("Fulfilled")).toBeInTheDocument();
  });
});

describe("the proof a closed request keeps", () => {
  it("sends the requester's address with the close", async () => {
    const sent = stubRoutes();
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /8f3a-contact-uuid/i }),
    );
    await draftClose("not a data subject");
    await userEvent.type(
      screen.getByLabelText(en["privacy.requesterEmail"]),
      "asker@acme.test",
    );
    const row = await findDsrRow("8f3a-contact-uuid");
    await userEvent.click(within(row).getByRole("button", { name: /reject/i }));
    await waitFor(() =>
      expect(
        sent.find((s) => s.key === "PATCH /data-subject-requests/d1")?.body,
      ).toEqual({
        status: "rejected",
        resolution: "not a data subject",
        identity_check: "id_document",
        requester_email: "asker@acme.test",
      }),
    );
  });

  it("offers only Reject for a request whose identity could not be checked", async () => {
    stubRoutes();
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /8f3a-contact-uuid/i }),
    );
    await userEvent.type(
      screen.getByLabelText(/resolution/i),
      "unknown sender",
    );
    await choose(
      screen.getByLabelText(en["privacy.identityCheck"]),
      en["privacy.identityCheck.not_verified"],
    );
    const row = await findDsrRow("8f3a-contact-uuid");
    expect(within(row).getByRole("button", { name: /fulfil/i })).toBeDisabled();
    expect(within(row).getByRole("button", { name: /reject/i })).toBeEnabled();
  });

  it("records a complaint on a closed request, which holds its proof", async () => {
    const sent = stubRoutes();
    render(<RequestsPanel />);
    await userEvent.click(
      await screen.findByRole("button", { name: /anna@acme\.test/i }),
    );
    await userEvent.click(
      screen.getByRole("button", { name: en["privacy.openDispute"] }),
    );
    await waitFor(() =>
      expect(
        sent.find((s) => s.key === "PATCH /data-subject-requests/d2")?.body,
      ).toEqual({ dispute_open: true }),
    );
  });
});
