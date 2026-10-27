import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { ifMatch, requireVersion } from "../../api/version";
import { SUBMIT_COPY, type SubmitIntent } from "../../components/create";
import {
  Button,
  EmptyState,
  Field,
  SectionHeader,
  TextInput,
} from "../../design-system/atoms";
import { DateInput, isoOrBlank } from "../../design-system/dateinput";
import { ErrorLine } from "../../design-system/errorline";
import { refuseUnreadableField } from "../../design-system/formvalidity";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { FieldGuard } from "../../design-system/rbac";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import {
  useCanWrite,
  useGrantRefused,
  useWriteRefusal,
} from "../../lib/capability";
import { isOption } from "../../lib/options";
import { throwProblem, unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { PartnerCommissions } from "./partnercommissions";
import { PartnerDeals } from "./partnerdeals";
import {
  CERT_LABELS,
  CERT_STATUSES,
  type CertStatus,
  MARGIN_TIER_LABELS,
  MARGIN_TIERS,
  type MarginTier,
  PARTNER_ROLES,
  type Partner,
  type PartnerRole,
  RELATIONSHIP_STAGES,
  type RelationshipStage,
  ROLE_LABELS,
  STAGE_LABELS,
  type UpsertPartnerRequest,
} from "./partners.vocab";

async function fetchPartner(companyId: string): Promise<Partner | null> {
  const { data, error, response } = await api.GET("/companies/{id}/partner", {
    params: { path: { id: companyId } },
  });
  if (response.status === 404) {
    return null;
  }
  if (error) {
    throwProblem(error);
  }
  return data ?? null;
}

type PartnerFormValues = {
  partner_role: PartnerRole;
  cert_status: CertStatus;
  margin_tier: "" | MarginTier;
  relationship_stage: RelationshipStage;
  next_step: string;
  next_step_due_at: string;
  served_segments: string;
};

function defaultFormValues(partner?: Partner): PartnerFormValues {
  return {
    partner_role: partner?.partner_role ?? "hosting",
    cert_status: partner?.cert_status ?? "applied",
    margin_tier: partner?.margin_tier ?? "",
    relationship_stage: partner?.relationship_stage ?? "research",
    next_step: partner?.next_step ?? "",
    next_step_due_at: partner?.next_step_due_at ?? "",
    served_segments: (partner?.served_segments ?? []).join(", "),
  };
}

// A seat without the partner grant leaves the tier out of the body. The server
// takes the grant for any tier the body names, a null included.
function buildUpsertBody(
  values: PartnerFormValues,
  setsTier: boolean,
): UpsertPartnerRequest {
  const segments = values.served_segments
    .split(",")
    .map((segment) => segment.trim())
    .filter((segment) => segment.length > 0);
  return {
    partner_role: values.partner_role,
    cert_status: values.cert_status,
    ...(setsTier ? { margin_tier: values.margin_tier || null } : {}),
    relationship_stage: values.relationship_stage,
    next_step: values.next_step.trim() || undefined,
    next_step_due_at: values.next_step_due_at || undefined,
    served_segments: segments.length > 0 ? segments : undefined,
  };
}

// The one form both "make this a partner" and "edit partner" render. They
// differ only in the record they prefill from and whether the PUT carries
// If-Match. First creation has no prior version to pin, so it sends none.
function PartnerForm({
  companyId,
  partner,
  onSaved,
  onCancel,
  intent,
}: Readonly<{
  companyId: string;
  partner?: Partner;
  onSaved: () => void;
  onCancel?: () => void;
  intent: SubmitIntent;
}>) {
  const t = useT();
  // This form mounts only while editing (PartnerDetail/PartnerTab remount it
  // each time `editing` flips true), so the lazy initializer is all the
  // seeding it needs. A re-sync effect keyed on `partner` would re-run on a
  // background refetch of ["partner", companyId] and overwrite the user's
  // typing.
  const [values, setValues] = useState<PartnerFormValues>(() =>
    defaultFormValues(partner),
  );
  const setsTier = useCanWrite("partner", "update");
  const tierRefused = useGrantRefused({ object: "partner", action: "update" });

  const mutation = useMutation({
    // The prior row rides the variables rather than the closure, and its
    // presence decides the precondition. First creation has no version to pin.
    // A replacement pins the one the form was filled from, and refuses rather
    // than upserting over an edit it never saw.
    mutationFn: async (prior: Partner | undefined) => {
      return unwrap(
        await api.PUT("/companies/{id}/partner", {
          params: {
            path: { id: companyId },
            ...(prior === undefined
              ? {}
              : ifMatch(requireVersion(prior.version))),
          },
          body: buildUpsertBody(values, setsTier),
        }),
      );
    },
    onSuccess: onSaved,
  });

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        if (!refuseUnreadableField(event.currentTarget)) {
          mutation.mutate(partner);
        }
      }}
      className="form-stack"
    >
      <Field label={t("partner.role")} required>
        {(control) => (
          <Select
            {...control}
            value={values.partner_role}
            onChange={(value) =>
              isOption(value, PARTNER_ROLES) &&
              setValues({ ...values, partner_role: value })
            }
            options={PARTNER_ROLES.map((role) => ({
              value: role,
              label: t(ROLE_LABELS[role]),
            }))}
          />
        )}
      </Field>
      <Field label={t("partner.certStatus")}>
        {(control) => (
          <Select
            {...control}
            value={values.cert_status}
            onChange={(value) =>
              isOption(value, CERT_STATUSES) &&
              setValues({ ...values, cert_status: value })
            }
            options={CERT_STATUSES.map((status) => ({
              value: status,
              label: t(CERT_LABELS[status]),
            }))}
          />
        )}
      </Field>
      <Field
        label={t("partner.marginTier")}
        hint={t(
          tierRefused ? "partner.marginTierRefused" : "partner.marginTierHint",
        )}
      >
        {(control) => (
          <Select
            {...control}
            disabled={!setsTier}
            value={values.margin_tier}
            onChange={(value) =>
              setValues({
                ...values,
                margin_tier: isOption(value, MARGIN_TIERS) ? value : "",
              })
            }
            // The clearing entry is a real choice, not a placeholder. A tier once
            // set has to be clearable back to "no tier agreed" (the wire's null).
            // It carries a label, because a blank one is an unreadable strip in a
            // drawn list and says nothing to a screen reader.
            options={[
              { value: "", label: t("field.unset") },
              ...MARGIN_TIERS.map((tier) => ({
                value: tier,
                label: t(MARGIN_TIER_LABELS[tier]),
              })),
            ]}
          />
        )}
      </Field>
      <Field label={t("partner.stage")}>
        {(control) => (
          <Select
            {...control}
            value={values.relationship_stage}
            onChange={(value) =>
              isOption(value, RELATIONSHIP_STAGES) &&
              setValues({ ...values, relationship_stage: value })
            }
            options={RELATIONSHIP_STAGES.map((stage) => ({
              value: stage,
              label: t(STAGE_LABELS[stage]),
            }))}
          />
        )}
      </Field>
      <Field label={t("partner.nextStep")}>
        {(control) => (
          <TextInput
            {...control}
            value={values.next_step}
            onChange={(event) =>
              setValues({ ...values, next_step: event.target.value })
            }
          />
        )}
      </Field>
      <Field label={t("partner.nextStepDue")}>
        {(control) => (
          <DateInput
            {...control}
            value={isoOrBlank(values.next_step_due_at)}
            onChange={(event) =>
              setValues({ ...values, next_step_due_at: event.target.value })
            }
          />
        )}
      </Field>
      <Field
        label={t("partner.servedSegments")}
        hint={t("partner.servedSegmentsHint")}
      >
        {(control) => (
          <TextInput
            {...control}
            value={values.served_segments}
            onChange={(event) =>
              setValues({ ...values, served_segments: event.target.value })
            }
          />
        )}
      </Field>
      <ErrorLine error={mutation.error} />
      <div className="form-actions">
        {onCancel && (
          <Button type="button" onClick={onCancel}>
            {t("create.cancel")}
          </Button>
        )}
        <Button
          variant="primary"
          type="submit"
          pending={mutation.isPending}
          busyLabel={t(SUBMIT_COPY[intent].busy)}
        >
          {t(SUBMIT_COPY[intent].label)}
        </Button>
      </div>
    </form>
  );
}

// Withheld is not empty. A role that may not read the commercial terms gets
// the tier as null with `masked_fields` naming it. A row that vanished would
// say this partner agreed no tier, so no row means only that none was agreed.
function MarginTierRow({ partner }: Readonly<{ partner: Partner }>) {
  const t = useT();
  const masked = partner.masked_fields?.includes("margin_tier") ?? false;
  const tier = partner.margin_tier;
  if (!masked && !tier) {
    return null;
  }
  return (
    <>
      <dt>{t("partner.marginTier")}</dt>
      <dd>
        <FieldGuard mode={masked ? "masked" : "visible"}>
          {tier ? t(MARGIN_TIER_LABELS[tier]) : null}
        </FieldGuard>
      </dd>
    </>
  );
}

function PartnerDetail({
  companyId,
  partner,
  refusal,
  onSaved,
}: Readonly<{
  companyId: string;
  partner: Partner;
  refusal: string | undefined;
  onSaved: () => void;
}>) {
  const t = useT();
  const [editing, setEditing] = useState(false);

  if (editing) {
    return (
      <div>
        <SectionHeader title={t("partner.edit")} />
        <PartnerForm
          companyId={companyId}
          partner={partner}
          intent="save"
          onCancel={() => setEditing(false)}
          onSaved={() => {
            setEditing(false);
            onSaved();
          }}
        />
      </div>
    );
  }

  return (
    <Panel
      title={t("tab.partner")}
      titleAction={
        <Button
          onClick={() => setEditing(true)}
          reason={refusal}
          data-testid="edit-partner"
        >
          {t("record.edit")}
        </Button>
      }
    >
      <PanelBody>
        <dl className="detail-grid">
          {partner.partner_role && (
            <>
              <dt>{t("partner.role")}</dt>
              <dd>{t(ROLE_LABELS[partner.partner_role])}</dd>
            </>
          )}
          <dt>{t("partner.certStatus")}</dt>
          <dd>{t(CERT_LABELS[partner.cert_status])}</dd>
          <MarginTierRow partner={partner} />
          <dt>{t("partner.stage")}</dt>
          <dd>{t(STAGE_LABELS[partner.relationship_stage])}</dd>
          {partner.next_step && (
            <>
              <dt>{t("partner.nextStep")}</dt>
              <dd>{partner.next_step}</dd>
            </>
          )}
          {partner.served_segments && partner.served_segments.length > 0 && (
            <>
              <dt>{t("partner.servedSegments")}</dt>
              <dd>{partner.served_segments.join(", ")}</dd>
            </>
          )}
        </dl>
      </PanelBody>
    </Panel>
  );
}

// The Partner tab. A company is a partner when it has a `partner` row, so a 404
// from GET /companies/{id}/partner means "not a partner yet", not an error. It
// renders the setup form instead of the shared error state. Setup and edit
// both PUT the same UpsertPartnerRequest. The first creation carries no
// If-Match, since there is no prior version; an edit carries the partner's
// own `version`.
export function PartnerTab({ companyId }: Readonly<{ companyId: string }>) {
  const t = useT();
  const queryClient = useQueryClient();
  // Every partner write needs the company grant; the tier alone also needs
  // the partner grant, which the form asks for itself.
  const writeRefusal = useWriteRefusal("company", "update", {
    seat: t("settings.seatRefused"),
    grant: t("partner.writeRefused"),
  });
  const refusal = useMe().data?.authorization ? writeRefusal : undefined;
  const query = useQuery({
    queryKey: ["partner", companyId],
    queryFn: () => fetchPartner(companyId),
  });

  function invalidateAfterSave() {
    queryClient.invalidateQueries({ queryKey: ["partner", companyId] });
    queryClient.invalidateQueries({
      queryKey: ["company", companyId],
    });
    queryClient.invalidateQueries({ queryKey: ["companies"] });
    // The deal form asks this list whether a partner programme exists at all,
    // so making the first partner has to reach it. Otherwise its fields stay
    // absent there until the cache goes stale, and the setting looks lost.
    queryClient.invalidateQueries({ queryKey: ["partners"] });
  }

  return (
    <QueryGate query={query} pendingLabel={t("nav.partners")}>
      {(partner) =>
        partner ? (
          // The record's own stack. This tab is three panes, and the work
          // column draws its children with no interval. As bare siblings they
          // would meet at the border and read as one card ruled into thirds.
          <div className="record-stack">
            <PartnerDetail
              companyId={companyId}
              partner={partner}
              refusal={refusal}
              onSaved={invalidateAfterSave}
            />
            {/* The work, then the money it produced. These deals belong to the
                customers, so the account's own Deals tab never shows them and
                this is the only page they surface on. */}
            <PartnerDeals companyId={companyId} />
            {/* What the tier above has actually produced. A margin tier with no
                money beside it is a number nobody can check. */}
            <PartnerCommissions companyId={companyId} />
          </div>
        ) : (
          // Two bodies: the panel's own seam divides what there is none of
          // from how the first one is made.
          <Panel title={t("tab.partner")}>
            <PanelBody>
              <EmptyState>{t("partner.none")}</EmptyState>
            </PanelBody>
            <PanelBody>
              <SectionHeader title={t("partner.setup")} />
              {refusal ? (
                <p>{refusal}</p>
              ) : (
                <PartnerForm
                  companyId={companyId}
                  intent="create"
                  onSaved={invalidateAfterSave}
                />
              )}
            </PanelBody>
          </Panel>
        )
      }
    </QueryGate>
  );
}
