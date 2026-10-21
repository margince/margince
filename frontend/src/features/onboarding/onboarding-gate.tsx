// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type FormEvent, useState } from "react";
import type { components } from "../../api/schema";
import { Disclosure } from "../../design-system/atoms";
import { SurfaceState } from "../../design-system/surfacestate";
import { type Locale, useT } from "../../i18n";
import { normalizeUrl } from "./onboarding";
import "./onboarding-gate.css";
import { GateColumn, TheatreTail } from "./onboarding-gate.theatre";

// The first screen of onboarding: one question, then the wait for the website
// read made worth watching.
//
// Two rules shape everything here. The surface is PROP-DRIVEN — no fetch, no
// router, no clock deciding what the UI claims — so the read's progress can only
// ever be what the polled `CompanySiteRead` actually says. And every number is
// an OPEN count: the wire carries no page-count denominator, so there is no
// fraction, no percentage and no bar with a known end to be drawn from it.

export { ReadTheatre } from "./onboarding-gate.theatre";

export type CompanySiteRead = components["schemas"]["CompanySiteRead"];

/**
 * A read in flight, as the column needs it. One optional object rather than
 * three optional props: a read without the host it is reading, or without the
 * locale its counts are formatted in, is not a state this surface has.
 */
export type GateScan = Readonly<{
  read: CompanySiteRead;
  host: string;
  locale: Locale;
}>;

export type CompanySiteReadPage = components["schemas"]["CompanySiteReadPage"];

export type CompanySiteReadFact = components["schemas"]["CompanySiteReadFact"];

export type AiRunSummary = components["schemas"]["AiRunSummary"];

export type Translate = ReturnType<typeof useT>;

/**
 * The ask: name, promise, one field.
 *
 * `running` covers the moment between the submit and the server handing back a
 * read — the Core answers, the submit stops accepting a second press. Once the
 * read exists the caller swaps this for `ReadTheatre`.
 */
/**
 * Why an earlier attempt is not on screen any more.
 *
 * `message` is a finished sentence, composed by the caller. The gate cannot
 * compose it itself: the reasons come from four different places (a failed
 * POST, a terminal read, a lost poll, a restore recap) and each already has its
 * own complete copy in the catalog. Wrapping one sentence inside another is how
 * you end up with "I could not start the read: I could not read that site."
 *
 * Two tones, because these are not the same news and must not read the same:
 * `error` means the read cannot happen as asked, `paused` means the server
 * shelved it and will come back to it. Rendering a deferral as a failure would
 * tell the reader to fix something that is not broken. The tone drives the
 * live-region role too, so the difference survives without colour.
 */
/**
 * Why the reader is at the gate rather than watching a read. The tone is what
 * decides whether it announces as an alert or a status, so a resumed setup can
 * never interrupt like a failure: only `error` is something that went wrong.
 */
export type GateNotice = Readonly<{
  tone: "error" | "paused" | "resumed";
  message: string;
}>;

export function OnboardingGate({
  name,
  running,
  notice,
  configuredModel,
  scan,
  readBroken,
  uncertainCount,
  onSubmit,
  onManual,
  onRetryRead,
}: Readonly<{
  name?: string;
  running: boolean;
  notice?: GateNotice;
  configuredModel: string;
  /**
   * The read this column is watching, once one is running. Present or absent as
   * a whole — the three values are only meaningful together, so there is no
   * state where the column has a read but not the host it is reading.
   */
  scan?: GateScan;
  /**
   * The read the reader is waiting on could not be fetched, and there is no
   * `scan` to fall back on: a resumed session whose OWN first poll fails
   * before any snapshot has ever landed narrates nothing `notice` can find
   * (the thread only carries what a running session watched happen), so
   * without this the column keeps `running` true forever over a plain form —
   * indistinguishable from a screen with nothing on it. `notice` still wins
   * where it has something to say.
   */
  readBroken?: boolean;
  /**
   * How many fields the read will hand to the confirm deck rather than settle
   * itself — the exact count `deckCards` builds one card per. This screen has
   * no query of its own that could derive it (PROP-DRIVEN, see the file
   * header), and the caller only has a real answer once the review proposal
   * exists, which is most of the way through a read. Absent, not zero, for
   * every render before that: a zero here would read as "nothing to decide"
   * when the honest fact is "not counted yet".
   */
  uncertainCount?: number;
  onSubmit: (host: string) => void;
  onManual: () => void;
  onRetryRead?: () => void;
}>) {
  const t = useT();
  const [website, setWebsite] = useState("");
  const [invalid, setInvalid] = useState(false);
  const named = name?.trim();

  // The read replaces the tail of the SAME column rather than a second screen
  // replacing this one — see GateColumn for why that is load-bearing.
  if (scan !== undefined) {
    return (
      <GateColumn scan={scan}>
        <TheatreTail
          read={scan.read}
          locale={scan.locale}
          configuredModel={configuredModel}
          uncertainCount={uncertainCount}
        />
      </GateColumn>
    );
  }

  if (readBroken && notice === undefined) {
    return (
      <GateColumn name={named}>
        <SurfaceState
          loadingLabel={t("ob.gate.sub")}
          state="failed"
          // Read by the `empty` arm alone, which a failed read never reaches:
          // what there is none of is not a question a broken poll has an
          // answer to.
          emptyLabel={t("common.empty")}
          detail={{ onRetry: onRetryRead }}
        >
          {null}
        </SurfaceState>
      </GateColumn>
    );
  }

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    // Guarded in the handler as well as the attribute: Enter reaches the form
    // even while the button is disabled.
    if (running) {
      return;
    }
    const target = normalizeUrl(website);
    if (!target.ok) {
      setInvalid(true);
      return;
    }
    setInvalid(false);
    onSubmit(target.host);
  };

  return (
    <GateColumn running={running} name={named}>
      <form className="ob-gate-form" onSubmit={submit}>
        <label className="sr-only" htmlFor="ob-gate-website">
          {t("ob.gate.field")}
        </label>
        {/* The border and the focus ring sit on the WRAPPER, so the field and
            its inline submit share one outline instead of drawing two. The
            stylesheet reads the input's own `aria-invalid` through `:has()` to
            colour that outline, so rejection is not mirrored onto a second
            attribute: one source, the one assistive technology reads. */}
        <div className="ob-gate-field">
          <input
            id="ob-gate-website"
            className="ob-gate-input"
            type="text"
            inputMode="url"
            autoComplete="url"
            spellCheck={false}
            placeholder={t("ob.gate.placeholder")}
            value={website}
            aria-invalid={invalid}
            aria-describedby={invalid ? "ob-gate-invalid" : undefined}
            onChange={(event) => {
              setWebsite(event.target.value);
              setInvalid(false);
            }}
          />
          <button
            type="submit"
            className="ob-gate-submit"
            disabled={running}
            aria-busy={running}
          >
            {t("ob.gate.submit")}
          </button>
        </div>
      </form>

      {invalid ? (
        // ds:ignore a bordered banner box, not a line
        <p className="ob-gate-alert" id="ob-gate-invalid" role="alert">
          {t("ob.gate.invalidUrl")}
        </p>
      ) : null}
      {notice === undefined ? null : (
        <p
          className={`ob-gate-alert is-${notice.tone}`}
          role={notice.tone === "error" ? "alert" : "status"}
        >
          {notice.message}
        </p>
      )}

      {/* Withheld while a start is in flight, so choosing the manual path cannot
          race a read that is already beginning. */}
      {running ? null : (
        <p className="ob-gate-alt t-sub">
          {t("ob.gate.altPrompt")}
          <button type="button" className="ob-gate-link" onClick={onManual}>
            {t("ob.gate.altAction")}
          </button>
        </p>
      )}

      {/* The promise the product makes about the read, one press away rather
          than in the sentence under the headline. Everyone is entitled to it;
          almost nobody needs it before typing a domain, and carrying it in the
          opening paragraph is what made that paragraph four lines long. */}
      <Disclosure summary={t("ob.gate.trustToggle")}>
        <p className="ob-gate-trust t-sub">{t("ob.gate.trustBody")}</p>
      </Disclosure>

      {/* Named BEFORE the reader hands over their website, not after: which
          model is about to read it is part of the decision to let it. */}
      <p className="ob-gate-ai t-caption">
        <span>{t("ob.scan.transparency")}</span>
        <b>{configuredModel}</b>
      </p>
    </GateColumn>
  );
}
