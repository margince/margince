import { useRecordZone } from "../app/recordzone";
import { Badge, Button } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDate } from "../format/format";
import { useLocale, useT } from "../i18n";
import {
  type ProjectHealthAssessment,
  type ProjectHealthState,
  useProjectHealth,
} from "./projecthealth.queries";

/**
 * How the project is going, and how it has been going.
 *
 * The empty state says nobody has judged it, never "on track". A project
 * assessed and found healthy and a project nobody has looked at are different
 * states, and the second is the one worth chasing — a card that rendered them
 * alike would hide exactly the deliveries going unwatched.
 *
 * Superseded readings stay in the list and say so. A correction is a statement
 * that the earlier reading was wrong, and dropping it would leave a gap in the
 * history where the mistake was, which is the part a review wants to see.
 */
export function ProjectHealth({
  projectId,
  readOnly = false,
  onRecord,
  onCorrect,
}: Readonly<{
  projectId: string;
  // The page's own answer to whether this project takes writes. The panel does
  // not re-derive it: the server refuses an unauthorized write whatever this
  // says, and a second implementation of the gate here is the defect rather
  // than the protection.
  readOnly?: boolean;
  // The page owns the dialog — one modal per page, so two cards cannot both
  // put one on the screen. The panel only says which verb was asked for.
  onRecord?: () => void;
  onCorrect?: (assessment: ProjectHealthAssessment) => void;
}>) {
  const t = useT();
  const { data, isPending, isError } = useProjectHealth(projectId);
  const rows = data ?? [];
  const current = rows.find((row) => !row.superseded);
  // A failed read is not an unjudged project. Inviting a reading over a list
  // that could not be fetched invites a second judgement beside one that may
  // already stand.
  const canWrite = !readOnly && !isPending && !isError && Boolean(onRecord);
  return (
    <Panel
      title={t("projectHealth.title")}
      actions={
        canWrite ? (
          <Button variant="ghost" onClick={onRecord}>
            {t("projectHealth.record")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        {isPending || isError || rows.length === 0 ? (
          <SurfaceState
            state={isPending ? "loading" : isError ? "failed" : "empty"}
            emptyLabel={t("projectHealth.empty")}
            emptyDetail={t("projectHealth.emptyDetail")}
            loadingLabel={t("projectHealth.title")}
          >
            {null}
          </SurfaceState>
        ) : (
          <>
            {current && (
              <CurrentReading
                assessment={current}
                onCorrect={
                  canWrite && onCorrect ? () => onCorrect(current) : undefined
                }
              />
            )}
            <ul className="firmo healthlog">
              {rows.map((row) => (
                <HistoryRow key={row.id} assessment={row} />
              ))}
            </ul>
          </>
        )}
      </PanelBody>
    </Panel>
  );
}

/** The judgement that stands, with the day it applies to and why. */
function CurrentReading({
  assessment,
  onCorrect,
}: Readonly<{
  assessment: ProjectHealthAssessment;
  onCorrect?: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  return (
    <div className="health-current">
      <StateBadge state={assessment.state} />
      <span className="t-caption mute">
        {t("projectHealth.assessedOn", {
          date: formatDate(assessment.assessed_at, locale, recordZone),
        })}
      </span>
      {assessment.note && <p>{assessment.note}</p>}
      {/* Correcting is offered on the reading that STANDS and on no other. A
          superseded row has already been answered, and the server refuses a
          second correction of it. */}
      {onCorrect && (
        <Button variant="ghost" onClick={onCorrect}>
          {t("projectHealth.correct")}
        </Button>
      )}
    </div>
  );
}

function HistoryRow({
  assessment,
}: Readonly<{ assessment: ProjectHealthAssessment }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  return (
    <li>
      <span className="t-eyebrow">
        {formatDate(assessment.assessed_at, locale, recordZone)}
        {/* A withdrawn reading is marked rather than hidden: the correction is
            part of the record, and a history with the mistake removed reads as
            though nobody ever got it wrong. */}
        {assessment.superseded && ` ${t("projectHealth.corrected")}`}
      </span>
      <span className="healthlog-reading">
        <StateBadge state={assessment.state} />
        {assessment.note}
      </span>
    </li>
  );
}

/**
 * The state as a WORD, never a colour alone: a reader who cannot tell the tones
 * apart still reads the judgement.
 */
function StateBadge({ state }: Readonly<{ state: ProjectHealthState }>) {
  const t = useT();
  // Both arms spelled out rather than composed into `projectHealth.state.${state}`:
  // the i18n census reads translation keys as literals from source, and a key it
  // cannot see is one it reports as orphaned and the next author deletes.
  if (state === "on_track") {
    return <Badge tone="success">{t("projectHealth.state.onTrack")}</Badge>;
  }
  if (state === "at_risk") {
    return <Badge tone="warn">{t("projectHealth.state.atRisk")}</Badge>;
  }
  return <Badge tone="danger">{t("projectHealth.state.offTrack")}</Badge>;
}
