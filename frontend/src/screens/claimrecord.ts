import { useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import { ifMatch, requireVersion } from "../api/version";
import { throwProblem } from "./common";

/** The record kinds the claim endpoint accepts. */
export type ClaimableRecordType = "organization" | "person" | "lead" | "deal";

// useClaimRecord is the claim door: POST /records/{type}/{id}/claim makes the
// caller the owner of an unowned record (or re-confirms one already theirs)
// and refreshes what shows it. Used wherever an owner control lets a reader
// pick themselves on a record nobody owns.
//
// Its own module rather than a company screen's export, because it is not the
// company's: an ownerless record of every claimable kind needs this same door,
// and the one that lived beside CompanyOwnerControl was reused by nothing for
// exactly as long as it looked like company code.
export function useClaimRecord(
  recordType: ClaimableRecordType,
  id: string,
  version: number | undefined,
) {
  const queryClient = useQueryClient();
  return async () => {
    const { error } = await api.POST("/records/{record_type}/{id}/claim", {
      params: {
        path: { record_type: recordType, id },
        ...ifMatch(requireVersion(version)),
      },
    });
    if (error) {
      throwProblem(error);
    }
    await queryClient.invalidateQueries({ queryKey: [`${recordType}s`] });
    await queryClient.invalidateQueries({ queryKey: [`${recordType}360`, id] });
    await queryClient.invalidateQueries({ queryKey: [recordType, id] });
  };
}
