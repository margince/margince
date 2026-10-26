// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { useMe } from "../../lib/session";
import { PhaseHistoryCard } from "./projectphasehistory";
import {
  PROJECT_COMMITMENTS_ANCHOR,
  PROJECT_CONTRACTS_ANCHOR,
  PROJECT_DEALS_ANCHOR,
} from "./projectreadings";
import {
  CommitmentsCard,
  type Project360,
  ProjectContractsCard,
  ProjectDealsCard,
} from "./projectsections";

/**
 * The cards under How it stands, in two columns: the deals over the open
 * tasks, the contracts over the phase history.
 */
export function ProjectCards({
  view,
  newDeal,
  tick,
  onOpenTask,
  onOpenTasks,
  onOpenDocuments,
}: Readonly<{
  view: Project360;
  // The deals card's verb, built by the page that may reach the deal flow.
  newDeal?: ReactNode;
  // The complete box of one open task, for a reader who may write tasks.
  tick?: (activityId: string) => ReactNode;
  onOpenTask: (activityId: string) => void;
  onOpenTasks: () => void;
  onOpenDocuments: () => void;
}>) {
  const me = useMe();
  return (
    <div className="project-cards">
      <div className="project-cards-column">
        <div id={PROJECT_DEALS_ANCHOR}>
          <ProjectDealsCard view={view} actions={newDeal} />
        </div>
        <div id={PROJECT_COMMITMENTS_ANCHOR}>
          <CommitmentsCard
            view={view}
            meId={me.data?.user.id}
            tick={tick}
            onOpenTask={onOpenTask}
            onOpenAll={onOpenTasks}
          />
        </div>
      </div>
      <div className="project-cards-column">
        <div id={PROJECT_CONTRACTS_ANCHOR}>
          <ProjectContractsCard view={view} onOpenDocuments={onOpenDocuments} />
        </div>
        <PhaseHistoryCard view={view} />
      </div>
    </div>
  );
}
