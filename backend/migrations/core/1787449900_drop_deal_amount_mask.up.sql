-- Deal amounts are open to every seat that may read the deal.
--
-- The baseline seeded one field mask: a rep read `amount_minor` as withheld on
-- any deal outside their write authority. It was written when a rep's write
-- authority covered their whole team, so it hid other teams' numbers and left
-- the rep's own team visible.
--
-- Two things changed. The seeded rep became own-scoped, which would have
-- narrowed the same mask from "another team's deals" to "every deal I do not
-- personally own" — a much larger blackout, arrived at as a side effect of a
-- decision about WRITES rather than as a decision about what reps may see. And
-- the product ruling is that deal values are open: a rep who cannot see what a
-- colleague's deal is worth cannot judge their own pipeline against it.
--
-- So the mask goes rather than being re-scoped. The masking MACHINERY stays —
-- field_mask, the two conditions, the sort/filter refusal, the masked_fields
-- wire field — because an operator may still author a mask on a custom role.
-- What is removed is the one row the product shipped.
DELETE FROM field_mask
 WHERE role_key = 'rep'
   AND object = 'deal'
   AND field = 'amount_minor'
   AND condition = 'outside_write_authority';
