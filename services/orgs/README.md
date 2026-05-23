# orgs

B2B multi-tenancy service (ADR-0010). Owns organisations + memberships.
Receives a webhook from Kratos at `POST /internal/identity-created` to create a
personal org for every new identity. Maps cleanly onto SpiceDB's `org` definition.
