// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import { ifMatch } from "../../api/version";
import { Button } from "../../design-system/atoms";
import { Checklist, ChecklistStep } from "../../design-system/checklist";
import { isoOrBlank } from "../../design-system/dateinput";
import {
  DrawerBody,
  DrawerFoot,
  DrawerHead,
} from "../../design-system/drawerbands";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { formatMoney, formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { analyticsSectionHref } from "../../lib/analytics.address";
import { unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { useUrlParams } from "../../lib/urlstate";
import { useAnalyticsContext } from "./analytics.context";
import {
  bodyOf,
  type Goal,
  GoalFields,
  goalOf,
  type Input,
  type MemberRef,
  type Members,
  type Rule,
  ruleOf,
  type Step,
} from "./reporting.initiativegoal";
import { MemberFields, MembersTable } from "./reporting.initiativemembers";
import {
  INITIATIVES_KEY,
  initiativeDials,
  type SalesInitiative,
} from "./reporting.initiatives";
import { useMetricNames } from "./reporting.queries";

/**
 * The reader's picks and exclusions. They start from the saved membership, so
 * a save without a change keeps them. Ticking a row back on lifts its
 * exclusion. A pick ticked off leaves the pick list, so the server never
 * sees one record both picked and excluded.
 */
function useMembership(members: Members | undefined) {
  const [overrides, setOverrides] = useState<Overrides | null>(null);
  const current = overrides ?? savedOverrides(members);
  const { picked, excluded } = current;
  const excludedSet = new Set(excluded.map((ref) => ref.record_id));
  const without = (refs: MemberRef[], id: string) =>
    refs.filter((other) => other.record_id !== id);
  const setPicked = (next: MemberRef[]) =>
    setOverrides({ picked: next, excluded });
  const toggle = (member: Members["data"][number], included: boolean) => {
    const ref: MemberRef = {
      record_type: member.record_type,
      record_id: member.record_id,
    };
    if (!included) {
      setOverrides({
        picked: without(picked, ref.record_id),
        excluded: [...excluded, ref],
      });
      return;
    }
    const alreadyPicked = picked.some(
      (other) => other.record_id === ref.record_id,
    );
    setOverrides({
      picked:
        member.reason === "picked" && !alreadyPicked
          ? [...picked, ref]
          : picked,
      excluded: without(excluded, ref.record_id),
    });
  };
  return { picked, setPicked, excluded, excludedSet, toggle };
}

type Overrides = Readonly<{ picked: MemberRef[]; excluded: MemberRef[] }>;

function savedOverrides(members: Members | undefined): Overrides {
  const refs = (reason: "picked" | "excluded") =>
    (members?.data ?? [])
      .filter((member) => member.reason === reason)
      .map((member) => ({
        record_type: member.record_type,
        record_id: member.record_id,
      }));
  return { picked: refs("picked"), excluded: refs("excluded") };
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
 * New sales initiative, in three steps: the goal, the included records with
 * their reasons, then review and launch. Nothing is enrolled before the
 * launch. A draft is saved from the first step on, so a closed window loses
 * nothing. On a live initiative the drawer opens on the records.
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
  const { picked, setPicked, excluded, excludedSet, toggle } = useMembership(
    members.data,
  );
  const saveMembers = useMutation({
    mutationFn: async ({
      id,
      body,
    }: {
      id: string;
      body: { picked: MemberRef[]; excluded: MemberRef[] };
    }) => {
      unwrap(
        await api.PUT("/analytics/initiatives/{id}/members", {
          params: { path: { id } },
          body,
        }),
      );
      // The save moved the version, and the launch names it, so the
      // initiative is read again rather than kept from the goal's save.
      return unwrap(
        await api.GET("/analytics/initiatives/{id}", {
          params: { path: { id } },
        }),
      );
    },
    onSuccess: (next) => {
      setSaved(next);
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
        // A pick made here belongs before it is saved, so it counts beside
        // what the server already holds.
        memberCount={
          (members.data?.leads ?? 0) +
          (members.data?.deals ?? 0) +
          picked.length
        }
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
