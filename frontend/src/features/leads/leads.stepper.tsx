import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import type { ReadingTone } from "../../design-system/readingtone";
import { StageLadder } from "../../design-system/stageladder";
import { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type Lead = components["schemas"]["Lead"];

// The ladder a lead climbs, as How it stands draws it:
// New → Contacted → Engaged → Qualified | Disqualified. Steps up to the
// current one are filled; an open step is a button that places the lead
// there by hand; the two terminal steps open their dialogs, because each is
// a governed transition with its own questions (a contact, a reason).

export const LADDER_OPEN_STEPS = ["new", "contacted", "engaged"] as const;
export type LadderOpenStep = (typeof LADDER_OPEN_STEPS)[number];

const STEP_LABEL: Record<Lead["status"], MessageKey> = {
  new: "lead.status.new",
  contacted: "lead.status.contacted",
  engaged: "lead.status.engaged",
  promoted: "lead.statusPromoted",
  disqualified: "lead.statusDisqualified",
};

function rungOf(status: Lead["status"]): number {
  switch (status) {
    case "new":
      return 0;
    case "contacted":
      return 1;
    case "engaged":
      return 2;
    case "promoted":
    case "disqualified":
      return 3;
  }
}

export function LeadStepper({
  lead,
  pending,
  reasonId,
  onStep,
  onQualify,
  onDisqualify,
  variant,
  tone,
  dates,
}: Readonly<{
  lead: Lead;
  // `bars` inside How it stands, with the day the lead entered each rung.
  variant?: "chevrons" | "bars";
  tone?: ReadingTone;
  dates?: Partial<Record<LadderOpenStep, ReactNode>>;
  pending: boolean;
  // The page's one sentence on why no step may be taken (a closed lead):
  // every rung points at it instead of printing it.
  reasonId?: string;
  onStep: (status: LadderOpenStep) => void;
  onQualify: () => void;
  onDisqualify: () => void;
}>) {
  const t = useT();
  const current = rungOf(lead.status);
  const steps: {
    key: Lead["status"];
    rung: number;
    onPick: () => void;
    filled: boolean;
    current: boolean;
  }[] = [
    ...LADDER_OPEN_STEPS.map((status, rung) => ({
      key: status,
      rung,
      onPick: () => onStep(status),
      filled: current >= rung,
      current: lead.status === status,
    })),
    {
      key: "promoted" as const,
      rung: 3,
      onPick: onQualify,
      filled: lead.status === "promoted",
      current: lead.status === "promoted",
    },
    {
      key: "disqualified" as const,
      rung: 3,
      onPick: onDisqualify,
      filled: lead.status === "disqualified",
      current: lead.status === "disqualified",
    },
  ];
  return (
    <StageLadder
      label={t("lead.ladder")}
      variant={variant}
      tone={tone}
      steps={steps.map((step) => ({
        key: step.key,
        label: t(STEP_LABEL[step.key]),
        done: step.filled && !step.current,
        current: step.current,
        // Qualified and disqualified are the two ways out of the ladder, not
        // its last two rungs — they share a rung and only one is ever taken.
        terminal: step.rung === 3,
        disabled: pending,
        reasonId,
        testId: `lead-step-${step.key}`,
        onPick: step.onPick,
        sub: isOpenStep(step.key) ? dates?.[step.key] : undefined,
      }))}
    />
  );
}

function isOpenStep(status: Lead["status"]): status is LadderOpenStep {
  return LADDER_OPEN_STEPS.some((step) => step === status);
}
