import { useId } from "react";

import type { components } from "../../api/schema";
import { ExtensionPanels } from "../../components/extensionpanels";
import { Button } from "../../design-system/atoms";
import { Chapter, ChapterJump, Chapters } from "../../design-system/chapter";
import { Panel, PanelBody } from "../../design-system/panel";
import { SurfaceState, sectionState } from "../../design-system/surfacestate";
import { useT } from "../../i18n";
import { RecordListsPanel } from "../records/recordlists";
import { omitted } from "./contact360";
import { ContactBillingRoles } from "./contactbillingroles";
import { ContactClaimReadings } from "./contactcards";
import { ContactConfirmCallout } from "./contactconfirm";
import { ContactConsentPanel } from "./contactconsentpanel";
import { EnrichedFields } from "./contactcorrections";
import { ContactDetails } from "./contactdetails";
import { ContactProviderSection } from "./contactprovider";
import { ContactHoldSection } from "./contactrail";
import "./contact360.css";

// The Research tab: what a machine read about this contact, kept beside the
// canonical record rather than folded into it. Then comes the record-keeping
// a rep does not need on the overview. Three chapters in a fixed order: what
// waits for the reader's verdict, what each source says, and the record.
// The enrichment rows are the panel that takes a verdict on each value
// (EnrichedFields); a second, read-only spelling of that list drifted.

type Contact360 = components["schemas"]["Contact360"];

export function ContactResearchTab({
  view,
  loading = false,
  onResearch,
}: Readonly<{
  view?: Contact360;
  loading?: boolean;
  // Opens the page's public-source research drawer, the same one the actions
  // menu opens. Absent where the reader may not start one.
  onResearch?: () => void;
}>) {
  const t = useT();
  const baseId = useId();
  const rows = view?.profile_fields ?? [];
  const fieldsState = sectionState(
    view,
    "profile_fields",
    Boolean(view?.profile_fields),
    rows.length,
    loading,
  );
  const providerWithheld = view ? omitted(view, "provider_profile") : false;
  // No provider is connected and none ever sold us anything here, so there is
  // no section to draw. An EMPTY list is that fact; a withheld one is the
  // grant, which the branch below names instead.
  const providerHasNothing =
    view != null &&
    !providerWithheld &&
    (view.provider_profiles?.length ?? 0) === 0;
  // Neither half has anything to say: the tab says so once, in Sources, and
  // has nothing to check.
  const nothing = view != null && providerHasNothing && fieldsState === "empty";
  const ids = {
    check: `${baseId}-check`,
    sources: `${baseId}-sources`,
    record: `${baseId}-record`,
  };
  const chapters = [
    ...(nothing
      ? []
      : [{ id: ids.check, title: t("contact.chapter.toCheck") }]),
    { id: ids.sources, title: t("contact.chapter.sources") },
    { id: ids.record, title: t("contact.chapter.recordKeeping") },
  ];

  return (
    <Chapters>
      <ChapterJump label={t("contact.chapter.jump")} chapters={chapters} />
      {nothing ? null : (
        <Chapter
          id={ids.check}
          title={t("contact.chapter.toCheck")}
          answer={t("contact.chapter.toCheckAnswer")}
        >
          <ToCheckBody view={view} fieldsState={fieldsState} />
        </Chapter>
      )}
      <Chapter
        id={ids.sources}
        title={t("contact.chapter.sources")}
        answer={t("contact.chapter.sourcesAnswer")}
        action={
          onResearch ? (
            <Button variant="ai" onClick={onResearch}>
              {t("contact.chapter.researchAction")}
            </Button>
          ) : undefined
        }
      >
        {view ? (
          <SourcesBody
            view={view}
            withheld={providerWithheld}
            nothing={nothing}
            hasNothing={providerHasNothing}
          />
        ) : null}
      </Chapter>
      {view ? (
        <Chapter
          id={ids.record}
          title={t("contact.chapter.recordKeeping")}
          answer={t("contact.chapter.recordKeepingAnswer")}
          rows
        >
          <ContactRecordKeeping view={view} />
        </Chapter>
      ) : null}
    </Chapters>
  );
}

// What waits for the reader's verdict: details to confirm, each captured
// value with its receipt, and what was read from conversations.
function ToCheckBody({
  view,
  fieldsState,
}: Readonly<{
  view?: Contact360;
  fieldsState: ReturnType<typeof sectionState>;
}>) {
  const t = useT();
  return (
    <>
      {view ? <ContactConfirmCallout view={view} /> : null}
      {/* Rows only when there are rows to judge; the withheld and loading
          states keep the panel's own shell, because EnrichedFields draws
          nothing for an empty list and a grant boundary must not read as
          "nothing was captured". */}
      {view && fieldsState === "ready" ? (
        <EnrichedFields contactId={view.contact.id} view={view} />
      ) : (
        <Panel title={t("contact.research.fields")}>
          <PanelBody>
            <SurfaceState
              loadingLabel={t("contact.research.fields")}
              state={fieldsState}
              emptyLabel={t("contact.research.fieldsEmpty")}
            >
              {null}
            </SurfaceState>
          </PanelBody>
        </Panel>
      )}
      {view ? <ContactClaimReadings view={view} /> : null}
    </>
  );
}

// What each source sold or read, or the one sentence that says nothing did.
function SourcesBody({
  view,
  withheld,
  nothing,
  hasNothing,
}: Readonly<{
  view: Contact360;
  withheld: boolean;
  nothing: boolean;
  hasNothing: boolean;
}>) {
  const t = useT();
  if (withheld || nothing) {
    return (
      <Panel>
        <PanelBody>
          <SurfaceState
            loadingLabel={t("provider.profile.title")}
            state={withheld ? "withheld" : "empty"}
            emptyLabel={t("contact.research.empty")}
          >
            {null}
          </SurfaceState>
        </PanelBody>
      </Panel>
    );
  }
  return hasNothing ? null : (
    <ContactProviderSection
      contactId={view.contact.id}
      profiles={view.provider_profiles}
    />
  );
}

// The record-keeping a rep does not need on the overview. It holds consent
// with its proof, billing roles, every field with its edit, and the
// correspondence hold. The lists and what extensions add close it.
function ContactRecordKeeping({ view }: Readonly<{ view: Contact360 }>) {
  return (
    <>
      <ContactConsentPanel view={view} />
      <ContactBillingRoles companies={view.billing_roles} />
      <ContactDetails
        contact={view.contact}
        profiles={view.provider_profiles}
      />
      <ContactHoldSection view={view} />
      <RecordListsPanel
        entityType="contact"
        entityId={view.contact.id}
        actions={false}
      />
      <ExtensionPanels
        slot="record_panel.contact"
        record={{ type: "contact", id: view.contact.id }}
      />
    </>
  );
}
