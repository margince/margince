SET LOCAL lock_timeout = '5s';

-- Who may answer an assurance finding.
--
-- `assurance.Store.Resolve` gates on `forecast.update`, which no seeded role
-- held. The answering half of the input-check review was unreachable on every
-- installation and nothing said so: the door refused, which is what a door
-- does, and that refusal is indistinguishable from a correct one.
--
-- The four seats that already CREATE a reading gain update, and so does rep: a
-- finding is about the quality of an input, and whoever knows why a deal's
-- numbers look the way they do is the seat that entered them. read_only is
-- deliberately absent — a seat that may not change the forecast may not answer
-- a finding about it. Delete stays false everywhere.
--
-- VALUE-guarded rather than presence-guarded: the object key already exists, so
-- absence cannot be the guard. Matching the posture the old seed wrote means an
-- operator who has hand-edited forecast keeps their edit.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,forecast,update}', 'true'::jsonb, false)
    WHERE is_system
      AND key IN ('admin', 'management', 'manager', 'ops')
      AND permissions -> 'objects' -> 'forecast'
          = '{"create":true,"read":true,"update":false,"delete":false}'::jsonb;

-- A rep reads the forecast rather than recording one, so their old posture is
-- read-only.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,forecast,update}', 'true'::jsonb, false)
    WHERE is_system
      AND key = 'rep'
      AND permissions -> 'objects' -> 'forecast'
          = '{"create":false,"read":true,"update":false,"delete":false}'::jsonb;
