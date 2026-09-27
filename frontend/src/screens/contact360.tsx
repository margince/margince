import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import { currentEmployer } from "./employmentcurrency";
import "./contact360.css";

export type Contact360 = components["schemas"]["Contact360"];

/**
 * useContact360 is the contact page's ONE read. It replaces the seven
 * per-card queries the screen used to fire, so every section describes the
 * same moment rather than a stack of independently-timed round trips.
 *
 * `enabled` is for the callers that are not the page: a surface that only
 * sometimes knows a contact asks under the SAME key, so it reads the page's
 * cache where there is one and opens no request at all where there is not.
 */
export function useContact360(id: string, enabled = true) {
  return useQuery({
    enabled,
    queryKey: ["contact360", id],
    queryFn: async () => {
      const { data, error } = await api.GET("/contacts/{id}/360", {
        params: { path: { id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
}

/** omitted reports whether a section was withheld for lack of a grant. */
export function omitted(
  view: Contact360 | undefined,
  section: string,
): boolean {
  return Boolean(view?.sections_omitted?.some((s) => s === section));
}

/**
 * thinRecord decides whether the page shows its authored thin state instead
 * of a stack of empty modules.
 *
 * The predicate is deliberately about the RELATIONSHIP, not about field
 * completeness: a contact with a full address block and no correspondence is
 * still someone nobody here has spoken to, which is the thing the page has
 * to say out loud.
 */
export function thinRecord(view: Contact360 | undefined): boolean {
  if (!view) {
    return false;
  }
  // A WITHHELD section is not an empty one. A caller who cannot read
  // activities or the network would otherwise be told this relationship is
  // thin — and the thin state suppresses the ordinary modules, so they would
  // lose the rest of the page over data that may well exist. Absent for want
  // of a grant and absent for want of data are different facts (the same
  // distinction sections_omitted exists to carry).
  if (omitted(view, "activities") || omitted(view, "network")) {
    return false;
  }
  if (!view.activities || !view.network) {
    return false;
  }
  return (
    view.activities.data.length === 0 && view.network.colleagues.length === 0
  );
}

/**
 * ThinState is the whole answer when a record is thin: one honest sentence
 * about what IS known, and ONE way forward. It replaces the six empty cards
 * that used to render — an absence inventory tells the reader six times that
 * the CRM knows nothing, which is both true and useless.
 */
export function ThinState({
  view,
  onLogActivity,
}: Readonly<{ view: Contact360; onLogActivity?: () => void }>) {
  const t = useT();
  const employer = currentEmployer(view.employments?.data);
  const email = view.contact.emails?.[0]?.email;

  // The remediation is chosen by what is MISSING, so the page offers the one
  // step that would actually change the answer rather than a menu.
  const remediation = employer
    ? t("contact.thin.remediation.capture")
    : t("contact.thin.remediation.employer");

  return (
    <Panel title={t("contact.thin.title")}>
      <PanelBody>
        <p className="pe-panel-text">
          {t("contact.thin.known", {
            name: view.contact.full_name,
            what: [email, employer?.company_name].filter(Boolean).join(" · "),
          })}
        </p>
        <p className="pe-thin-remediation">{remediation}</p>
        {/* A bare `.btn` names no variant, and the variants are what carry the
            fill, the border and the ink — so this rendered transparent,
            borderless and unreadable against the plate behind it. It is the one
            move this surface offers, so it is the primary. */}
        {onLogActivity && (
          <Button
            variant="primary"
            className="thin-log-first"
            onClick={onLogActivity}
          >
            {t("contact.thin.logFirst")}
          </Button>
        )}
      </PanelBody>
    </Panel>
  );
}
