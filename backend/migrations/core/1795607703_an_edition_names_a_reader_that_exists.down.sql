SET LOCAL lock_timeout = '3s';

ALTER TABLE report_edition
    DROP CONSTRAINT IF EXISTS report_edition_publication_team_id_fkey,
    DROP CONSTRAINT IF EXISTS report_edition_owner_id_fkey;
