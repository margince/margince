-- The grant goes with the object it names: leaving it behind would have every
-- role document claim authority over a table that no longer exists.
UPDATE role SET permissions = permissions #- '{objects,knowledge_corpus}'
    WHERE permissions ? 'objects' AND (permissions -> 'objects') ? 'knowledge_corpus';

UPDATE role SET permissions = permissions #- '{objects,knowledge_document}'
    WHERE permissions ? 'objects' AND (permissions -> 'objects') ? 'knowledge_document';
