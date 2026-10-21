/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { Dispatch } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../../i18n";
import { en } from "../../../i18n/en";
import { jsonResponse, stubWithSession } from "../../../testing/story-utils";
import { CompanyAct } from "./company-act";
import {
  type FieldFixture,
  proposedField,
  REVIEW_READ,
  REVIEW_READ_ID,
  REVIEW_STATE,
  reviewProposal,
  toColdField,
} from "./company-act.testkit";
import type { ConversationEvent } from "./conversation-machine";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// A confirm submits every required field filled and nothing else in the
// server's way. In that shape, one of the three documented 409s is worth
// reacting to.
const CONFIRM_FIELDS: readonly FieldFixture[] = [
  proposedField("display_name", "Acme Inc", 0.95),
  proposedField("offer_summary", "CRM software", 0.9),
  proposedField("icp", "Mid-market B2B", 0.9),
];

const CONFIRM_PATH = `POST /company/site-reads/${REVIEW_READ_ID}/confirm`;

// The read has moved on to a new draft while the review sat on the old one.
// A confirm quotes this version pair, so a proposal still answering for the
// previous draft is the stale one.
const MOVED_DRAFT = { draft_version: 2, proposal_hash: "proposal-2" } as const;

// The sentence a refused confirm reads as, one per code the server documents
// for this operation (crm.yaml, confirmCompanySiteRead). Either the read has
// no draft to confirm, or it was confirmed already but its company could not
// be loaded. Read from the catalog rather than transcribed: these cases are
// about which notice appears, and a copy edit should not break the test.
const NOT_READY_NOTICE = en["ob.conv.review.confirmNotReady"];

const CHECK_FAILED_NOTICE = en["ob.conv.review.confirmCheckFailed"];

// The review, rendered from the one state every rejection case starts in:
// only the stubbed routes and the code the confirm comes back with differ.
function renderConfirmReview(
  dispatch: Dispatch<ConversationEvent> = vi.fn(),
): void {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <CompanyAct
          state={REVIEW_STATE}
          dispatch={dispatch}
          profile={null}
          persist={vi.fn(async () => true)}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("recovering from a rejected confirm", () => {
  it("blocks a retry until the refetched proposal actually changed, never the one the server just rejected", async () => {
    let proposalCalls = 0;
    // The refetch this driver starts on a version-skew rejection is held
    // open, so the still-disabled window is observable here. A mocked fetch
    // would otherwise settle before the assertion runs.
    // Boxed rather than a bare `let`: TypeScript narrows a variable only
    // assigned inside a nested closure to `never` at the call site.
    const gate: { release: (() => void) | null } = { release: null };
    stubWithSession(
      {
        [`GET /company/site-reads/${REVIEW_READ_ID}`]: () =>
          jsonResponse({
            ...REVIEW_READ,
            profile_fields: CONFIRM_FIELDS.map(toColdField),
          }),
        "GET /onboarding/company/proposal": async () => {
          proposalCalls += 1;
          if (proposalCalls > 1) {
            await new Promise<void>((resolve) => {
              gate.release = resolve;
            });
          }
          const hash = proposalCalls === 1 ? "proposal-1" : "proposal-2";
          return jsonResponse({
            ...reviewProposal(CONFIRM_FIELDS),
            proposal_hash: hash,
          });
        },
        [CONFIRM_PATH]: () =>
          jsonResponse({ title: "conflict", code: "version_skew" }, 409),
      },
      {},
    );
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider initial="en">
          <CompanyAct
            state={REVIEW_STATE}
            dispatch={vi.fn()}
            profile={null}
            persist={vi.fn(async () => true)}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    const continueButton = await screen.findByRole("button", {
      name: "Confirm profile",
    });
    expect(continueButton).toBeEnabled();

    fireEvent.click(continueButton);

    // The version-skew notice names what happened. The button disables while
    // the refetch it triggered still carries the same hash the server just
    // rejected (react-query holds prior data steady in flight).
    await screen.findByText(en["ob.conv.review.confirmVersionSkew"]);
    expect(continueButton).toBeDisabled();
    await vi.waitFor(() => expect(proposalCalls).toBeGreaterThan(1));
    // The refetch is in flight but has not resolved: the button stays
    // disabled on the still-stale hash. A refetch attempt alone never re-arms it.
    expect(continueButton).toBeDisabled();

    // Once the refetch lands a new hash, the block lifts on its own. Nothing
    // else has to happen for Continue to be safe to press.
    expect(gate.release).not.toBeNull();
    gate.release?.();
    await vi.waitFor(() => expect(continueButton).toBeEnabled());
  });

  it("stays blocked on the no-data path too, once the refetch settles without ever producing a hash", async () => {
    // The proposal endpoint never succeeds here, so every confirm attempt
    // falls back to `proposalFromRead(prevSnapshot.current)`. The
    // disabled-Continue guard must cover that path too, not only the one where
    // `proposal.data` carries the hash. A failed refetch is outcome (3) of the
    // three refreshAfterSkew can settle into. No new hash exists to resubmit,
    // so the block does not lift on its own. Unlike a permanent lock, the
    // reader is told so and given a retry in place of Continue.
    let siteReadCalls = 0;
    const gate: { release: (() => void) | null } = { release: null };
    stubWithSession(
      {
        [`GET /company/site-reads/${REVIEW_READ_ID}`]: async () => {
          siteReadCalls += 1;
          if (siteReadCalls > 1) {
            await new Promise<void>((resolve) => {
              gate.release = resolve;
            });
          }
          return jsonResponse({
            ...REVIEW_READ,
            profile_fields: CONFIRM_FIELDS.map(toColdField),
          });
        },
        "GET /onboarding/company/proposal": () =>
          Promise.reject(new Error("proposal endpoint unreachable")),
        [CONFIRM_PATH]: () =>
          jsonResponse({ title: "conflict", code: "version_skew" }, 409),
      },
      {},
    );
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider initial="en">
          <CompanyAct
            state={REVIEW_STATE}
            dispatch={vi.fn()}
            profile={null}
            persist={vi.fn(async () => true)}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    const continueButton = await screen.findByRole("button", {
      name: "Confirm profile",
    });
    await vi.waitFor(() => expect(continueButton).toBeEnabled());

    fireEvent.click(continueButton);

    await screen.findByText(en["ob.conv.review.confirmVersionSkew"]);
    expect(continueButton).toBeDisabled();
    await vi.waitFor(() => expect(siteReadCalls).toBeGreaterThan(1));
    // The refetch this rejection triggered is in flight, and `proposal.data`
    // never carries a hash on this path. A guard reading only that field would
    // re-arm the instant it saw `undefined`, straight after the 409.
    expect(continueButton).toBeDisabled();

    gate.release?.();
    // The refetch settled into a failure, and Continue must not re-arm onto a
    // draft the server has never seen. The dedicated notice names that, with
    // its own retry standing in for the button.
    await screen.findByText(en["ob.conv.review.confirmVersionSkewStuck"]);
    expect(continueButton).toBeDisabled();
    expect(screen.getByRole("button", { name: "Retry" })).toBeInTheDocument();
  });

  it("re-arms Continue once a skew refetch actually lands a NEW hash, whether that is the automatic one or a later manual retry", async () => {
    // The first refetch this 409 triggers lands the same hash: a concurrent
    // confirm elsewhere left the draft as it was. A guard that re-armed on
    // that alone, not on a hash that differs, would earn the identical 409 on
    // the next press. The reader's own retry moves things on, once the draft
    // underneath has changed.
    let proposalCalls = 0;
    const gate: { release: (() => void) | null } = { release: null };
    stubWithSession(
      {
        [`GET /company/site-reads/${REVIEW_READ_ID}`]: () =>
          jsonResponse({
            ...REVIEW_READ,
            profile_fields: CONFIRM_FIELDS.map(toColdField),
          }),
        "GET /onboarding/company/proposal": async () => {
          proposalCalls += 1;
          if (proposalCalls > 1) {
            await new Promise<void>((resolve) => {
              gate.release = resolve;
            });
          }
          const hash = proposalCalls <= 2 ? "proposal-1" : "proposal-2";
          return jsonResponse({
            ...reviewProposal(CONFIRM_FIELDS),
            proposal_hash: hash,
          });
        },
        [CONFIRM_PATH]: () =>
          jsonResponse({ title: "conflict", code: "version_skew" }, 409),
      },
      {},
    );
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider initial="en">
          <CompanyAct
            state={REVIEW_STATE}
            dispatch={vi.fn()}
            profile={null}
            persist={vi.fn(async () => true)}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    const continueButton = await screen.findByRole("button", {
      name: "Confirm profile",
    });
    fireEvent.click(continueButton);

    await screen.findByText(en["ob.conv.review.confirmVersionSkew"]);
    expect(continueButton).toBeDisabled();
    await vi.waitFor(() => expect(proposalCalls).toBeGreaterThan(1));
    expect(continueButton).toBeDisabled();

    gate.release?.();
    // The refetch settled with the same hash the server just rejected. The
    // block stays, and the reader is told why, with a retry of their own
    // rather than the Continue button that is still disabled.
    const retry = await screen.findByRole("button", { name: "Retry" });
    expect(continueButton).toBeDisabled();

    fireEvent.click(retry);
    await vi.waitFor(() => expect(proposalCalls).toBeGreaterThan(2));
    gate.release?.();
    // This second refetch landed a new hash. Only now may Continue re-arm,
    // onto a draft the server has not rejected.
    await vi.waitFor(() => expect(continueButton).toBeEnabled());
  });

  it("holds Continue back on a read the server refuses as not confirmable, and probes nothing to work that out", async () => {
    let readCalls = 0;
    let getCompanyCalls = 0;
    stubWithSession(
      {
        [`GET /company/site-reads/${REVIEW_READ_ID}`]: () => {
          readCalls += 1;
          return jsonResponse({
            ...REVIEW_READ,
            profile_fields: CONFIRM_FIELDS.map(toColdField),
          });
        },
        "GET /onboarding/company/proposal": () =>
          jsonResponse(reviewProposal(CONFIRM_FIELDS)),
        "GET /company": () => {
          // An existing member company from before this attempt. It is
          // present whether or not this read's own confirmation landed, so it
          // must never be read as proof that it did.
          getCompanyCalls += 1;
          return jsonResponse({
            display_name: "Some Other Existing Co",
            offer_summary: "unrelated",
            icp: "unrelated",
          });
        },
        [CONFIRM_PATH]: () =>
          jsonResponse({ title: "conflict", code: "not_confirmable" }, 409),
      },
      {},
    );
    renderConfirmReview();

    const continueButton = await screen.findByRole("button", {
      name: "Confirm profile",
    });
    fireEvent.click(continueButton);

    await screen.findByText(NOT_READY_NOTICE);
    // The server would refuse the same submission the same way, so this notice
    // takes Continue away. The re-check it offers is the only route that can
    // end differently.
    expect(continueButton).toBeDisabled();
    // The server's own code named this refusal, so nothing is re-derived
    // from a second look at the read or at the company.
    expect(readCalls).toBe(1);
    expect(getCompanyCalls).toBe(0);
  });

  it("re-arms Continue only once a re-check finds the read confirmable again", async () => {
    let readCalls = 0;
    stubWithSession(
      {
        [`GET /company/site-reads/${REVIEW_READ_ID}`]: () => {
          readCalls += 1;
          // The review was built from a confirmable snapshot; the first
          // re-check finds the read deferred (still nothing to confirm), the
          // second finds it confirmable again.
          return jsonResponse({
            ...REVIEW_READ,
            status: readCalls === 2 ? "deferred" : "ready",
            status_code: readCalls === 2 ? "budget_deferred" : null,
            profile_fields: CONFIRM_FIELDS.map(toColdField),
          });
        },
        "GET /onboarding/company/proposal": () =>
          jsonResponse(reviewProposal(CONFIRM_FIELDS)),
        [CONFIRM_PATH]: () =>
          jsonResponse({ title: "conflict", code: "not_confirmable" }, 409),
      },
      {},
    );
    renderConfirmReview();

    const continueButton = await screen.findByRole("button", {
      name: "Confirm profile",
    });
    fireEvent.click(continueButton);
    await screen.findByText(NOT_READY_NOTICE);

    const retry = await screen.findByRole("button", { name: "Retry" });
    fireEvent.click(retry);
    await vi.waitFor(() => expect(readCalls).toBe(2));
    // The re-check landed a read that still has nothing to confirm, so the
    // block stands. Re-arming here would send the identical submission back
    // into the identical 409.
    //
    // Waited on `aria-busy` rather than on the control being enabled. A
    // pending write keeps its button focusable and swallows a second press.
    // So "enabled" is true throughout, and the next click would land inside
    // the look it is meant to follow.
    await vi.waitFor(() =>
      expect(retry).not.toHaveAttribute("aria-busy", "true"),
    );
    expect(continueButton).toBeDisabled();

    fireEvent.click(retry);
    // This one found the read confirmable, the only thing that lifts the
    // block. The notice that named the block goes with it.
    await vi.waitFor(() => expect(continueButton).toBeEnabled());
    expect(screen.queryByText(NOT_READY_NOTICE)).toBeNull();
  });

  it("never re-arms Continue on a re-check that itself failed", async () => {
    let readCalls = 0;
    stubWithSession(
      {
        [`GET /company/site-reads/${REVIEW_READ_ID}`]: () => {
          readCalls += 1;
          return readCalls > 1
            ? Promise.reject(new Error("site-read endpoint unreachable"))
            : Promise.resolve(
                jsonResponse({
                  ...REVIEW_READ,
                  profile_fields: CONFIRM_FIELDS.map(toColdField),
                }),
              );
        },
        "GET /onboarding/company/proposal": () =>
          jsonResponse(reviewProposal(CONFIRM_FIELDS)),
        [CONFIRM_PATH]: () =>
          jsonResponse({ title: "conflict", code: "not_confirmable" }, 409),
      },
      {},
    );
    renderConfirmReview();

    const continueButton = await screen.findByRole("button", {
      name: "Confirm profile",
    });
    fireEvent.click(continueButton);
    await screen.findByText(NOT_READY_NOTICE);

    const retry = await screen.findByRole("button", { name: "Retry" });
    fireEvent.click(retry);
    await vi.waitFor(() => expect(readCalls).toBeGreaterThan(1));
    await vi.waitFor(() => expect(retry).toBeEnabled());
    // A failed re-check leaves the last good snapshot standing: the "ready"
    // one the server already refused. A guard reading that snapshot alone
    // would re-arm onto the identical rejection. Only a refetch that landed
    // can lift the block.
    expect(continueButton).toBeDisabled();
    expect(screen.getByText(NOT_READY_NOTICE)).toBeInTheDocument();
  });

  // The re-check refreshes the read and the proposal because the confirm
  // sends both. The server must call the read confirmable, and the press
  // quotes the version pair. The read moving on is what the reader waits for,
  // and it leaves the proposal's own pair behind.
  //
  // First, the moved read with a proposal half that failed. react-query
  // serves its last good proposal through a failure. A block lifted on the
  // read alone would re-arm Continue onto the pair from before the read
  // moved. The next press would earn a version_skew 409 and a second
  // avoidable recovery.
  it("never re-arms Continue on a re-check whose proposal half failed, however ready the read comes back", async () => {
    let proposalCalls = 0;
    let readCalls = 0;
    stubWithSession(
      {
        [`GET /company/site-reads/${REVIEW_READ_ID}`]: () => {
          readCalls += 1;
          return jsonResponse({
            ...REVIEW_READ,
            ...(readCalls > 1 ? MOVED_DRAFT : {}),
            profile_fields: CONFIRM_FIELDS.map(toColdField),
          });
        },
        "GET /onboarding/company/proposal": () => {
          proposalCalls += 1;
          return proposalCalls > 1
            ? Promise.reject(new Error("proposal endpoint unreachable"))
            : Promise.resolve(jsonResponse(reviewProposal(CONFIRM_FIELDS)));
        },
        [CONFIRM_PATH]: () =>
          jsonResponse({ title: "conflict", code: "not_confirmable" }, 409),
      },
      {},
    );
    renderConfirmReview();

    const continueButton = await screen.findByRole("button", {
      name: "Confirm profile",
    });
    fireEvent.click(continueButton);
    await screen.findByText(NOT_READY_NOTICE);

    const retry = await screen.findByRole("button", { name: "Retry" });
    fireEvent.click(retry);
    await vi.waitFor(() => expect(proposalCalls).toBeGreaterThan(1));
    await vi.waitFor(() => expect(retry).toBeEnabled());

    // The read came back ready, so half the re-check succeeded. The failed half
    // is the one the next press would quote. The block stands, and the retry
    // that can still end it stays where the reader can press it.
    expect(continueButton).toBeDisabled();
    expect(screen.getByText(NOT_READY_NOTICE)).toBeInTheDocument();
  });

  // The same moved read, with a proposal half that succeeded but answers for
  // the draft the read has left behind. A check that asks only whether the
  // request came back cannot tell this from a current one. This is the likelier
  // case: the proposal answers off its own join, so it trails the read rather
  // than failing with it.
  it("never re-arms Continue on a proposal that answered for a draft the read has moved past", async () => {
    let proposalCalls = 0;
    let readCalls = 0;
    stubWithSession(
      {
        [`GET /company/site-reads/${REVIEW_READ_ID}`]: () => {
          readCalls += 1;
          return jsonResponse({
            ...REVIEW_READ,
            ...(readCalls > 1 ? MOVED_DRAFT : {}),
            profile_fields: CONFIRM_FIELDS.map(toColdField),
          });
        },
        "GET /onboarding/company/proposal": () => {
          proposalCalls += 1;
          return jsonResponse(reviewProposal(CONFIRM_FIELDS));
        },
        [CONFIRM_PATH]: () =>
          jsonResponse({ title: "conflict", code: "not_confirmable" }, 409),
      },
      {},
    );
    renderConfirmReview();

    const continueButton = await screen.findByRole("button", {
      name: "Confirm profile",
    });
    fireEvent.click(continueButton);
    await screen.findByText(NOT_READY_NOTICE);

    const retry = await screen.findByRole("button", { name: "Retry" });
    fireEvent.click(retry);
    await vi.waitFor(() => expect(proposalCalls).toBeGreaterThan(1));
    await vi.waitFor(() => expect(retry).toBeEnabled());

    // Both halves landed, and the read is confirmable again. The pair the next
    // press would quote still names the draft the read has moved past. Releasing
    // here would trade this refusal for a version_skew on the next press.
    expect(continueButton).toBeDisabled();
    expect(screen.getByText(NOT_READY_NOTICE)).toBeInTheDocument();
  });

  // The one case with no pair to compare: no proposal was ever served. The
  // confirm quotes the refreshed read's own version pair (the mutation's
  // `proposalFromRead` fallback), which cannot be behind the read it came
  // from. Holding the block here would leave the reader pressing a re-check
  // that can never release, on a read the server calls confirmable.
  it("re-arms Continue when the proposal endpoint has never answered at all", async () => {
    let readCalls = 0;
    stubWithSession(
      {
        [`GET /company/site-reads/${REVIEW_READ_ID}`]: () => {
          readCalls += 1;
          return jsonResponse({
            ...REVIEW_READ,
            profile_fields: CONFIRM_FIELDS.map(toColdField),
          });
        },
        "GET /onboarding/company/proposal": () =>
          Promise.reject(new Error("proposal endpoint unreachable")),
        [CONFIRM_PATH]: () =>
          jsonResponse({ title: "conflict", code: "not_confirmable" }, 409),
      },
      {},
    );
    renderConfirmReview();

    const continueButton = await screen.findByRole("button", {
      name: "Confirm profile",
    });
    fireEvent.click(continueButton);
    await screen.findByText(NOT_READY_NOTICE);

    const retry = await screen.findByRole("button", { name: "Retry" });
    fireEvent.click(retry);
    await vi.waitFor(() => expect(readCalls).toBeGreaterThan(1));
    await vi.waitFor(() => expect(continueButton).toBeEnabled());
    expect(screen.queryByText(NOT_READY_NOTICE)).toBeNull();
  });

  it("walks the reader on to the company an already-confirmed read created, with no second look at the read", async () => {
    let readCalls = 0;
    const dispatch = vi.fn();
    stubWithSession(
      {
        [`GET /company/site-reads/${REVIEW_READ_ID}`]: () => {
          readCalls += 1;
          return jsonResponse({
            ...REVIEW_READ,
            profile_fields: CONFIRM_FIELDS.map(toColdField),
          });
        },
        "GET /onboarding/company/proposal": () =>
          jsonResponse(reviewProposal(CONFIRM_FIELDS)),
        "GET /company": () =>
          jsonResponse({
            display_name: "Acme Inc",
            offer_summary: "CRM software",
            icp: "Mid-market B2B",
          }),
        [CONFIRM_PATH]: () =>
          jsonResponse({ title: "conflict", code: "already_confirmed" }, 409),
      },
      {},
    );
    renderConfirmReview(dispatch);

    fireEvent.click(
      await screen.findByRole("button", { name: "Confirm profile" }),
    );

    await vi.waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "COMPANY_CONFIRMED" }),
    );
    // The server said this read was confirmed, so the only work left is to
    // load what that confirmation created. Nothing re-fetches the read to
    // establish which 409 this was.
    expect(readCalls).toBe(1);
    expect(screen.queryByText(CHECK_FAILED_NOTICE)).toBeNull();
  });

  // The already-confirmed recovery runs with no notice on screen. Its whole
  // answer is the company lookup, which either exits the review or ends in
  // the "checkFailed" notice. Continue must be out of reach for that window.
  // The server has settled this read, so a second press earns the identical
  // 409 and starts a second lookup racing the first. The generic save banner
  // must not report a save that went through.
  it("puts Continue out of reach while the already-confirmed recovery is still loading the company", async () => {
    let confirmCalls = 0;
    const gate: { release: (() => void) | null } = { release: null };
    const dispatch = vi.fn();
    stubWithSession(
      {
        [`GET /company/site-reads/${REVIEW_READ_ID}`]: () =>
          jsonResponse({
            ...REVIEW_READ,
            profile_fields: CONFIRM_FIELDS.map(toColdField),
          }),
        "GET /onboarding/company/proposal": () =>
          jsonResponse(reviewProposal(CONFIRM_FIELDS)),
        // Held open, so the in-flight window is observable here rather than
        // racing a mocked fetch that settles first.
        "GET /company": async () => {
          await new Promise<void>((resolve) => {
            gate.release = resolve;
          });
          return jsonResponse({
            display_name: "Acme Inc",
            offer_summary: "CRM software",
            icp: "Mid-market B2B",
          });
        },
        [CONFIRM_PATH]: () => {
          confirmCalls += 1;
          return jsonResponse(
            { title: "conflict", code: "already_confirmed" },
            409,
          );
        },
      },
      {},
    );
    renderConfirmReview(dispatch);

    const continueButton = await screen.findByRole("button", {
      name: "Confirm profile",
    });
    fireEvent.click(continueButton);
    await vi.waitFor(() => expect(gate.release).not.toBeNull());

    expect(continueButton).toBeDisabled();
    // Nothing on screen calls this a failed save: the confirmation landed, and
    // the only thing outstanding is the profile it created.
    expect(screen.queryAllByText(/I could not save that yet/)).toHaveLength(0);

    gate.release?.();
    await vi.waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "COMPANY_CONFIRMED" }),
    );
    // One confirm, one lookup: the reader was never able to start a second of
    // either while the first was still running.
    expect(confirmCalls).toBe(1);
  });

  // The one step of this recovery that can still fail is loading the company.
  // A failure there is as stuck as the loop the recovery exists to close. The
  // confirm attempt cleared its own block on the way in, so a reader with no
  // notice presses the same button and earns the same 409. Say what happened,
  // never walk the reader forward on a profile that never arrived, and leave
  // a way forward on screen.
  it("admits the company never loaded after an already-confirmed read, and offers the load again", async () => {
    let companyCalls = 0;
    const dispatch = vi.fn();
    stubWithSession(
      {
        [`GET /company/site-reads/${REVIEW_READ_ID}`]: () =>
          jsonResponse({
            ...REVIEW_READ,
            profile_fields: CONFIRM_FIELDS.map(toColdField),
          }),
        "GET /onboarding/company/proposal": () =>
          jsonResponse(reviewProposal(CONFIRM_FIELDS)),
        "GET /company": () => {
          companyCalls += 1;
          // The one probe that could move the reader forward fails first: a
          // problem body, so the profile never arrives.
          return companyCalls > 1
            ? jsonResponse({
                display_name: "Acme Inc",
                offer_summary: "CRM software",
                icp: "Mid-market B2B",
              })
            : jsonResponse(
                { title: "backend unavailable", code: "internal" },
                503,
              );
        },
        [CONFIRM_PATH]: () =>
          jsonResponse({ title: "conflict", code: "already_confirmed" }, 409),
      },
      {},
    );
    renderConfirmReview(dispatch);

    fireEvent.click(
      await screen.findByRole("button", { name: "Confirm profile" }),
    );

    await screen.findByText(CHECK_FAILED_NOTICE);
    // The reader was never walked forward onto a profile that never arrived.
    // The read was never called unconfirmable either: the server said the
    // opposite.
    expect(dispatch).not.toHaveBeenCalledWith({ type: "COMPANY_CONFIRMED" });
    expect(screen.queryByText(NOT_READY_NOTICE)).toBeNull();

    // The notice's own retry is the route forward: the same load, run again,
    // with the probe answering this time.
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await vi.waitFor(() =>
      expect(dispatch).toHaveBeenCalledWith({ type: "COMPANY_CONFIRMED" }),
    );
  });
});
