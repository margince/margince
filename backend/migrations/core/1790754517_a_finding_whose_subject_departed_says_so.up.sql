-- A finding on a deal that left the eligible set — won, lost, archived — is a
-- third fact, and the two words already here both say something false about it.
--
-- `resolved` claims somebody answered it. `condition_cleared` claims the
-- condition stopped being true, which is what CloseCleared refuses to say
-- about a departed deal: absence is only a fact about what was WALKED, and a
-- deal that left the set was never walked. What went away is the subject, not
-- the condition — the finding was never fixed, the record simply stopped being
-- checkable.
--
-- Until it has its own word those findings stay open forever, so the queue
-- carries them with nothing a reader can do about them and its count is wrong
-- by however many deals closed this quarter.
SET LOCAL lock_timeout = '3s';
ALTER TABLE assurance_exception
    DROP CONSTRAINT assurance_exception_status_check;
ALTER TABLE assurance_exception
    ADD CONSTRAINT assurance_exception_status_check CHECK (
        status IN ('open', 'resolved', 'expired', 'condition_cleared', 'subject_departed'));
