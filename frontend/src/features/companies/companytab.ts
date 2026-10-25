import { useEffect } from "react";
import { type CompanyTab, companyTabRoute } from "../../lib/companytab";
import { navigateReplacing } from "../../lib/router";
import { keptWorklistReturn } from "../worklist/worklist.return";

// The tabs an archived account cannot answer: the server refuses its roster,
// deals, tasks and money.
export const REFUSED_WHEN_ARCHIVED: readonly CompanyTab[] = [
  "contacts",
  "deals",
  "tasks",
  "finance",
];

// A link to a tab an archived account cannot answer opens its Overview, and the
// address is replaced so it names the tab on screen.
export function useShownCompanyTab(
  id: string,
  archived: boolean,
  addressed: CompanyTab,
): CompanyTab {
  const refused = archived && REFUSED_WHEN_ARCHIVED.includes(addressed);
  useEffect(() => {
    if (refused) {
      navigateReplacing(companyTabRoute(id, "overview"), keptWorklistReturn());
    }
  }, [refused, id]);
  return refused ? "overview" : addressed;
}
