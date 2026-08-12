-- migrate:up
-- org_members.user_id is the Kratos identity of a person (ADR-0304). It is
-- pseudonymous rather than directly identifying, and it still resolves to a natural
-- person through the identity store, so erasure and DSAR must reach it — which is
-- exactly what the tag makes queryable from pg_description (ADR-0301).
--
-- Tagged in a later migration than the one that creates the column because a dbmate
-- migration is immutable once applied; the tag travels with the schema either way.
comment on column org_members.user_id is 'pii:identifier';

-- The remaining columns carry no personal data, and saying so explicitly is what
-- distinguishes a considered decision from nobody having looked.
comment on column orgs.name is 'pii:none';

-- migrate:down
comment on column org_members.user_id is null;
comment on column orgs.name is null;
