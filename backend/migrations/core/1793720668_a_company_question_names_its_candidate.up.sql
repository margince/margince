-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- What an open company question can tell its reader beyond the reason code.
--
-- candidate_company_id is the existing company a near_duplicate question
-- found with the same name, so the reader can confirm it instead of searching.
-- site_unreadable separates a site that could not be read from one that was
-- read and named no company; both carry pending_reason 'unevidenced'.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company_domain_disposition
    ADD COLUMN candidate_company_id uuid
        CONSTRAINT company_domain_disposition_candidate_company_fkey
        REFERENCES company(id) ON DELETE SET NULL,
    ADD COLUMN site_unreadable boolean NOT NULL DEFAULT false;

-- Deleting a company clears the candidate, and this index finds those rows.
CREATE INDEX idx_company_domain_disposition_candidate
    ON company_domain_disposition (candidate_company_id)
    WHERE candidate_company_id IS NOT NULL;

-- Open questions written before this column carry only the sentence the
-- triage stored. These are the sentences for a site that was never read, in
-- every language the triage writes.
UPDATE company_domain_disposition
   SET site_unreadable = true
 WHERE pending_reason = 'unevidenced'
   AND evidence IN (
       'Every attempt to read this site failed, so the question was answered from what the workspace already knew.',
       'Alle Versuche, diese Website zu lesen, sind fehlgeschlagen. Die Frage wurde anhand der bereits bekannten Informationen beantwortet.',
       'Mọi lần thử đọc trang web đều thất bại. Câu hỏi được giải quyết dựa trên thông tin đã biết.',
       'No site could be read for this domain, so the company question was answered from what the workspace already knew.',
       'Für diese Domain konnte keine Website gelesen werden. Die Unternehmensfrage wurde anhand der bereits bekannten Informationen beantwortet.',
       'Không đọc được trang web nào cho tên miền này. Câu hỏi về công ty được giải quyết dựa trên thông tin đã biết.',
       'Automatic enrichment is off, so the company question was answered without reading the site.',
       'Die automatische Anreicherung ist deaktiviert. Die Unternehmensfrage wurde ohne Lesen der Website beantwortet.',
       'Tự động bổ sung thông tin đang tắt. Câu hỏi về công ty được giải quyết mà không đọc trang web.'
   );
