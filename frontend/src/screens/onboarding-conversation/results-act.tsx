import { useQuery } from "@tanstack/react-query";
import type { Dispatch } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { useLocale, useT } from "../../i18n";
import { throwProblem } from "../common";
import type { PayoffCounts } from "../onboarding-payoff";
import { PayoffGrid } from "../onboarding-payoff";
import { ResultsStep } from "../onboarding-results";
import type {
  ConversationEvent,
  ConversationState,
} from "./conversation-machine";
import { loadWizardState } from "./index";
import { presenceFor } from "./presence";
import { ConversationWorkbench } from "./workbench";

// The results act: an honest recap of what the funnel actually did — a
// skipped voice step is named a starter voice, an unconfirmed profile is
// named unsaved, never claimed as captured.

type CompanyProfile = components["schemas"]["CompanyProfile"];
type CompanySiteRead = components["schemas"]["CompanySiteRead"];

type ResultsActProps = Readonly<{
  state: ConversationState;
  dispatch: Dispatch<ConversationEvent>;
  profile: CompanyProfile | null;
  voiceBuilt: boolean;
  /** Server corpus words; null when the voice step was skipped or never built. */
  corpusWords: number | null;
}>;

// The counts alone. What the setup COST belongs to the band, on every screen
// rather than only this one, and it comes off this same cached read: see
// `useSetupRuntime`.
//
// What the setup actually produced, counted from the server's own records.
//
// The confirmed profile is the authority for what was kept — its `fields` and
// `facts` are what a later screen will show — while the site read is the only
// record of the work that produced them (pages crawled, people proposed). Any
// count whose source is missing stays null, and the grid omits that cell rather
// than printing a zero the reader did not earn.
function usePayoffFacts(
  profile: CompanyProfile | null,
  corpusWords: number | null,
): PayoffCounts {
  const wizard = useQuery({
    queryKey: ["onboarding-conv-state"],
    queryFn: loadWizardState,
  });
  const readId = wizard.data?.site_read_id ?? null;
  const read = useQuery({
    queryKey: ["onboarding-conv-read", readId],
    enabled: readId !== null,
    queryFn: async (): Promise<CompanySiteRead | null> => {
      const { data, error, response } = await api.GET(
        "/company/site-reads/{readId}",
        { params: { path: { readId: readId ?? "" } } },
      );
      if (error) {
        // A read the server no longer serves costs the reader two cells of a
        // recap, not the recap: the counts go absent, nothing throws.
        if (response.status === 404) {
          return null;
        }
        throwProblem(error);
      }
      return data;
    },
  });

  const dossier = read.data ?? null;
  return {
    factsRead: dossier?.facts.length ?? null,
    factsConfirmed:
      profile?.facts?.length ?? wizard.data?.selected_fact_keys.length ?? null,
    peopleFound: dossier?.people.length ?? null,
    profileFields: profile?.fields?.length ?? null,
    pagesRead: dossier?.pages_read ?? null,
    voiceWords: corpusWords,
  };
}

export function ResultsAct({
  state,
  dispatch,
  profile,
  voiceBuilt,
  corpusWords,
}: ResultsActProps) {
  const t = useT();
  const { locale } = useLocale();
  const counts = usePayoffFacts(profile, corpusWords);
  return (
    <ConversationWorkbench
      core={presenceFor(state).core}
      railState={state}
      status={t("ob.ai.ready")}
      title={t("ob.conv.results.artifactTitle")}
      sub={t("ob.conv.results.artifactBody")}
    >
      {/* What the setup produced, in figures, before the recap that names each
          one. The counts used to be a message in the transcript, which put the
          only quantities anybody would repeat to a colleague inside a chat log
          nobody scrolls back through. */}
      <PayoffGrid counts={counts} locale={locale} />
      <div className="mw-review ob-conv-artifact">
        <ResultsStep
          voiceBuilt={voiceBuilt}
          profileSaved={profile !== null}
          profile={profile ?? undefined}
        />
        {/* Pinned to the surface's own foot: the recap has no unmet
            condition, so the bar carries the action alone. */}
        <div className="ob-triage-continue">
          <p className="ob-triage-continue-status" role="status" />
          <Button
            variant="primary"
            onClick={() => dispatch({ type: "RESULTS_CONTINUE" })}
          >
            {t("ob.payoff.understood")}
          </Button>
        </div>
      </div>
    </ConversationWorkbench>
  );
}
