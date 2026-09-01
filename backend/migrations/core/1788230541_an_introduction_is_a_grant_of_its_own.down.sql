SET LOCAL lock_timeout = '5s';
UPDATE role SET permissions = permissions #- '{objects,introduction}'
    WHERE permissions ? 'objects' AND (permissions -> 'objects') ? 'introduction';
