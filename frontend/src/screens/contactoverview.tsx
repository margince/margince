import type { components } from "../api/schema";
import { navigate } from "../app/router";
import { useT } from "../i18n";
import {
  ContactBriefCard,
  ContactCommercialCard,
  ContactCommitmentsCard,
  ContactMattersCard,
  hasCommercial,
  hasMatters,
  hasOpenCommitments,
} from "./contactcards";
import { EnrichedFields } from "./contactcorrections";
import { ContactMemory } from "./contactmemory";
import { owedPromises } from "./contactowed";
import { ContactReadings } from "./contactreadings";
import { contactTabRoute } from "./contacttab";
import { ContactToday, hasContactWork } from "./contacttoday";

type Contact360 = components["schemas"]["Contact360"];

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
  const commitments = hasOpenCommitments(view);
  const work = hasContactWork(view) || commitments;
  const today = (
    <ContactToday
      moment={view.moment}
      view={view}
      onAction={onAction}
      onOpenEmail={onOpenEmail}
      onOpenTasks={() => navigate({ screen: "worklist" })}
    />
  );
  const readings = Boolean(
    view.last_inbound_at ||
      view.last_outbound_at ||
      view.next_meeting ||
      hasCommercial(view) ||
      owedPromises(view).length,
  );
  return (
    <div className="record-stack">
      {hasContactWork(view) && today}
      {commitments && (
        <ContactCommitmentsCard
          view={view}
          firstName={view.contact.full_name.split(" ")[0]}
          includeTasks={false}
        />
      )}
      <ContactBriefCard
        brief={brief}
        loading={briefLoading}
        failed={briefFailed}
        onRetry={onRetryBrief}
        view={view}
        onOpenEmail={onOpenEmail}
      />
      {!work && today}
      <ContactOverviewCoverage view={view} />
      {readings && (
        <ContactReadings
          view={view}
          onOpenTab={(tab) => navigate(contactTabRoute(view.contact.id, tab))}
        />
      )}
      <ContactMemory view={view} onOpenEmail={onOpenEmail} hideEmpty />
      {hasCommercial(view) && <ContactCommercialCard view={view} />}
      {hasMatters(view) && (
        <ContactMattersCard
          view={view}
          firstName={view.contact.full_name.split(" ")[0]}
        />
      )}
      <EnrichedFields contactId={view.contact.id} view={view} />
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
