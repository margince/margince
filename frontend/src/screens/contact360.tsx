import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button } from "../design-system/atoms";
import { EvidenceMark } from "../design-system/evidencemark";
import { FactList } from "../design-system/factlist";
import { Panel, PanelBody } from "../design-system/panel";
import type { ConfidenceLevel } from "../design-system/trust";
import { useT } from "../i18n";
import { provenanceOf, throwProblem } from "./common";
import { currentEmployer, stillHeld } from "./employmentcurrency";
import { EntityRef } from "./entityref";
import { dealRoleLabel } from "./record360";
import "./contact360.css";

export type Contact360 = components["schemas"]["Contact360"];
type ProfileField = components["schemas"]["ContactProfileField"];

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

/**
 * IdentityRail is what the record IS: contact methods, current employment,
 * buying roles, and the career history that a re-read must never overwrite.
 *
 * Every enriched value carries its receipt — the verbatim text it was read
 * from — because a field the reader cannot check is a claim, not a fact.
 */
export function IdentityRail({
  view,
  children,
}: Readonly<{ view: Contact360; children?: ReactNode }>) {
  const t = useT();
  const byField = new Map<string, ProfileField>(
    (view.profile_fields ?? []).map((f) => [f.field, f]),
  );
  const current = currentEmployer(view.employments?.data);
  const career = (view.employments?.data ?? []).filter(
    (e) => e.relationship_id !== current?.relationship_id,
  );

  return (
    <>
      <Panel title={t("contact.identity.title")}>
        <PanelBody>
          <FactList
            facts={[
              ...(view.contact.emails ?? []).map((e) => ({
                key: `email-${e.id}`,
                term: t("contact.identity.email"),
                // The dead marker is DERIVED from the send ledger — the
                // latest delivery to this address hard-bounced and nothing
                // has arrived since — so a later send that works clears it
                // on its own. Absent section (no activity grant) marks
                // nothing rather than guessing.
                value: (view.dead_addresses ?? []).includes(e.email) ? (
                  <>
                    {e.email}{" "}
                    <Badge tone="danger">
                      {t("contact.identity.emailDead")}
                    </Badge>
                  </>
                ) : (
                  e.email
                ),
              })),
              ...(view.contact.phones ?? []).map((p) => ({
                key: `phone-${p.id}`,
                term: t("contact.identity.phone"),
                value: (
                  <Evidenced value={p.phone} field={byField.get("phone")} />
                ),
              })),
              ...(current
                ? [
                    {
                      key: "current-role",
                      term: t("contact.identity.currentRole"),
                      value: (
                        <>
                          <Evidenced
                            value={current.role ?? view.contact.title ?? "—"}
                            field={byField.get("role") ?? byField.get("title")}
                          />
                          {current.company_name && (
                            <>
                              {" · "}
                              <EntityRef
                                kind="company"
                                id={current.company_id}
                              />
                            </>
                          )}
                        </>
                      ),
                    },
                  ]
                : []),
              ...(view.deal_roles?.data ?? []).map((r) => ({
                key: `deal-role-${r.relationship_id}`,
                term: t("contact.identity.buyingRole"),
                value: (
                  <>
                    {/* The wire spells a buying role `economic_buyer`; the
                        product has always had the words for it, one screen
                        over on the account this deal belongs to. */}
                    <Badge tone="accent">{dealRoleLabel(r.role, t)}</Badge>
                    {/* The deal, as a link. `deal_id` has always been on this
                        payload and the title was printed as text beside it,
                        which left the one row on this card naming a record the
                        reader could not open — while the employment row above
                        links the company through the same component. */}
                    {r.deal_title && (
                      <>
                        {" · "}
                        <EntityRef
                          kind="deal"
                          id={r.deal_id}
                          name={r.deal_title}
                        />
                      </>
                    )}
                  </>
                ),
              })),
            ]}
          />
        </PanelBody>
      </Panel>

      {career.length > 0 && (
        <Panel title={t("contact.career.title")}>
          <PanelBody>
            <ul className="pe-career">
              {career.map((e) => (
                <li key={e.relationship_id} className="pe-career-item">
                  {/* EntityRef preserves the company link and handles withheld names. */}
                  <EntityRef
                    kind="company"
                    id={e.company_id}
                    name={e.company_name}
                  />
                  {e.role && <> · {e.role}</>} ·{" "}
                  {t(
                    stillHeld(e)
                      ? "employment.status.current"
                      : e.employment_status === "unknown"
                        ? "employment.status.unknown"
                        : "employment.status.former",
                  )}
                </li>
              ))}
            </ul>
          </PanelBody>
        </Panel>
      )}

      <ConsentGuard view={view} />
      {children}
    </>
  );
}

/**
 * ConsentGuard compresses the consent module to what the reader needs
 * BEFORE acting: whether an outbound message is allowed. The per-purpose
 * proof log stays one click away — it is the ledger, this is the guard.
 */
function ConsentGuard({ view }: Readonly<{ view: Contact360 }>) {
  const t = useT();
  if (!view.consent) {
    return null;
  }
  const granted = view.consent.state.filter((s) => s.state === "granted");
  const blocked = view.consent.state.filter((s) => s.state !== "granted");
  return (
    <Panel title={t("contact.consent.title")}>
      <PanelBody>
        <p className="pe-panel-text">
          {granted.length > 0
            ? t("contact.consent.allowed", {
                purposes: granted.map((g) => g.purpose_key ?? "").join(", "),
              })
            : t("contact.consent.noneGranted")}
        </p>
        {blocked.length > 0 && (
          <p className="pe-consent-blocked">
            {t("contact.consent.blocked", {
              purposes: blocked.map((b) => b.purpose_key ?? "").join(", "),
            })}
          </p>
        )}
      </PanelBody>
    </Panel>
  );
}

/**
 * Evidenced renders a value with its receipt when one exists. Without
 * evidence it renders the plain value — the mark is never decoration, it
 * means "there is a source and you can read it".
 */
function Evidenced({
  value,
  field,
}: Readonly<{ value: string; field?: ProfileField }>) {
  if (!field) {
    return <>{value}</>;
  }
  return (
    <EvidenceMark
      value={value}
      source={{
        provenance: provenanceOf(field.captured_by, undefined),
        confidence: confidenceBand(field.confidence),
        snippet: field.evidence_snippet,
        at: field.captured_at,
      }}
    />
  );
}

/**
 * confidenceBand renders the stored 0..1 score in the three words the design
 * system speaks. A number on screen would invite arithmetic the reader
 * cannot check; the band says how much to lean on the value.
 */
function confidenceBand(score?: number | null): ConfidenceLevel | undefined {
  if (score === undefined || score === null) {
    return undefined;
  }
  if (score >= 0.8) {
    return "high";
  }
  return score >= 0.5 ? "med" : "low";
}
