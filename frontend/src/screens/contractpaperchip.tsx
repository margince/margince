// The signed paper on one agreement's row.
//
// Its own file because companycontracts.tsx sits at a frozen line ceiling
// and this is the part of the row with no commercial facts in it: it takes
// a contract and a company, reads the shared paper query, and draws chips.

import { FileChip } from "../design-system/filechip";
import { SurfaceState } from "../design-system/surfacestate";
import { formatBytes } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useContractPaper } from "./contractpaper";

/**
 * ContractPaper is the signed document itself, on the row for the agreement it
 * belongs to.
 *
 * The link is filed at upload as `attachment.contract_id`, so this asks the
 * documents endpoint for exactly that agreement's paper rather than guessing
 * from a matching title — a company with a 2024 and a 2026 framework agreement
 * has two files whose names differ by one digit, and matching on text would
 * hand a reader the wrong contract with full confidence.
 *
 * A contract with no paper renders NOTHING, not an error and not an empty
 * word. Recording what was agreed and filing the PDF are separate acts, and a
 * commercial record entered from an invoice is complete without a file.
 *
 * What it never does is present a PAGE as the paper. The documents endpoint
 * paginates, so a row that kept the first page and dropped `page.has_more`
 * showed some of the files under a label that reads as all of them — the same
 * silent truncation on the row as in the form, and the same fix: the chips the
 * read reached, and under them how many it did not.
 *
 * Each link is NAMED BY ITS FILE, not by a generic word for paper. This row is
 * the only place the file is read — the account's library below deliberately
 * leaves agreement paper to the agreement — so an amendment filed beside a
 * signed original has to be tellable from it, and two identical links are two
 * coin flips.
 */
export function ContractPaper({
  contractId,
  companyId,
}: Readonly<{ contractId: string; companyId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const query = useContractPaper(companyId, contractId);

  // A failed read says nothing here. The row's own commercial facts are
  // already on screen and are what the reader came for; an error chip next to
  // them would report a document problem as though the agreement were doubtful.
  const paper = query.data;
  if (!paper || paper.documents.length === 0) {
    return null;
  }
  // `remaining` is 0 only when the read reached the end of the list. Anything
  // else — a counted remainder, or more paper than the bounded count could
  // walk — is a row showing part of the paper, and it has to say so.
  const complete = paper.remaining === 0;
  return (
    // A DIV, not a span: the truncation sentence SurfaceState draws is a
    // paragraph, and a paragraph inside phrasing content is invalid markup.
    // Both containers set their own `display: flex`, so nothing moves.
    <div className="rec-files">
      <span className="rec-files-label">{t("contracts.files")}</span>
      {/* The cards wrap as their OWN group. Left in the label's row they wrap
          back to the panel's edge, so a second file starts to the left of the
          first and the label stops reading as a label for both. */}
      <div className="rec-files-items">
        <SurfaceState
          loadingLabel={t("contracts.files")}
          state={complete ? "ready" : "partial"}
          emptyLabel=""
          detail={{ remaining: paper.remaining }}
        >
          {paper.documents.map((file) => (
            // The filename, not the title: a paper's title is very often the
            // agreement's own title, and a link repeating the row it sits on
            // names nothing.
            <FileChip
              key={file.id}
              href={`/v1/attachments/${file.id}`}
              filename={file.filename}
              size={
                file.byte_size == null
                  ? undefined
                  : formatBytes(file.byte_size, locale)
              }
            />
          ))}
        </SurfaceState>
      </div>
    </div>
  );
}
