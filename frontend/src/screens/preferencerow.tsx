import { Lock } from "lucide-react";

import { Checkbox } from "../design-system/atoms";
import type { useT } from "../i18n";
import { labelOf, type PurposeView, stateLineKey } from "./preferences.logic";

// One purpose, as a row the subject can stage a change on.
//
// Split from the page because the page is a query, a draft, a save and four
// notices, and this is a control with its own rules about what it may offer.
//
export function PreferenceRow({
  purpose,
  on,
  wording,
  t,
  onToggle,
  disabled,
  mayGrant,
}: Readonly<{
  purpose: PurposeView;
  on: boolean;
  wording: string;
  t: ReturnType<typeof useT>;
  onToggle: () => void;
  // True while a write this row's toggle could race with is in flight — a
  // locked purpose is already disabled for its own reason, so the two
  // conditions just combine below rather than this prop overriding that one.
  disabled: boolean;
  // Whether this SUBJECT may take a grant at all — see grantRefused below.
  //
  // Named mayGrant rather than subjectMayGrant because the helper that
  // computes it is called subjectMayGrant: an identically named prop resolves
  // to the IMPORT when somebody forgets to destructure it, which typechecks
  // cleanly and throws at render. That happened once already.
  mayGrant: boolean;
}>) {
  // A double-opt-in purpose can now be turned ON here, and this row used to
  // refuse it.
  //
  // The reason it refused was true when it was written: the write recorded a
  // grant this page's reusable token could not evidence, the send gate refused
  // that grant anyway, and a switch that always fails is worse than no switch.
  // So the subject could turn a subscription off and never back on, and the
  // mint guard refuses anybody doing it for them — a withdrawal was permanent.
  //
  // What the server does now is mail the confirmation link instead of writing
  // a dead row, and answer `confirmation_sent`. The switch no longer always
  // fails, so withholding it would be withholding the only route somebody has
  // back to a subscription they once wanted.
  //
  // The row still SAYS what pressing it does, because the outcome is not the
  // one a checkbox implies: nothing is subscribed until a link is clicked in
  // another window.
  const needsConfirmation = purpose.grant_needs_confirmation && !on;
  // WHETHER THE SUBJECT MAY BE GRANTED ANYTHING AT ALL, which is a different
  // question from whether this purpose needs a round trip.
  //
  // can_opt_in is `!locked && !grant_needs_confirmation && grantable`
  // (preferenceview.go), and `grantable` is about the SUBJECT: an archived or
  // Art. 17 anonymised record whose erasure destroyed the capability a fresh
  // grant would re-open. The server refuses those with `cannot_grant` however
  // the page asks.
  //
  // Reading can_opt_in directly would re-block every DOI row, because the flag
  // is false for those too. So the subject-level half is recovered from the
  // rows the round trip does NOT apply to: if not one of them may be granted,
  // it is the subject rather than the purpose.
  const grantRefused = !on && !mayGrant;
  return (
    <li className="pref-row">
      <div className="pref-row-main">
        <div className="pref-row-head">
          {purpose.locked && <Lock className="pref-lock-icon" aria-hidden />}
          <span className="pref-label">{labelOf(t, purpose)}</span>
          {purpose.locked && (
            <span className="pref-lock-badge">{t("prefs.alwaysOn")}</span>
          )}
        </div>
        <p className="t-caption" data-testid={`wording-${purpose.key}`}>
          {wording}
        </p>
        <p className="t-caption pref-state">{t(stateLineKey(purpose, on))}</p>
        {purpose.locked && (
          <p className="t-caption pref-locked-why">{t("prefs.lockedWhy")}</p>
        )}
        {grantRefused && (
          <p className="t-caption pref-locked-why">
            {t("prefs.cannotGrantWhy")}
          </p>
        )}
        {needsConfirmation && !grantRefused && (
          <p className="t-caption pref-locked-why">
            {t("prefs.confirmationNeededWhy")}
          </p>
        )}
      </div>
      {/* A Checkbox, not a Switch, and the difference is not cosmetic: a
          Switch IS the write, while a Checkbox states an intent something
          later submits (design-system/README.md). This page stages every
          change and commits them from the save bar below, so announcing
          role="switch" would tell a screen-reader user their choice had
          already taken effect when it has not. The visible label is
          hidden because the row draws its own richer heading above. */}
      <Checkbox
        checked={on}
        aria-label={labelOf(t, purpose)}
        disabled={purpose.locked || grantRefused || disabled}
        className="pref-check"
        // Empty, because aria-label above already names the control: a
        // second copy of the purpose name would put the same words on the
        // page twice for a sighted reader and read them twice to everyone
        // else.
        label=""
        onChange={onToggle}
      />
    </li>
  );
}
