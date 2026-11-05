import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

export type ProjectPhase = components["schemas"]["Project"]["phase"];

// Display order permits reopening a closed project into delivery.
export const PROJECT_PHASES = [
  "initiative",
  "pursuing",
  "delivering",
  "closed",
] as const satisfies readonly ProjectPhase[];

export const PHASE_LABEL: Record<ProjectPhase, MessageKey> = {
  initiative: "project.phase.initiative",
  pursuing: "project.phase.pursuing",
  delivering: "project.phase.delivering",
  closed: "project.phase.closed",
};

export function isProjectPhase(value: string): value is ProjectPhase {
  return (PROJECT_PHASES as readonly string[]).includes(value);
}
