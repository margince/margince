-- Nothing to undo. A validated constraint cannot be returned to NOT VALID, and
-- there is no state here worth inventing a way back to: rolling this back means
-- dropping the constraint, which is what the previous migration's own down does.
SELECT 1;
