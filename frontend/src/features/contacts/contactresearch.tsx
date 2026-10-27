import type { components } from "../../api/schema";
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

// The Data & tools tab: what a machine read about this contact, kept beside
// the canonical record rather than folded into it, and the record-keeping a
// rep does not need on the overview. Two kinds of research sit here: a bought
// provider snapshot and the enrichment evidence this app's own capture read
// off a page or a signature. The enrichment rows are the same panel that
// takes a verdict on each value (EnrichedFields). So a reader both checks a
// reading and corrects it here. A second, read-only spelling
// of the same list stood here before, and the two drifted. Under them sits
// the record-keeping the left column does not draw. It holds the details to
// confirm, consent with its proof and billing roles. It also holds every
// field with its inline edit, tags, source, custom fields, the correspondence
// hold and the lists.

type Contact360 = components["schemas"]["Contact360"];

export function ContactResearchTab({
  view,
  loading = false,
}: Readonly<{ view?: Contact360; loading?: boolean }>) {
  const t = useT();
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
  // grant, which the branch above names instead.
  const providerHasNothing =
    view != null &&
    !providerWithheld &&
    (view.provider_profiles?.length ?? 0) === 0;

  // The tab-wide empty state: neither half has anything to say about this
  // contact. Two stacked empty panels would say the same "nothing here" twice;
  // this says it once.
  if (view && providerHasNothing && fieldsState === "empty") {
    return (
      <div className="record-stack">
        <Panel title={t("tab.research")}>
          <PanelBody>
            <SurfaceState
              state="empty"
              emptyLabel={t("contact.research.empty")}
              loadingLabel={t("tab.research")}
            >
              {null}
            </SurfaceState>
          </PanelBody>
        </Panel>
        <ContactRecordKeepingStack view={view} />
      </div>
    );
  }

  return (
    <div className="record-stack">
      {view &&
        (providerWithheld ? (
          <Panel title={t("provider.profile.title")}>
            <PanelBody>
              <SurfaceState
                loadingLabel={t("provider.profile.title")}
                state="withheld"
                emptyLabel={t("contact.research.empty")}
              >
                {null}
              </SurfaceState>
            </PanelBody>
          </Panel>
        ) : (
          <ContactProviderSection
            contactId={view.contact.id}
            profiles={view.provider_profiles}
          />
        ))}
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
      {view && <ContactRecordKeepingStack view={view} />}
    </div>
  );
}

function ContactRecordKeepingStack({ view }: Readonly<{ view: Contact360 }>) {
  return (
    <>
      <ContactClaimReadings view={view} />
      <ContactConfirmCallout view={view} />
      <ContactConsentPanel view={view} />
      <ContactBillingRoles companies={view.billing_roles} />
      <ContactDetails
        contact={view.contact}
        profiles={view.provider_profiles}
        createdBy={view.created_by}
      />
      <ContactHoldSection view={view} />
      <RecordListsPanel entityType="contact" entityId={view.contact.id} />
    </>
  );
}
