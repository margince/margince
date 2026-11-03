// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { ifMatch } from "../../api/version";
import { Badge, Button, EmptyState } from "../../design-system/atoms";
import { Checklist, ChecklistStep } from "../../design-system/checklist";
import { DataTable } from "../../design-system/datatable";
import { DateInput, isoOrBlank } from "../../design-system/dateinput";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../../design-system/drawerbands";
import { ErrorLine } from "../../design-system/errorline";
import { Checkbox, Field, TextInput } from "../../design-system/field";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { Select } from "../../design-system/select";
import { SetPicker } from "../../design-system/setpicker";
import { countryOptions } from "../../format/assignedcountries";
import { formatMoney, formatNumber } from "../../format/format";
import { INDUSTRY_CODES, industryLabelKey } from "../../format/industry";
import { useLocale, useT } from "../../i18n";
import { analyticsSectionHref } from "../../lib/analytics.address";
import { unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { useUrlParams } from "../../lib/urlstate";
import { useAnalyticsContext } from "./analytics.context";
import { useProductOptions } from "./reporting.dimensions";
import {
  INITIATIVES_KEY,
  initiativeDials,
  type SalesInitiative,
} from "./reporting.initiatives";
import { useMetricNames, useReportingCatalog } from "./reporting.queries";

type Input = components["schemas"]["SalesInitiativeInput"];
type Members = components["schemas"]["SalesInitiativeMembers"];
type MemberRef = components["schemas"]["SalesInitiativeMemberRef"];
type MetricID = components["schemas"]["ReportingMetricID"];

/** The objectives an initiative may be measured by, in the order offered. */
const OBJECTIVES: readonly MetricID[] = [
  "qualified_opportunities_created",
  "qualified_pipeline_created",
  "bookings_won",
  "open_pipeline",
];

type Step = "goal" | "members" | "review";

type Goal = Readonly<{
  name: string;
  objective: MetricID;
  target: string;
  deadline: string;
  includeFuture: boolean;
}>;

type Rule = Readonly<{ product: string; industry: string; country: string }>;

/** The rule as the predicate engine reads it: every set condition, AND-ed. */
function predicateOf(rule: Rule, recordType: "lead" | "deal") {
  const leaves: { field: string; op: "eq"; value: string }[] = [];
  if (rule.product)
    leaves.push({ field: "product_id", op: "eq", value: rule.product });
  if (rule.industry)
    leaves.push({
      field: recordType === "deal" ? "company_industry" : "industry",
      op: "eq",
      value: rule.industry,
    });
  if (rule.country)
    leaves.push({ field: "company_country", op: "eq", value: rule.country });
  return leaves.length === 0 ? undefined : { and: leaves };
}

function goalOf(initiative?: SalesInitiative): Goal {
  return {
    name: initiative?.name ?? "",
    objective:
      initiative?.objective_metric ?? "qualified_opportunities_created",
    target:
      initiative?.target_value == null ? "" : String(initiative.target_value),
    deadline: initiative?.deadline ?? "",
    includeFuture: initiative?.include_future ?? true,
  };
}

function ruleOf(initiative?: SalesInitiative): Rule {
  const tree = initiative?.deal_rule as
    | { and?: { field: string; value: string }[] }
    | undefined;
  const leaves = tree?.and ?? [];
  const value = (field: string) =>
    leaves.find((leaf) => leaf.field === field)?.value ?? "";
  return {
    product: value("product_id"),
    industry: value("company_industry"),
    country: value("company_country"),
  };
}

function bodyOf(goal: Goal, rule: Rule): Input {
  return {
    name: goal.name.trim(),
    objective_metric: goal.objective,
    target_value: goal.target.trim() === "" ? null : Number(goal.target),
    deadline: goal.deadline,
    include_future: goal.includeFuture,
    deal_rule: predicateOf(rule, "deal") ?? {},
    lead_rule: predicateOf(rule, "lead") ?? {},
  };
}

/**
 * The reader's picks and exclusions over the saved membership. Ticking a row
 * back on lifts its exclusion; a pick ticked off leaves the pick list, so the
 * server never sees one record both picked and excluded.
 */
function useMembership() {
  const [picked, setPicked] = useState<MemberRef[]>([]);
  const [excluded, setExcluded] = useState<MemberRef[]>([]);
  const excludedSet = new Set(excluded.map((ref) => ref.record_id));
  const without = (refs: MemberRef[], id: string) =>
    refs.filter((other) => other.record_id !== id);
  const toggle = (member: Members["data"][number], included: boolean) => {
    const ref: MemberRef = {
      record_type: member.record_type,
      record_id: member.record_id,
    };
    if (!included) {
      setPicked((prev) => without(prev, ref.record_id));
      setExcluded((prev) => [...prev, ref]);
      return;
    }
    setExcluded((prev) => without(prev, ref.record_id));
    const alreadyPicked = picked.some(
      (other) => other.record_id === ref.record_id,
    );
    if (member.reason === "picked" && !alreadyPicked)
      setPicked((prev) => [...prev, ref]);
  };
  return { picked, setPicked, excluded, excludedSet, toggle };
}

/** The drawer's foot: the one forward action of the step, and the way back. */
function DrawerActions({
  step,
  live,
  saved,
  goal,
  memberCount,
  pending,
  onClose,
  onStep,
  onSaveGoal,
  onSaveMembers,
  onLaunch,
}: Readonly<{
  step: Step;
  live: boolean;
  saved: SalesInitiative | undefined;
  goal: Goal;
  memberCount: number;
  pending: { goal: boolean; members: boolean; launch: boolean };
  onClose: () => void;
  onStep: (step: Step) => void;
  onSaveGoal: () => void;
  onSaveMembers: (current: SalesInitiative) => void;
  onLaunch: (current: SalesInitiative) => void;
}>) {
  const t = useT();
  return (
    <DrawerFoot>
      <Button variant="ghost" onClick={onClose}>
        {t("reporting.cancel")}
      </Button>
      {step === "goal" && (
        <Button
          variant="primary"
          pending={pending.goal}
          disabled={goal.name.trim() === "" || !isoOrBlank(goal.deadline)}
          onClick={onSaveGoal}
        >
          {t("reporting.chooseIncludedRecords")}
        </Button>
      )}
      {step === "members" && saved && (
        <>
          {!live && (
            <Button variant="ghost" onClick={() => onStep("goal")}>
              {t("reporting.backToGoal")}
            </Button>
          )}
          <Button
            variant="primary"
            pending={pending.members || pending.goal}
            disabled={memberCount === 0}
            onClick={() => onSaveMembers(saved)}
          >
            {live
              ? t("reporting.saveMembership")
              : t("reporting.reviewIncludedRecords")}
          </Button>
        </>
      )}
      {step === "review" && saved && (
        <>
          <Button variant="ghost" onClick={() => onStep("members")}>
            {t("reporting.backToIncludedRecords")}
          </Button>
          <Button
            variant="primary"
            pending={pending.launch}
            onClick={() => onLaunch(saved)}
          >
            {t("reporting.launchInitiative")}
          </Button>
        </>
      )}
    </DrawerFoot>
  );
}

/**
 * New sales initiative, in the prototype's order: the goal, the included
 * records with their reasons, then review and launch. Nothing is enrolled
 * before the launch; a draft is saved from the first step on, so a closed
 * window loses nothing. On a live initiative the drawer opens on the records.
 */
export function InitiativeDrawer({
  initiative,
  onClose,
}: Readonly<{ initiative?: SalesInitiative; onClose: () => void }>) {
  const t = useT();
  const titleId = useId();
  const queryClient = useQueryClient();
  const [params] = useUrlParams();
  const live = initiative?.status === "live";
  const [step, setStep] = useState<Step>(live ? "members" : "goal");
  const [saved, setSaved] = useState<SalesInitiative | undefined>(initiative);
  const [goal, setGoal] = useState<Goal>(() => goalOf(initiative));
  const [rule, setRule] = useState<Rule>(() => ruleOf(initiative));
  const { picked, setPicked, excluded, excludedSet, toggle } = useMembership();
  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: [INITIATIVES_KEY] });

  const saveGoal = useMutation({
    mutationFn: async ({
      body,
      current,
    }: {
      body: Input;
      current?: SalesInitiative;
    }) =>
      current
        ? unwrap(
            await api.PATCH("/analytics/initiatives/{id}", {
              params: { path: { id: current.id }, ...ifMatch(current.version) },
              body,
            }),
          )
        : unwrap(await api.POST("/analytics/initiatives", { body })),
    onSuccess: (next) => {
      setSaved(next);
      invalidate();
      setStep("members");
    },
  });
  const members = useQuery({
    queryKey: [INITIATIVES_KEY, saved?.id, "members", saved?.version],
    queryFn: async () =>
      unwrap(
        await api.GET("/analytics/initiatives/{id}/members", {
          params: { path: { id: saved?.id ?? "" } },
        }),
      ),
    enabled: saved !== undefined && step !== "goal",
  });
  const saveMembers = useMutation({
    mutationFn: async ({
      id,
      body,
    }: {
      id: string;
      body: { picked: MemberRef[]; excluded: MemberRef[] };
    }) =>
      unwrap(
        await api.PUT("/analytics/initiatives/{id}/members", {
          params: { path: { id } },
          body,
        }),
      ),
    onSuccess: () => {
      invalidate();
      queryClient.invalidateQueries({ queryKey: ["reporting-evaluation"] });
      if (live) onClose();
      else setStep("review");
    },
  });
  const launch = useMutation({
    mutationFn: async (current: SalesInitiative) =>
      unwrap(
        await api.POST("/analytics/initiatives/{id}/launch", {
          params: { path: { id: current.id }, ...ifMatch(current.version) },
        }),
      ),
    onSuccess: (launched) => {
      invalidate();
      onClose();
      navigate(
        { screen: "analytics", id: "overview" },
        initiativeDials(params, launched.id),
      );
    },
  });

  const failure = saveGoal.error ?? saveMembers.error ?? launch.error;
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="drawer">
      <DrawerHead>
        <Heading size="large" id={titleId} className="modal-title">
          {initiative ? initiative.name : t("reporting.newInitiative")}
        </Heading>
        <p className="t-caption">{t("reporting.initiativeDrawerPurpose")}</p>
        <Checklist variant="inline" label={t("reporting.initiativeSteps")}>
          <ChecklistStep
            state={stepState("goal", step)}
            title={t("reporting.stepGoal")}
          />
          <ChecklistStep
            state={stepState("members", step)}
            title={t("reporting.stepMembers")}
          />
          <ChecklistStep
            state={stepState("review", step)}
            title={t("reporting.stepReview")}
          />
        </Checklist>
      </DrawerHead>
      <DrawerBody>
        {step === "goal" && <GoalFields goal={goal} onChange={setGoal} />}
        {step === "members" && (
          <MemberFields
            rule={rule}
            onRule={setRule}
            members={members.data}
            pending={members.isPending && saved !== undefined}
            picked={picked}
            onPick={setPicked}
            excludedSet={excludedSet}
            onToggle={toggle}
            includeFuture={goal.includeFuture}
            onIncludeFuture={(includeFuture) =>
              setGoal((prev) => ({ ...prev, includeFuture }))
            }
          />
        )}
        {step === "review" && saved && (
          <Review initiative={saved} goal={goal} members={members.data} />
        )}
        <ErrorLine error={failure} />
      </DrawerBody>
      <DrawerActions
        step={step}
        live={live}
        saved={saved}
        goal={goal}
        memberCount={(members.data?.leads ?? 0) + (members.data?.deals ?? 0)}
        pending={{
          goal: saveGoal.isPending,
          members: saveMembers.isPending,
          launch: launch.isPending,
        }}
        onClose={onClose}
        onStep={setStep}
        onSaveGoal={() =>
          saveGoal.mutate({ body: bodyOf(goal, rule), current: saved })
        }
        onSaveMembers={async (current) => {
          const next = await saveGoal.mutateAsync({
            body: bodyOf(goal, rule),
            current,
          });
          saveMembers.mutate({ id: next.id, body: { picked, excluded } });
        }}
        onLaunch={(current) => launch.mutate(current)}
      />
    </Modal>
  );
}

function stepState(own: Step, current: Step): "done" | "current" | "open" {
  const order: Step[] = ["goal", "members", "review"];
  if (own === current) return "current";
  return order.indexOf(own) < order.indexOf(current) ? "done" : "open";
}

function GoalFields({
  goal,
  onChange,
}: Readonly<{ goal: Goal; onChange: (goal: Goal) => void }>) {
  const t = useT();
  const catalog = useReportingCatalog();
  const metricName = useMetricNames();
  const offered = OBJECTIVES.filter((id) =>
    catalog.data?.metrics.some((metric) => metric.id === id),
  );
  return (
    <>
      <Field label={t("reporting.initiativeName")}>
        {(field) => (
          <TextInput
            {...field}
            value={goal.name}
            placeholder={t("reporting.initiativeNamePlaceholder")}
            onChange={(event) =>
              onChange({ ...goal, name: event.target.value })
            }
          />
        )}
      </Field>
      <Field label={t("reporting.objective")}>
        {(field) => (
          <Select
            {...field}
            value={goal.objective}
            options={(offered.length > 0 ? offered : OBJECTIVES).map((id) => ({
              value: id,
              label: metricName(id),
            }))}
            onChange={(value) => {
              const objective = OBJECTIVES.find((id) => id === value);
              if (objective) onChange({ ...goal, objective });
            }}
          />
        )}
      </Field>
      <Field
        label={t("reporting.targetValue")}
        hint={t("reporting.targetHint")}
      >
        {(field) => (
          <TextInput
            {...field}
            inputMode="numeric"
            value={goal.target}
            onChange={(event) =>
              onChange({
                ...goal,
                target: event.target.value.replace(/\D/g, ""),
              })
            }
          />
        )}
      </Field>
      <Field label={t("reporting.deadline")}>
        {(field) => (
          <DateInput
            {...field}
            value={isoOrBlank(goal.deadline)}
            onChange={(event) =>
              onChange({ ...goal, deadline: event.target.value })
            }
          />
        )}
      </Field>
      <p className="t-caption">{t("reporting.nameDoesNotDecide")}</p>
    </>
  );
}

function MemberFields({
  rule,
  onRule,
  members,
  pending,
  picked,
  onPick,
  excludedSet,
  onToggle,
  includeFuture,
  onIncludeFuture,
}: Readonly<{
  rule: Rule;
  onRule: (rule: Rule) => void;
  members: Members | undefined;
  pending: boolean;
  picked: MemberRef[];
  onPick: (picked: MemberRef[]) => void;
  excludedSet: ReadonlySet<string>;
  onToggle: (member: Members["data"][number], included: boolean) => void;
  includeFuture: boolean;
  onIncludeFuture: (value: boolean) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const products = useProductOptions();
  const candidates = useQuery({
    queryKey: ["initiative-candidates"],
    queryFn: async () => {
      const [deals, leads] = await Promise.all([
        api.GET("/deals", {
          params: { query: { limit: 200, status: "open" } },
        }),
        api.GET("/leads", { params: { query: { limit: 200 } } }),
      ]);
      return {
        deals: unwrap(deals).data.map((deal) => ({
          value: deal.id,
          label: deal.name,
        })),
        leads: unwrap(leads).data.map((lead) => ({
          value: lead.id,
          label: lead.full_name || lead.company_name || lead.id,
        })),
      };
    },
    staleTime: 60_000,
  });
  const pickedOf = (type: "lead" | "deal") =>
    picked
      .filter((ref) => ref.record_type === type)
      .map((ref) => ref.record_id);
  const setPickedOf = (type: "lead" | "deal", ids: string[]) =>
    onPick([
      ...picked.filter((ref) => ref.record_type !== type),
      ...ids.map((record_id) => ({ record_type: type, record_id })),
    ]);
  const pickerWords = {
    removeLabel: (item: { label: string }) =>
      t("reporting.removePick", { name: item.label }),
    none: t("reporting.nonePicked"),
    addLabel: t("reporting.addRecord"),
    searchLabel: t("reporting.searchRecords"),
    noMatch: t("reporting.noMatchingRecords"),
    doneLabel: t("reporting.pickDone"),
  };
  return (
    <>
      <p>{t("reporting.membersPurpose")}</p>
      <Field label={t("reporting.product")}>
        {(field) => (
          <Select
            {...field}
            value={rule.product}
            options={[
              { value: "", label: t("reporting.anyProduct") },
              ...(products.data ?? []),
            ]}
            onChange={(product) => onRule({ ...rule, product })}
          />
        )}
      </Field>
      <Field label={t("reporting.industry")}>
        {(field) => (
          <Select
            {...field}
            value={rule.industry}
            options={[
              { value: "", label: t("reporting.anyIndustry") },
              ...INDUSTRY_CODES.map((code) => ({
                value: code,
                label: industryLabelKey(code)
                  ? t(industryLabelKey(code) ?? "reporting.industry")
                  : code,
              })),
            ]}
            onChange={(industry) => onRule({ ...rule, industry })}
          />
        )}
      </Field>
      <Field label={t("reporting.country")}>
        {(field) => (
          <Select
            {...field}
            value={rule.country}
            options={[
              { value: "", label: t("reporting.anyCountry") },
              ...countryOptions(locale),
            ]}
            onChange={(country) => onRule({ ...rule, country })}
          />
        )}
      </Field>
      <p className="t-caption">
        {predicateOf(rule, "deal")
          ? t("reporting.ruleAnd")
          : t("reporting.noConditions")}
      </p>
      <Checkbox
        label={t("reporting.includeFuture")}
        checked={includeFuture}
        onChange={(event) => onIncludeFuture(event.target.checked)}
      />
      <SetPicker
        legend={t("reporting.pickDeals")}
        options={candidates.data?.deals ?? []}
        values={pickedOf("deal")}
        onChange={(ids) => setPickedOf("deal", ids)}
        {...pickerWords}
      />
      <SetPicker
        legend={t("reporting.pickLeads")}
        options={candidates.data?.leads ?? []}
        values={pickedOf("lead")}
        onChange={(ids) => setPickedOf("lead", ids)}
        {...pickerWords}
      />
      <p className="t-caption">{t("reporting.pickExplains")}</p>
      {members && (
        <>
          <p>
            {t("reporting.memberCounts", {
              leads: formatNumber(members.leads, locale),
              deals: formatNumber(members.deals, locale),
              companies: formatNumber(members.companies, locale),
            })}
            {members.truncated && ` ${t("reporting.membersTruncated")}`}
          </p>
          <MembersTable
            members={members}
            excludedSet={excludedSet}
            onToggle={onToggle}
          />
        </>
      )}
      {pending && <p className="t-caption">{t("reporting.membersPending")}</p>}
      {!members && !pending && (
        <EmptyState>{t("reporting.saveGoalFirst")}</EmptyState>
      )}
    </>
  );
}

function MembersTable({
  members,
  excludedSet,
  onToggle,
}: Readonly<{
  members: Members;
  excludedSet: ReadonlySet<string>;
  onToggle: (member: Members["data"][number], included: boolean) => void;
}>) {
  const t = useT();
  return (
    <DataTable
      label={t("reporting.stepMembers")}
      rows={members.data}
      rowKey={(member) => `${member.record_type}:${member.record_id}`}
      columns={[
        {
          key: "include",
          header: t("reporting.include"),
          render: (member) => (
            <Checkbox
              label={t("reporting.includeRecord", { name: member.label })}
              checked={member.included && !excludedSet.has(member.record_id)}
              onChange={(event) => onToggle(member, event.target.checked)}
            />
          ),
        },
        {
          key: "record",
          header: t("reporting.memberRecord"),
          render: (member) => (
            <>
              {member.label}
              <p className="t-sub">
                {t(`reporting.recordType.${member.record_type}`)}
                {member.company_label ? ` · ${member.company_label}` : ""}
              </p>
            </>
          ),
        },
        {
          key: "reason",
          header: t("reporting.whyIncluded"),
          render: (member) => (
            <Badge>{t(`reporting.memberReason.${member.reason}`)}</Badge>
          ),
        },
      ]}
    />
  );
}

function Review({
  initiative,
  goal,
  members,
}: Readonly<{
  initiative: SalesInitiative;
  goal: Goal;
  members: Members | undefined;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [params] = useUrlParams();
  const metricName = useMetricNames();
  const context = useAnalyticsContext();
  const metricLabel = metricName(goal.objective);
  const value = (members?.data ?? [])
    .filter((member) => member.included && member.record_type === "deal")
    .reduce((sum, member) => sum + (member.amount_minor_base ?? 0), 0);
  return (
    <>
      <Heading size="small" as="h3">
        {metricLabel}
      </Heading>
      <p>
        {goal.target.trim() === ""
          ? t("reporting.trackingOnlyVerdict")
          : t("reporting.targetBy", {
              target: goal.target,
              deadline: goal.deadline,
            })}
      </p>
      {members && (
        <p>
          {t("reporting.memberCounts", {
            leads: formatNumber(members.leads, locale),
            deals: formatNumber(members.deals, locale),
            companies: formatNumber(members.companies, locale),
          })}
          {" · "}
          {t("reporting.knownDealValue", {
            value: formatMoney(
              value,
              context.data?.base_currency ?? "",
              locale,
            ),
          })}
        </p>
      )}
      <p className="t-caption">{t("reporting.baselineNote")}</p>
      <p className="t-caption">
        {goal.includeFuture
          ? t("reporting.futureJoins")
          : t("reporting.futureStays")}
      </p>
      {members && (
        <MembersTable
          members={{
            ...members,
            data: members.data.filter((member) => member.included),
          }}
          excludedSet={new Set()}
          onToggle={() => {}}
        />
      )}
      <p className="t-caption">
        {t("reporting.reviewAfterLaunch")}{" "}
        <a
          href={analyticsSectionHref(
            "overview",
            initiativeDials(params, initiative.id),
          )}
        >
          {initiative.name}
        </a>
      </p>
    </>
  );
}
