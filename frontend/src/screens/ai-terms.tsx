// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Layers, ListChecks, Server } from "lucide-react";
import type { ReactNode } from "react";
import { Badge } from "../design-system/atoms";
import { useT } from "../i18n";

// The three words this settings area is built from, each with one mark wherever
// it appears: a PROVIDER is who is called, a TIER is a class of work with a
// model bound to it, a TASK is what the product does and is fixed to a tier.
//
// Told apart by icon and not by colour. Colour on this page already carries
// state (active, ready, needs a key, not responding), and a term drawn in the
// same tints would read as a status.

export type Term = "provider" | "tier" | "task";

const ICON = { provider: Server, tier: Layers, task: ListChecks } as const;

export function TermChip({
  term,
  children,
}: Readonly<{ term: Term; children: ReactNode }>) {
  return <Badge icon={ICON[term]}>{children}</Badge>;
}

/** A model as every list on this page writes it: its provider's mark, then its id. */
export function ModelRef({
  provider,
  model,
}: Readonly<{ provider: string; model: string }>) {
  return (
    <span className="ai-tier-modelline">
      <TermChip term="provider">{provider}</TermChip>
      <span>{model}</span>
    </span>
  );
}

/** A card title led by its term's icon, so the card names what it holds. */
export function PanelTitle({
  term,
  children,
}: Readonly<{ term: Term; children: ReactNode }>) {
  const Icon = ICON[term];
  return (
    <span className="ai-panel-title">
      <Icon size={20} aria-hidden="true" />
      {children}
    </span>
  );
}

/** The chain in one line, above the list that spells it out row by row. */
export function TermLegend() {
  const t = useT();
  return (
    <p className="t-sub ai-term-legend">
      <TermChip term="provider">{t("aiTerms.provider")}</TermChip>
      <span>{t("aiTerms.providerGloss")}</span>
      <span aria-hidden>→</span>
      <TermChip term="tier">{t("aiTerms.tier")}</TermChip>
      <span>{t("aiTerms.tierGloss")}</span>
      <span aria-hidden>→</span>
      <TermChip term="task">{t("aiTerms.task")}</TermChip>
      <span>{t("aiTerms.taskGloss")}</span>
    </p>
  );
}
