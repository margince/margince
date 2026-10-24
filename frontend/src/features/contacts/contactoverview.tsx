import type { components } from "../../api/schema";
import type { ContactTab } from "../../lib/contacttab";
import { useUrlParams } from "../../lib/urlstate";
import {
  ContactCommitmentsCard,
  ContactDealCard,
  ContactUnderstandingCard,
} from "./contactcards";
import { contactFirstName } from "./contactfirstname";
import { ContactHowItStands } from "./contacthowitstands";
import { ContactMemory } from "./contactmemory";
import { ContactToday } from "./contacttoday";

type Contact360 = components["schemas"]["Contact360"];

export function ContactOverview({
  view,
  onAction,
  onOpenEmail,
  onOpenTab,
}: Readonly<{
  view: Contact360;
  onAction: (action: components["schemas"]["ContactMomentAction"]) => void;
  onOpenEmail: (id: string) => void;
  onOpenTab: (tab: ContactTab) => void;
}>) {
  // The whole queue opens as a drawer over the record, the same one the Home
  // page draws. A reader who leaves the record to see everything owed has
  // just lost the reason they opened it.
  const [params, setParams] = useUrlParams();
  const openQueue = () => {
    const next = new Map(params);
    next.set("queue", "1");
    setParams(next);
  };
  const firstName = contactFirstName(view.contact);
  return (
    <>
      <ContactToday
        moment={view.moment}
        view={view}
        onAction={onAction}
        onOpenEmail={onOpenEmail}
        onOpenTasks={openQueue}
      />
      <ContactHowItStands view={view} />
      <div className="pe-overview-grid">
        <div className="pe-overview-column">
          <ContactCommitmentsCard
            view={view}
            firstName={firstName}
            onAllTasks={openQueue}
          />
          <ContactDealCard
            view={view}
            firstName={firstName}
            onOpenDeals={() => onOpenTab("deals")}
          />
        </div>
        <div className="pe-overview-column">
          <ContactMemory
            view={view}
            onOpenEmail={onOpenEmail}
            onFullHistory={() => onOpenTab("timeline")}
          />
          <ContactUnderstandingCard
            view={view}
            firstName={firstName}
            onCorrect={() => onOpenTab("research")}
          />
        </div>
      </div>
    </>
  );
}
