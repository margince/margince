/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { candidateVersion } from "../../testing/voice-candidate";
import { CandidateBanner } from "./voice-versions";

// The banner asks the owner to accept or reject a voice that will write their
// mail. Everything that decision needs has to be ON it: what the voice says
// about them, and why it is waiting rather than activating itself. It shipped
// with a title, the evaluator's own log sentences, and two buttons — so "Use
// this version" meant approving writing nobody had seen, and reading it first
// was impossible.

type VoiceProfileVersion = components["schemas"]["VoiceProfileVersion"];

function view(ui: ReactNode, locale: "en" | "de" | "vi" = "en") {
  return render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial={locale}>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

// The banner carries the version it asks about; nothing on it fetches.
function review(version: VoiceProfileVersion, locale?: "en" | "de" | "vi") {
  return view(
    <CandidateBanner
      profileId="p-1"
      candidate={version}
      canEdit
      onChanged={() => undefined}
    />,
    locale,
  );
}

afterEach(cleanup);

describe("reviewing a candidate voice", () => {
  it("formats typed scores for German readers and retains incomplete reasons", async () => {
    const version = candidateVersion([
      "Stored similarity",
      "Missing score",
      "Recorded new reason",
    ]);
    version.evaluation.review_reason_details = [
      {
        code: "identity_shift",
        text: "Stored similarity",
        values: { score: 0.56 },
      },
      { code: "low_score", text: "Missing score", values: { floor: 0.6 } },
      { code: "malformed", text: "Different reason", values: {} },
    ];
    review(version, "de");
    expect(await screen.findByText(/Ähnlichkeit 0,56/)).toBeTruthy();
    expect(screen.getByText("Missing score")).toBeTruthy();
    expect(screen.getByText("Recorded new reason")).toBeTruthy();
  });

  it("renders typed reasons independently of their stored language", async () => {
    const raw = "Der Median der Stilbewertung liegt unter dem Mindestwert.";
    const version = candidateVersion([raw, "Unknown recorded reason"]);
    version.evaluation.review_reason_details = [
      { code: "low_score", text: raw, values: { score: 0.56, floor: 0.6 } },
      { code: "future_reason", text: "Unknown recorded reason", values: {} },
    ];
    review(version);
    expect(
      await screen.findByText(/scored 0.56 against your writing/),
    ).toBeTruthy();
    expect(screen.queryByText(raw)).toBeNull();
    expect(screen.getByText("Unknown recorded reason")).toBeTruthy();
  });

  it("explains changed inputs and all removed writing rules", async () => {
    const reasons = ["Changed corpus", "Changed active", "Removed rules"];
    const version = candidateVersion(reasons);
    version.evaluation.review_reason_details = [
      { code: "corpus_changed", text: reasons[0], values: {} },
      { code: "active_changed", text: reasons[1], values: {} },
      {
        code: "rules_removed",
        text: reasons[2],
        values: { avoid: 2, register: 3 },
      },
    ];
    review(version);
    expect(
      await screen.findByText(/writing samples changed during this build/),
    ).toBeTruthy();
    expect(
      screen.getByText(/active voice changed during this evaluation/),
    ).toBeTruthy();
    expect(
      screen.getByText(
        /wording and register dropped since your previous version: 5/,
      ),
    ).toBeTruthy();
  });

  it.each(["future_reason", "low_score", "missing"])(
    "retains stored wording for unsupported metadata: %s",
    async (code) => {
      const reason = "median voice score 0.56 is below the 0.60 floor";
      const version = candidateVersion([reason]);
      version.evaluation.review_reason_details =
        code === "missing" ? [] : [{ code, text: reason, values: {} }];
      review(version);
      expect(await screen.findByText(reason)).toBeTruthy();
      expect(screen.queryByText(/scored 0.56 against your writing/)).toBeNull();
    },
  );

  it("shows the voice itself, so the decision is about something visible", async () => {
    review(candidateVersion([]));

    // What the build actually learned, on the card that asks about it.
    expect(
      await screen.findByText(
        "State the ask, then the reason, then the deadline.",
      ),
    ).toBeTruthy();
    expect(screen.getByText(/Direct, concrete/)).toBeTruthy();
    // And the fact that nothing is written in it yet.
    expect(
      screen.getByText(/It is not used for drafts until you select it/),
    ).toBeTruthy();
  });

  // The evaluator writes for an operator reading a log. "median voice score
  // 0.56 is below the 0.60 floor" is not a sentence the contact being asked to
  // decide can act on.
  it("says why it is waiting in words about the owner's own voice", async () => {
    review(
      candidateVersion([
        "the model returned malformed drafts during evaluation",
        "median voice score 0.56 is below the 0.60 floor",
      ]),
    );

    expect(
      await screen.findByText(/scored 0.56 against your writing/),
    ).toBeTruthy();
    expect(screen.getByText(/could not read some sample drafts/)).toBeTruthy();
    // The raw operator sentences are gone from the reader's view.
    expect(screen.queryByText(/below the 0.60 floor/)).toBeNull();
    // And the reader is told what each answer means for them.
    expect(screen.getByText(/If it reads like you, use it/)).toBeTruthy();
  });

  // A reason nobody can read is bad; a reason nobody can SEE is worse, because
  // the candidate would then be held back for a cause never stated.
  it("shows a reason it does not recognize rather than dropping it", async () => {
    review(candidateVersion(["some future evaluator sentence"]));

    expect(
      await screen.findByText("some future evaluator sentence"),
    ).toBeTruthy();
  });
});
