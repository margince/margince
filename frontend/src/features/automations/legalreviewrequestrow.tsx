import {
  type QueryKey,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { api } from "../../api/client";
import { Button, Card } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import {
  type Approval,
  invalidateApprovalViews,
} from "../../lib/approvals.queries";
import { unwrap } from "../../lib/problem";
import { routeHash } from "../../lib/router";

/**
 * A legal review waiting for this reader. Nothing is decided here: the review is
 * answered on its own screen, so the row offers that screen and a way to clear
 * the row. Clearing records no answer, which is why the verbs of a decision are
 * absent and no Approved or Rejected status is shown.
 */
export function LegalReviewRequestRow({
  approval,
  decided,
  extraInvalidateKeys,
}: Readonly<{
  approval: Approval;
  decided?: boolean;
  extraInvalidateKeys?: readonly QueryKey[];
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const dismiss = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/approvals/{id}/approve", {
          params: { path: { id: approval.id } },
        }),
      ),
    onSuccess: () => {
      void invalidateApprovalViews(queryClient);
      for (const queryKey of extraInvalidateKeys ?? []) {
        queryClient.invalidateQueries({ queryKey });
      }
    },
  });
  return (
    <Card className="approval-row">
      <p className="t-body">{approval.summary}</p>
      {!decided && (
        <div className="row-actions">
          <a className="button" href={routeHash({ screen: "legal-reviews" })}>
            {t("legalReview.openQueue")}
          </a>
          <Button
            variant="ghost"
            disabled={dismiss.isPending}
            onClick={() => dismiss.mutate()}
          >
            {t("legalReview.dismissRequest")}
          </Button>
        </div>
      )}
      <ErrorLine error={dismiss.error} />
    </Card>
  );
}
