import { Badge } from "../design-system/atoms";
import { formatNumber, INTL_LOCALE } from "../format/format";
import { useNow } from "../format/now";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
import { useAgentTierMap } from "../lib/autonomy";
import type { LicensePosture } from "../lib/license-posture";
import { routeHash } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";
import { modelText } from "./agentrail.model";
import { RailLine } from "./agentrail-line";
import type {
  AiActivityItem,
  AiCall,
  AiPosture,
  OfflineSource,
} from "./agentrail-reads";
import { speak } from "./ai-activity-speak";

/** How many of the agent's last actions the panel recaps. */
const RECAP_ROWS = 5;

/** Where a licence key is entered: the seats section of settings. */
const LICENSE_SETTINGS_HREF = routeHash(settingsHref("seats"));

/**
 * What a recap row's mark says, which is how that occurrence went.
 *
 * The orb's current tone would be wrong here. A brief that failed at four in
 * the morning would show in the green of a quiet afternoon. Position and the
 * stamp at the row's end already say how old a row is. So the mark says the
 * one thing nothing else on the row does.
 */
const ROW_TONE: Readonly<
  Record<AiActivityItem["state"], "info" | "success" | "warning" | "danger">
> = {
  queued: "info",
  running: "info",
  stalled: "warning",
  done: "success",
  degraded: "warning",
  failed: "danger",
};

/**
 * When it happened, as a contact would say it. A wall-clock stamp answers "at
 * what time", and the question a recap answers is "how long ago".
 */
function agoFor(
  iso: string,
  locale: Locale,
  now: number,
  t: Translator,
): string {
  const seconds = Math.round((now - Date.parse(iso)) / 1000);
  if (Number.isNaN(seconds)) {
    return t("agent.line.justNow");
  }
  const format = new Intl.NumberFormat(INTL_LOCALE[locale], {
    style: "unit",
    unitDisplay: "narrow",
    maximumFractionDigits: 0,
    unit:
      seconds < 60
        ? "second"
        : seconds < 3600
          ? "minute"
          : seconds < 86_400
            ? "hour"
            : "day",
  });
  const size =
    seconds < 60 ? 1 : seconds < 3600 ? 60 : seconds < 86_400 ? 3600 : 86_400;
  return format.format(Math.max(0, Math.floor(seconds / size)));
}

/**
 * The recap: what the agent has done lately, and the door to the whole trace.
 *
 * Five rows at most. A sixth turns the panel into a log viewer, which already
 * exists and is better at it.
 *
 * It reads the AI-activity feed, not the model-call trace, because of what a
 * row can say. The trace is telemetry and carries no record. A call's subject
 * goes to the occurrence and never to `ai_call`. So a recap read from the trace
 * could only say that something happened five times. The occurrence knows what
 * the work was about. That is how these rows name the account and link to it,
 * in the vocabulary the running section above uses.
 */
export function Recap({
  settled,
}: Readonly<{
  /** Today's settled occurrences, or undefined while no read has answered. */
  settled: readonly AiActivityItem[] | undefined;
}>) {
  const { locale } = useLocale();
  const t = useT();
  // One reading of the clock per render, so five rows cannot disagree about
  // what "now" is.
  const now = useNow(60_000);
  if (settled === undefined) {
    // Render nothing. The panel cannot tell a read still in flight from one
    // that failed. Saying "nothing has finished today" would be false for both.
    return null;
  }
  const said = settled
    .flatMap((item) => {
      const line = speak(item, t);
      return line === null ? [] : [{ item, line }];
    })
    .slice(0, RECAP_ROWS);
  if (said.length === 0) {
    return <p className="arempty t-caption">{t("agent.panel.nothingToday")}</p>;
  }
  return (
    <>
      {said.map(({ item, line }) => (
        <p className="aritem" key={item.id}>
          {/* How that one went, in the state families: a failed run is the row
              a reader is looking for, and it was the only thing on this list
              that could not be told from the four beside it. */}
          <span
            className="armark"
            aria-hidden="true"
            data-tone={ROW_TONE[item.state]}
          />
          <span className="arsaid">
            <RailLine line={line} />
          </span>
          {/* A settled occurrence has a finish (the projection's own `CHECK`
              says so). `started_at` is the fallback if a server ever sends
              one without it. */}
          <span className="armuted t-caption t-num">
            {agoFor(item.finished_at ?? item.started_at, locale, now, t)}
          </span>
        </p>
      ))}
    </>
  );
}

/**
 * What the agent is standing on: the provider, the model it last ran on, how
 * many tools it holds and which sources it cannot reach.
 *
 * A definition list, because every one of them is a term and its value. They
 * were one wrapped line of `<b>`-and-word fragments, which is the shape of a
 * sentence and read as one.
 *
 * The faults lead, above the list and as badges. An unbound model and a refused
 * licence decide whether anything under them means anything. Neither is a fact
 * with a value: each is a repair, with somewhere to go.
 */
export function RuntimeFacts({
  offline,
  model,
  ai,
  license,
  licenseLine,
}: Readonly<{
  offline: readonly OfflineSource[];
  model: Readonly<{ allowed: boolean; calls: readonly AiCall[] }>;
  ai: AiPosture;
  license: LicensePosture | undefined;
  licenseLine: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const tools = Object.values(useAgentTierMap()).length;
  const licenceFault = license === "none" || license === "refused";
  return (
    <div className="armeta">
      {(ai === "unconfigured" || licenceFault) && (
        <div className="arflags">
          {ai === "unconfigured" && (
            <Badge tone="warning">{t("agent.fact.noModel")}</Badge>
          )}
          {/* The badge names a fault the reader can repair only on the seats
              page, so a link around it takes them there: the badge stays the
              label nobody presses, and the anchor carries the press. */}
          {licenceFault && (
            <a className="arwarning" href={LICENSE_SETTINGS_HREF}>
              <Badge tone="warning">{licenseLine}</Badge>
            </a>
          )}
        </div>
      )}
      <dl className="arfacts">
        {ai === "development" && (
          <>
            <dt>{t("auth.coreDevelopment")}</dt>
            <dd>{t("auth.coreModeDevelopment")}</dd>
          </>
        )}
        <dt>{t("agent.fact.model")}</dt>
        {/* Italic where there is no model to name: the words are the reason
            there is none, not a value. */}
        <dd>
          {model.calls.length > 0 ? (
            modelText(model, t)
          ) : (
            <i>{modelText(model, t)}</i>
          )}
        </dd>
        {tools > 0 && (
          <>
            <dt>{t("agent.fact.tools")}</dt>
            <dd>{formatNumber(tools, locale)}</dd>
          </>
        )}
        {offline.length > 0 && (
          <>
            <dt>{t("agent.fact.sources")}</dt>
            <dd>
              {offline.map((source) => (
                <span className="arconn" key={source.id}>
                  <i aria-hidden="true" />
                  {`${source.label} ${t("agent.fact.offline")}`}
                </span>
              ))}
            </dd>
          </>
        )}
      </dl>
    </div>
  );
}
