import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { CoverageLine } from "./analytics.forecast.review";
import { QueryGate, throwProblem } from "./common";

type Preview = components["schemas"]["ForecastAssurancePreview"];

// The check a workspace has not had yet, and the one it can ask for again.
//
// Until somebody starts it the nightly pass skips this workspace, so the panel
// is not waiting on anything — it is asking. Drawing it as an ordinary empty
// state would be the wrong claim twice over: the pipeline has not been found
// clean, and nothing is on its way.
//
// The preview beside the button is what makes the ask answerable. The first
// pass over a backlog raises every finding it can see at once and hands each
// affected deal a task, so "start" is a decision about a queue, and a decision
// about a queue needs its size.

// howOftenToAskIfItLanded paces the poll that runs while a pass is in flight.
//
// The pass runs in the background, so nothing it writes is readable at the
// moment the press returns. Without this the panel would sit on "running"
// until something else happened to refetch — which, for a reader who pressed
// and is waiting, is indistinguishable from a press that did nothing.
const howOftenToAskIfItLanded = 5000;

// useStartCheck is the one press, shared by the first check and the recheck.
//
// A second press while a pass is in flight is refused by the server (409), and
// that refusal is not a failure to report as one: the pass the presser wanted
// is already running. Both callers render it as the same waiting line.
function useStartCheck() {
  const queries = useQueryClient();
  const [running, setRunning] = useState(false);
  const start = useMutation({
    mutationFn: async () => {
      const { error, response } = await api.POST(
        "/forecast/assurance/runs",
        {},
      );
      // 409 says the pass this reader wanted is already under way. Raised as an
      // error it would send them to fix something that is not wrong.
      if (response.status !== 409 && error) {
        throwProblem(error);
      }
      setRunning(true);
    },
  });

  // Ask again until it lands. Both reads, because a pass changes the run and
  // the findings together and a panel showing one against the other's previous
  // answer is a panel disagreeing with itself.
  useEffect(() => {
    if (!running) {
      return;
    }
    const poll = setInterval(() => {
      void queries.invalidateQueries({ queryKey: ["forecast-assurance"] });
      void queries.invalidateQueries({ queryKey: ["input-checks"] });
    }, howOftenToAskIfItLanded);
    return () => clearInterval(poll);
  }, [running, queries]);

  return { start, running, landed: () => setRunning(false) };
}

// StartError says why a press did nothing.
//
// Some seats see this panel without holding `forecast: create`, and for them
// the press is a 403. Silent, the stopped spinner is indistinguishable from a
// button that is not wired up.
function StartError({ error }: Readonly<{ error: Error | null }>) {
  const t = useT();
  if (!error) {
    return null;
  }
  return (
    <Callout tone="danger" kind="outcome" title={t("review.check.failed")}>
      <p>{error.message}</p>
    </Callout>
  );
}

// FirstCheck is the panel a workspace nobody has started renders.
export function FirstCheck({ title }: Readonly<{ title: string }>) {
  const t = useT();
  const preview = useQuery({
    queryKey: ["forecast-assurance-preview"],
    queryFn: async () => {
      const { data, error } = await api.GET("/forecast/assurance/preview", {});
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  return (
    <Panel title={title}>
      <PanelBody>
        {/* Said plainly, because the alternative reading is dangerous: a
            reader who takes an unchecked pipeline for a clean one has been
            told the opposite of what happened. */}
        <p>{t("review.notCheckedYet")}</p>
      </PanelBody>
      <QueryGate query={preview} pendingLabel={t("review.firstCheck.pending")}>
        {(scope) => <FirstCheckOffer preview={scope} />}
      </QueryGate>
    </Panel>
  );
}

function FirstCheckOffer({ preview }: Readonly<{ preview: Preview }>) {
  const t = useT();
  const { locale } = useLocale();
  const { start, running } = useStartCheck();
  const total = preview.findings.reduce((sum, found) => sum + found.count, 0);
  // A preview that could not read the deals reports no findings, and no
  // findings is exactly what a clean pipeline reports. Printing its zeroes
  // would tell a reader the pipeline is sound when nobody could look — the one
  // misreading this whole surface exists to prevent — so the scope sentence is
  // withheld and the sources that went unread take its place.
  const couldNotLook = preview.readiness === "checks_incomplete";

  return (
    <PanelBody>
      <Callout
        tone="discovery"
        kind="standing"
        title={t("review.firstCheck.title")}
        actions={
          running ? null : (
            <Button
              variant="primary"
              pending={start.isPending}
              busyLabel={t("review.firstCheck.starting")}
              onClick={() => start.mutate()}
            >
              {t("review.firstCheck.start")}
            </Button>
          )
        }
      >
        {running ? (
          <p>{t("review.check.running")}</p>
        ) : (
          <>
            <p>{t("review.firstCheck.body")}</p>
            {couldNotLook ? (
              <>
                <p>{t("review.firstCheck.cannotLook")}</p>
                <CoverageLine sources={preview.sources} />
              </>
            ) : (
              <p>
                {t("review.firstCheck.scope", {
                  deals: formatNumber(preview.eligible_deals, locale),
                  findings: formatNumber(total, locale),
                })}
              </p>
            )}
          </>
        )}
      </Callout>
      <StartError error={start.error} />
    </PanelBody>
  );
}

// Recheck is the same press on a workspace already in the cadence.
//
// It exists because a manager who has just fixed a record cannot otherwise make
// the panel agree with it before the call: the finding stands until the next
// nightly tick, and they walk in with a screen that contradicts the record.
//
// `runId` is what tells it the pass landed. The panel around it stays mounted
// across a run, so without a fact that CHANGES the waiting line would outlive
// the wait and the button would never come back.
export function Recheck({ runId }: Readonly<{ runId: string }>) {
  const t = useT();
  const { start, running, landed } = useStartCheck();
  const [pressedOn, setPressedOn] = useState<string | null>(null);

  useEffect(() => {
    if (running && pressedOn !== null && runId !== pressedOn) {
      landed();
    }
  }, [running, pressedOn, runId, landed]);

  if (running) {
    return <p className="sub">{t("review.check.running")}</p>;
  }
  return (
    <>
      <Button
        pending={start.isPending}
        busyLabel={t("review.recheck.starting")}
        onClick={() => {
          setPressedOn(runId);
          start.mutate();
        }}
      >
        {t("review.recheck.label")}
      </Button>
      <StartError error={start.error} />
    </>
  );
}
