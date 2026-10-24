import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Field, PendingBody } from "../../design-system/atoms";
import { DateInput, type ISODate } from "../../design-system/dateinput";
import { ErrorLine } from "../../design-system/errorline";
import { FactList } from "../../design-system/factlist";
import { formatDate } from "../../format/format";
import { ruleText } from "../../format/jurisdiction";
import { endOfDayInZone, startOfDayInZone } from "../../format/timezone";
import { useLocale, useT } from "../../i18n";
import { throwProblem } from "../../lib/problem";

// The deadline a manual subject request gets. The server computes it from the
// installation's jurisdiction and the day the request arrived. The form shows
// that date and its rule. It sends a date only when the officer picks an
// earlier one.

type DsrKind = components["schemas"]["CreateDataSubjectRequest"]["kind"];
type RequestDeadline = components["schemas"]["RequestDeadline"];

export type DeadlineDraft = Readonly<{
  /** The day the request arrived; "" is today. */
  receivedOn: ISODate | "";
  /** An earlier deadline the officer chose; "" takes the computed one. */
  earlierDue: ISODate | "";
}>;

export const EMPTY_DEADLINE: DeadlineDraft = { receivedOn: "", earlierDue: "" };

/** received_at for a draft: the start of the chosen day, or absent for today. */
export function receivedAtOf(
  draft: DeadlineDraft,
  zone: string,
): string | undefined {
  return draft.receivedOn === ""
    ? undefined
    : startOfDayInZone(draft.receivedOn, zone);
}

/**
 * due_at for a draft. Absent unless the officer picked a day, and never later
 * than the computed deadline. The end of the picked day can pass an exact
 * 72-hour deadline. Such a day sends nothing, and the computed deadline holds.
 */
export function dueAtOf(
  draft: DeadlineDraft,
  computed: RequestDeadline | undefined,
  zone: string,
): string | undefined {
  if (draft.earlierDue === "" || computed === undefined) return undefined;
  const picked = endOfDayInZone(draft.earlierDue, zone);
  return Date.parse(picked) < Date.parse(computed.due_at) ? picked : undefined;
}

/** Whether a picked day starts after the computed deadline, which the server refuses. */
export function earlierDueTooLate(
  draft: DeadlineDraft,
  computed: RequestDeadline | undefined,
  zone: string,
): boolean {
  if (draft.earlierDue === "" || computed === undefined) return false;
  return (
    Date.parse(startOfDayInZone(draft.earlierDue, zone)) >
    Date.parse(computed.due_at)
  );
}

/** The deadline fields of a create body, from a draft and its computed deadline. */
export function deadlineBody(
  draft: DeadlineDraft,
  computed: RequestDeadline,
  zone: string,
) {
  return {
    received_at: receivedAtOf(draft, zone),
    due_at: dueAtOf(draft, computed, zone),
  };
}

export function useRequestDeadline(
  kind: DsrKind,
  draft: DeadlineDraft,
  zone: string,
) {
  const receivedAt = receivedAtOf(draft, zone);
  return useQuery({
    queryKey: ["dsr-deadline", kind, receivedAt ?? "now"],
    queryFn: async () => {
      const { data, error, response } = await api.GET(
        "/data-subject-requests/deadline",
        {
          params: { query: { kind, received_at: receivedAt } },
        },
      );
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

export function DsrDeadlineFields({
  draft,
  query,
  refused,
  zone,
  onChange,
}: Readonly<{
  draft: DeadlineDraft;
  query: ReturnType<typeof useRequestDeadline>;
  /** The server refused the date sent, which the field then says. */
  refused: boolean;
  zone: string;
  onChange: (next: Partial<DeadlineDraft>) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const computed = query.data;
  const tooLate = refused || earlierDueTooLate(draft, computed, zone);
  return (
    <>
      <Field label={t("privacy.receivedAt")} hint={t("privacy.receivedAtHint")}>
        {(control) => (
          <DateInput
            {...control}
            value={draft.receivedOn}
            onChange={(event) => onChange({ receivedOn: event.target.value })}
          />
        )}
      </Field>
      {computed ? (
        <FactList
          facts={[
            {
              key: "computed",
              term: t("privacy.deadline.computed"),
              value: formatDate(computed.due_at, locale, zone),
              note: t("privacy.deadline.computedHint", {
                rule: ruleText(computed.rule, t),
              }),
            },
          ]}
        />
      ) : query.isPending ? (
        <PendingBody label={t("privacy.deadline.loading")} lines={1} visible />
      ) : (
        <ErrorLine error={query.error} />
      )}
      <Field
        label={t("privacy.earlierDue")}
        hint={tooLate ? undefined : t("privacy.earlierDueHint")}
        error={
          tooLate && computed
            ? t("privacy.earlierDueTooLate", {
                date: formatDate(computed.due_at, locale, zone),
              })
            : undefined
        }
      >
        {(control) => (
          <DateInput
            {...control}
            value={draft.earlierDue}
            onChange={(event) => onChange({ earlierDue: event.target.value })}
          />
        )}
      </Field>
    </>
  );
}
