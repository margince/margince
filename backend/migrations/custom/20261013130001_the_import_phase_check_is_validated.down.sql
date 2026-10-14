-- Put the check back as this migration found it: present and unvalidated.
SET LOCAL lock_timeout = '3s';

ALTER TABLE import_run
    DROP CONSTRAINT import_run_phase_check,
    ADD CONSTRAINT import_run_phase_check CHECK (phase IN (
        'reading', 'checking', 'creating_tags', 'importing', 'linking', 'undoing', 'finishing')) NOT VALID;
