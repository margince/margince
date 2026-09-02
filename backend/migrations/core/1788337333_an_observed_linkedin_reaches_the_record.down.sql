-- No rows are removed. The up promoted evidence-backed handles into the slot
-- the product reads, and once there they are indistinguishable from handles a
-- human typed or accepted — deleting by matching the evidence row again would
-- also delete a handle somebody entered independently. Reversing the schema is
-- not needed (the up changed none), and reversing the data would destroy
-- statements this migration did not make.
SELECT 1;
