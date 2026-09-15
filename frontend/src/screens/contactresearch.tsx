import type { components } from "../api/schema";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState, sectionState } from "../design-system/surfacestate";
import { useT } from "../i18n";
import { omitted } from "./contact360";
import { EnrichedFields } from "./contactcorrections";
import { ContactProviderSection } from "./contactprovider";
import { ContactHoldSection } from "./contactrail";
import "./contact360.css";

// The Data & tools tab: what a machine read about this contact, kept beside
// the canonical record rather than folded into it, and the record-keeping a
// rep does not need on the overview. Two kinds of research sit here: a bought
// provider snapshot and the enrichment evidence this app's own capture read
// off a page or a signature. The enrichment rows are the SAME panel that
// takes a verdict on each value (EnrichedFields), so the tab is where a
// reader both checks a reading and corrects it; a second, read-only spelling
// of the same list stood here before, and the two drifted. Under them, the
// correspondence hold and the tags: filing, which the overview used to carry
// in its rail.

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
      {view && <ContactHoldSection view={view} />}
    </div>
  );
}
