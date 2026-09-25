import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
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
// pass over a backlog raises every exception it can see at once and hands a rep
// a task per faulted deal, so "start" is a decision about a queue, and a
// decision about a queue needs its size.

// useStartCheck is the one press, shared by the first check and the recheck.
//
// A second press while a pass is in flight is refused by the server (409), and
// that refusal is not a failure to report as one: the pass the presser wanted
// is already running. Both callers render it as the same waiting line.
function useStartCheck() {
  const queries = useQueryClient();
  const [alreadyRunning, setAlreadyRunning] = useState(false);
  const start = useMutation({
    mutationFn: async () => {
      const { error, response } = await api.POST(
        "/forecast/assurance/runs",
        {},
      );
      if (response.status === 409) {
        setAlreadyRunning(true);
        return;
      }
      if (error) {
        throwProblem(error);
      }
      setAlreadyRunning(true);
    },
    onSettled: async () => {
      // The pass runs in the background, so nothing it wrote is readable yet.
      // Re-asking both reads is what turns the panel over when it lands.
      await queries.invalidateQueries({ queryKey: ["forecast-assurance"] });
      await queries.invalidateQueries({ queryKey: ["input-checks"] });
    },
  });
  return { start, alreadyRunning };
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
  const { start, alreadyRunning } = useStartCheck();
  const total = preview.findings.reduce((sum, found) => sum + found.count, 0);

  return (
    <PanelBody>
      <Callout
        tone="discovery"
        kind="standing"
        title={t("review.firstCheck.title")}
        actions={
          alreadyRunning ? null : (
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
        {alreadyRunning ? (
          <p>{t("review.check.running")}</p>
        ) : (
          <>
            <p>{t("review.firstCheck.body")}</p>
            {/* The two numbers the decision is about: how much would be
                looked at, and how much would land in the queue. */}
            <p>
              {t("review.firstCheck.scope", {
                deals: formatNumber(preview.eligible_deals, locale),
                findings: formatNumber(total, locale),
              })}
            </p>
          </>
        )}
      </Callout>
    </PanelBody>
  );
}

// Recheck is the same press on a workspace already in the cadence.
//
// It exists because a manager who has just fixed a record cannot otherwise make
// the panel agree with it before the call: the finding stands until the next
// nightly tick, and they walk in with a screen that contradicts the record.
export function Recheck() {
  const t = useT();
  const { start, alreadyRunning } = useStartCheck();

  if (alreadyRunning) {
    return <p className="sub">{t("review.check.running")}</p>;
  }
  return (
    <Button
      pending={start.isPending}
      busyLabel={t("review.recheck.starting")}
      onClick={() => start.mutate()}
    >
      {t("review.recheck.label")}
    </Button>
  );
}
