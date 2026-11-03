-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- What an open company question can show its reader.
--
-- candidate_company_id is the existing company a near_duplicate question found
-- under the same name, and candidate_name the name the site stated. The read
-- suggests the company only while it is still the one live company of that name. site_unread tells an
-- unevidenced question whose site was never read apart from one whose site was
-- read and named no company.
SET LOCAL lock_timeout = '3s';

-- The foreign key is NOT VALID here and validated in a migration of its own,
-- and the index is built CONCURRENTLY in a third, so this one takes no scan of
-- its own under the ALTER's lock.
ALTER TABLE company_domain_disposition
    ADD COLUMN candidate_company_id uuid,
    ADD COLUMN candidate_name text,
    ADD COLUMN site_unread boolean NOT NULL DEFAULT false;

ALTER TABLE company_domain_disposition
    ADD CONSTRAINT company_domain_disposition_candidate_company_id_fkey
        FOREIGN KEY (candidate_company_id) REFERENCES company(id) ON DELETE SET NULL NOT VALID;

-- Open questions only, found through idx_domain_disposition_unevidenced. Their
-- evidence is the sentence the triage wrote when no read succeeded or none was
-- allowed, in each workspace language.
UPDATE company_domain_disposition
   SET site_unread = true
 WHERE status = 'pending' AND pending_reason = 'unevidenced'
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
