import { useMemo } from "react";
import { api } from "../../api/client";
import type { components, paths } from "../../api/schema";
import { useRecordZone } from "../../app/recordzone";
import { Badge } from "../../design-system/atoms";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { throwProblem } from "../common";
import { CoverageExplorer } from "../coverageexplorer";
import {
  type ListPage,
  type ListQuery,
  ListTable,
  listFetchLimit,
  useListQuery,
} from "../listquery";

// The company's people, as a list a rep works from rather than a roster they
// read.
//
// The 360's people section answers "who works here" in twenty-five rows. This
// answers the question a rep actually opens the tab with — who do I write to
// next — over the whole account, which is why it is a list surface with the
// account's own filters rather than a longer card.

type OrganizationContact = components["schemas"]["OrganizationContact"];
type Engagement = components["schemas"]["ContactEngagement"];
type ContactStatus = Engagement | undefined;
type ContactSort =
  | NonNullable<
      NonNullable<
        paths["/organizations/{id}/contacts"]["get"]["parameters"]["query"]
      >["sort"]
    >
  | undefined;

/**
 * ENGAGEMENT_TONES gives each state its own weight, because the three are not
 * degrees of one thing.
 *
 * Answered is the way in and reads as success. No-reply is the one that needs a
 * reason before acting, so it carries the warning tone. Untried is neutral on
 * purpose: nobody has done anything wrong, and colouring it like a problem is
 * what makes a rep skip the easiest opening on a stalled account.
 */
const ENGAGEMENT_TONES: Record<Engagement, "success" | "warn" | undefined> = {
  answered: "success",
  no_reply: "warn",
  untried: undefined,
};

const ENGAGEMENT_LABELS: Record<
  Engagement,
  "co.reach.answered" | "co.reach.silent" | "co.reach.untried"
> = {
  answered: "co.reach.answered",
  no_reply: "co.reach.silent",
  untried: "co.reach.untried",
};

export function CompanyPeopleList({ orgId }: { readonly orgId: string }) {
  const t = useT();

  // Bound to the account, so switching company starts its own list rather than
  // carrying the previous one's page and filters.
  const fetchPage = useMemo(
    () =>
      async (
        query: ListQuery,
        cursor: string | null,
      ): Promise<ListPage<OrganizationContact>> => {
        // The declared filters, named one by one rather than spread. `filters`
        // carries whatever the address holds, and the address is the reader's
        // to edit: a spread after the paging keys lets `?cursor=` or `?limit=`
        // from a pasted URL overwrite the ones this function just computed,
        // which breaks paging with a 422 the reader cannot explain.
        const { data, error } = await api.GET("/organizations/{id}/contacts", {
          params: {
            path: { id: orgId },
            query: {
              q: query.q || undefined,
              sort: (query.sort || undefined) as ContactSort,
              status: query.filters.status as ContactStatus,
              cursor: cursor || undefined,
              limit: listFetchLimit(query.perPage),
            },
          },
        });
        // `throwProblem` returns never, so returning it is what tells the
        // compiler `data` is present below — an `if (error) { … }` block leaves
        // the pair unnarrowed and invites the non-null assertions this file
        // must not carry.
        if (error || !data) {
          return throwProblem(error);
        }
        return {
          data: data.data,
          page: {
            next_cursor: data.page.next_cursor ?? null,
            has_more: data.page.has_more,
          },
        };
      },
    [orgId],
  );

  const state = useListQuery<OrganizationContact>({
    key: `company-contacts:${orgId}`,
    // The server's own order, which is the recommendation. Naming a sort here
    // would make the page open on an alphabet and bury the person who answered.
    initialSort: "",
    fetchPage,
  });

  return (
    <ListTable
      state={state}
      unit="unit.contacts"
      searchable
      showArchivedToggle={false}
      // The colleague-by-contact comparison, kept as the diagnostic it is
      // rather than a headline: it answers "where are we thin across the
      // team", which a reader asks after choosing somebody, not before.
      tools={
        <CoverageExplorer
          orgId={orgId}
          contacts={state.rows.map((row) => ({
            person_id: row.person_id,
            full_name: row.full_name,
          }))}
        />
      }
      chips={[
        {
          key: "status",
          label: "co.people.filter.status",
          allLabel: "co.people.filter.statusAll",
          options: [
            { value: "answered", label: "co.reach.answered" },
            { value: "no_reply", label: "co.reach.silent" },
            { value: "untried", label: "co.reach.untried" },
          ],
        },
      ]}
      columns={[
        {
          key: "name",
          header: t("people.name"),
          cell: (contact: OrganizationContact) => (
            <span>
              <strong>{contact.full_name}</strong>
              {contact.title && (
                <span className="t-caption"> · {contact.title}</span>
              )}
            </span>
          ),
          sort: "name",
          fixed: true,
        },
        {
          key: "engagement",
          header: t("co.people.engagement"),
          cell: (contact: OrganizationContact) => (
            <Badge tone={ENGAGEMENT_TONES[contact.engagement]}>
              {t(ENGAGEMENT_LABELS[contact.engagement])}
            </Badge>
          ),
        },
        {
          key: "last_interaction",
          header: t("co.people.lastInteraction"),
          cell: (contact: OrganizationContact) => (
            <LastTouch contact={contact} />
          ),
          sort: "last_interaction",
          numeric: true,
        },
        {
          key: "strength",
          header: t("co.people.strength"),
          cell: (contact: OrganizationContact) => (
            <span className="t-caption">
              {t(`strength.bucket.${contact.strength.bucket}`)}
            </span>
          ),
          sort: "strength",
          numeric: true,
        },
      ]}
      rowKey={(contact: OrganizationContact) => contact.person_id}
      rowRoute={(contact: OrganizationContact) => ({
        screen: "contacts",
        id: contact.person_id,
      })}
    />
  );
}

/**
 * LastTouch prints which way the conversation is owed, not just when it moved.
 *
 * A date alone cannot say that: "12 March" is the same string whether they
 * wrote or we did, and those are opposite next moves. The direction is the
 * fact worth the column.
 */
function LastTouch({ contact }: { readonly contact: OrganizationContact }) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const inbound = contact.last_inbound_at;
  const outbound = contact.last_outbound_at;
  if (!inbound && !outbound) {
    return <span className="t-caption">{t("co.people.neverInTouch")}</span>;
  }
  const theyWroteLast =
    inbound && (!outbound || new Date(inbound) > new Date(outbound));
  const at = theyWroteLast ? inbound : outbound;
  return (
    <span className="t-caption">
      {theyWroteLast ? t("co.people.theyWrote") : t("co.people.weWrote")}
      {at && ` · ${formatDate(at, locale, recordZone)}`}
    </span>
  );
}
