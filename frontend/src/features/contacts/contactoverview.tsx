import type { components } from "../../api/schema";
import { useT } from "../../i18n";
import { useUrlParams } from "../../lib/urlstate";
import { ContactBriefCard } from "./contactbrief";
import {
  ContactCommercialCard,
  ContactCommitmentsCard,
  ContactMattersCard,
  hasCommercial,
  hasMatters,
  hasOpenCommitments,
} from "./contactcards";
import { ContactMemory } from "./contactmemory";
import { ContactToday } from "./contacttoday";

type Contact360 = components["schemas"]["Contact360"];

// Where the relationship brief stands on the page. The standing chip in the
// head leads here (lib/reveal), so the id is one constant the two share.
export const BRIEF_ANCHOR = "contact-relationship-brief";

export function ContactOverview({
  view,
  brief,
  briefLoading,
  briefFailed,
  onRetryBrief,
  onAction,
  onOpenEmail,
}: Readonly<{
  view: Contact360;
  brief: components["schemas"]["ContactBrief"] | undefined;
  briefLoading: boolean;
  briefFailed: boolean;
  onRetryBrief: () => void;
  onAction: (action: components["schemas"]["ContactMomentAction"]) => void;
  onOpenEmail: (id: string) => void;
}>) {
  // The whole queue opens as a drawer OVER the record, the same one the Home
  // page draws, rather than as a page in its place: a reader who leaves the
  // record to see everything owed has just lost the reason they opened it.
  const [params, setParams] = useUrlParams();
  const openQueue = () => {
    const next = new Map(params);
    next.set("queue", "1");
    setParams(next);
  };
  const commitments = hasOpenCommitments(view);
  // The 360 leads, then what needs a contact today, then what was promised:
  // the record's standing is what every row under it is read against.
  return (
    <div className="record-stack">
      <div id={BRIEF_ANCHOR} className="pe-brief-anchor">
        <ContactBriefCard
          brief={brief}
          loading={briefLoading}
          failed={briefFailed}
          onRetry={onRetryBrief}
          view={view}
          onOpenEmail={onOpenEmail}
        />
      </div>
      <ContactToday
        moment={view.moment}
        view={view}
        onAction={onAction}
        onOpenEmail={onOpenEmail}
        onOpenTasks={openQueue}
      />
      {commitments && (
        <ContactCommitmentsCard
          view={view}
          firstName={view.contact.full_name.split(" ")[0] ?? ""}
          includeTasks={false}
        />
      )}
      <ContactOverviewCoverage view={view} />
      <ContactMemory view={view} onOpenEmail={onOpenEmail} hideEmpty />
      {hasCommercial(view) && <ContactCommercialCard view={view} />}
      {hasMatters(view) && (
        <ContactMattersCard
          view={view}
          firstName={view.contact.full_name.split(" ")[0] ?? ""}
        />
      )}
    </div>
  );
}

function ContactOverviewCoverage({ view }: Readonly<{ view: Contact360 }>) {
  const t = useT();
  const hidden = new Set(view.sections_omitted);
  const unavailable = [
    ...(hidden.has("activities") || hidden.has("conversation_memory")
      ? [t("contact.memory.title")]
      : []),
    ...(hidden.has("last_touch") ||
    hidden.has("commercial") ||
    hidden.has("claims") ||
    hidden.has("next_meeting")
      ? [t("contact.readings.title")]
      : []),
    ...(hidden.has("moments") ? [t("today.source.moments")] : []),
    ...(hidden.has("next_steps") ? [t("today.source.nextSteps")] : []),
  ];
  if (view.sections_omitted.length === 0) return null;
  return (
    <p className="t-sub">
      {t("contact.overview.partial")}
      {unavailable.length > 0 && (
        <>
          {" "}
          {t("contact.overview.unavailable", {
            sections: unavailable.join(", "),
          })}
        </>
      )}
    </p>
  );
}
