// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { OffsiteLink } from "../design-system/offsitelink";
import { useT } from "../i18n";

type CitedProvision = components["schemas"]["CitedProvision"];

// The law a refusal or finding rests on, as the recipient's pack cites it. The
// server sends one only while it may be shown. Without one, the caller shows
// its factual sentence alone.

/** The provision, linked to its official text. Not translated. */
export function CitedLaw({
  provision,
}: Readonly<{ provision: CitedProvision }>) {
  return (
    <OffsiteLink href={provision.source_url}>
      {`${provision.instrument} ${provision.provision}`}
    </OffsiteLink>
  );
}

/**
 * A finding with the law it rests on in brackets, as one sentence: "No reason
 * to contact … is on record (GDPR Art. 6(1))." The law is linked.
 */
export function FindingWithLaw({
  sentence,
  provision,
}: Readonly<{ sentence: string; provision: CitedProvision }>) {
  return (
    <>
      {sentence.replace(/\.\s*$/, "")} (
      <OffsiteLink href={provision.source_url}>
        {`${shortInstrument(provision.instrument)} ${provision.provision}`}
      </OffsiteLink>
      ).
    </>
  );
}

// The short name an instrument carries in its own trailing brackets, such as
// "GDPR" for "Regulation (EU) 2016/679 (GDPR)", or the whole name without one.
function shortInstrument(instrument: string): string {
  const abbreviation = /\(([^()]+)\)\s*$/.exec(instrument)?.[1];
  return abbreviation && !/\d/.test(abbreviation) ? abbreviation : instrument;
}

/**
 * Each distinct law a list of findings cites, after one short label. Nothing
 * when none cites one.
 */
export function CitedLaws({
  provisions,
}: Readonly<{ provisions: readonly (CitedProvision | undefined)[] }>) {
  const t = useT();
  const distinct = new Map<string, CitedProvision>();
  for (const provision of provisions) {
    if (provision) {
      distinct.set(
        `${provision.instrument}\u0000${provision.provision}`,
        provision,
      );
    }
  }
  if (distinct.size === 0) return null;
  return (
    <span>
      {t("compose.citation.law")}{" "}
      {[...distinct.entries()].map(([key, provision], index) => (
        <span key={key}>
          {index > 0 ? ", " : null}
          <CitedLaw provision={provision} />
        </span>
      ))}
    </span>
  );
}
