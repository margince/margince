// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { RingsLegend, type RingTone } from "../../design-system/rings";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";

type Contact360 = components["schemas"]["Contact360"];

/** One ring of a contact's 360: a share, its tone, and the words for it. */
export type ContactRing = {
  key: "conversation" | "reach" | "promises";
  value: number;
  max: number;
  tone: RingTone;
};

// Below a third of the exchange coming from them, or above two thirds, the
// conversation runs one way; the company 360 reads the same balance.
const ONE_WAY_LOW = 1 / 3;
const ONE_WAY_HIGH = 2 / 3;

/**
 * The three shares a contact's relationship is read from: her part of the
 * conversation, how many of our colleagues are in touch with her, and how many
 * of our promises to her are kept. A ring is drawn only from a section this
 * reader was sent and that has something to count, so a withheld or empty
 * section is left out rather than drawn as a zero.
 */
export function contactRings(view: Contact360): ContactRing[] {
  const omitted = new Set(view.sections_omitted ?? []);
  const rings: ContactRing[] = [];
  const strength = omitted.has("strength") ? undefined : view.strength;
  const theirs = strength?.inbound_90d ?? 0;
  const exchanged = theirs + (strength?.outbound_90d ?? 0);
  if (exchanged > 0) {
    const share = theirs / exchanged;
    rings.push({
      key: "conversation",
      value: theirs,
      max: exchanged,
      tone: share < ONE_WAY_LOW || share > ONE_WAY_HIGH ? "warning" : "calm",
    });
  }
  const colleagues = omitted.has("network")
    ? []
    : (view.network?.colleagues ?? []);
  if (colleagues.length > 0) {
    const inTouch = colleagues.filter((one) => one.interactions_90d > 0).length;
    rings.push({
      key: "reach",
      value: inTouch,
      max: colleagues.length,
      tone: inTouch > 1 ? "calm" : "warning",
    });
  }
  const promises = omitted.has("claims")
    ? []
    : (view.claims ?? []).filter(
        (claim) =>
          claim.kind === "commitment_ours" && claim.status !== "dismissed",
      );
  if (promises.length > 0) {
    const kept = promises.filter((claim) => claim.status === "done").length;
    rings.push({
      key: "promises",
      value: kept,
      max: promises.length,
      tone: kept === promises.length ? "calm" : "warning",
    });
  }
  return rings;
}

/**
 * ContactRings draws those shares beside the contact's verdict, each with the
 * count it was read from, in the same order as the figure.
 */
export function ContactRings({
  view,
  firstName,
}: Readonly<{ view: Contact360; firstName: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const rings = contactRings(view);
  if (rings.length === 0) {
    return null;
  }
  const n = (value: number) => formatNumber(value, locale);
  const caption = (ring: ContactRing) =>
    plural(`contact.rings.${ring.key}.caption`, ring.max, {
      name: firstName,
      part: n(ring.value),
      count: n(ring.max),
    });
  const labelled = rings.map((ring) => ({
    ...ring,
    label: t(`contact.rings.${ring.key}`),
    caption: caption(ring),
  }));
  return (
    <RingsLegend
      rings={labelled}
      label={labelled
        .map((ring) => `${ring.label}: ${ring.caption}`)
        .join(", ")}
    />
  );
}
