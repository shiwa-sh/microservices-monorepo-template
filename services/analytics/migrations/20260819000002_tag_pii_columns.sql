-- migrate:up
-- Every column, classified (ADR-0301, ADR-0700). The analytics store is the one
-- place on this platform where saying "no personal data here" would be false for
-- the whole table, so each column states its class and the ones that carry nothing
-- say so explicitly.
comment on column events.session_id is 'pii:identifier';
comment on column events.identity_id is 'pii:identifier';
-- The event's own fields. A funnel step's properties are written by first-party
-- code and are not meant to carry personal data — but the column cannot enforce
-- that, and a field a caller fills is a field that may hold anything.
comment on column events.properties is 'pii:free_text';
-- A class, not a user agent. Tagged `device` because it is derived from one, which
-- is what an erasure request needs to know.
comment on column events.device_class is 'pii:device';
comment on column events.name is 'pii:none';

comment on column consent.session_id is 'pii:identifier';
comment on column consent.identity_id is 'pii:identifier';
comment on column consent.state is 'pii:none';
comment on column consent.purpose_version is 'pii:none';
comment on column consent.source is 'pii:none';

-- migrate:down
comment on column events.session_id is null;
comment on column events.identity_id is null;
comment on column events.properties is null;
comment on column events.device_class is null;
comment on column events.name is null;
comment on column consent.session_id is null;
comment on column consent.identity_id is null;
comment on column consent.state is null;
comment on column consent.purpose_version is null;
comment on column consent.source is null;
