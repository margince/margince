-- 1795580606: an_edition_names_a_reader_that_exists.
SET LOCAL lock_timeout = '3s';

-- An edition's owner and publication team decide who may read it today, so
-- each names a row that exists. The writer copies both from the report
-- definition, whose own keys already hold them, and neither parent is ever
-- deleted. RESTRICT matches the definition's keys. The next file validates
-- them, so the scan runs under the lighter lock.
ALTER TABLE report_edition
    ADD CONSTRAINT report_edition_owner_id_fkey
        FOREIGN KEY (owner_id) REFERENCES app_user(id) ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT report_edition_publication_team_id_fkey
        FOREIGN KEY (publication_team_id) REFERENCES team(id) ON DELETE RESTRICT NOT VALID;
