# Mail DNS

The `SPF`, `DKIM` and `DMARC` records platform mail authenticates with, one file per environment ([ADR-0307](../../docs/adr/0307-outbound-email.md)).

**Records, not a provider.** [ADR-0200](../../docs/adr/0200-cluster-topology.md) leaves provisioning to the project — `infra/terraform/` holds a README and no modules — so these files state the record content and leave the apply to whatever manages the zone. A Terraform module, a provider console, and a zone file all read the same lines.

**Why these are committed rather than set once by hand.** A mail sender's reputation lives in three records that are invisible when correct and silent when wrong: a message signed with a key whose `DKIM` record was never published fails at the receiver, is accepted by the sender, and reports nothing on this side. The records belong under review for the same reason the DKIM key belongs under SOPS.

## Applying them

1. Provision the dedicated egress IP and request its `PTR` record from the provider. It must resolve to the `hostname` in `infra/helm/platform/maddy/values.yaml` — a mismatch between HELO name and reverse DNS is a strong negative signal at every major receiver, and it fails at first send.
2. Generate the DKIM keypair. maddy writes both halves when `key_path` is absent, so generate once, put the private half in the environment's SopsSecret as `maddy-dkim`, and the public half in the `DKIM` record below. **Never let the pod mint its own**: it will sign with a key no record matches, every message will fail authentication, and nothing will be unhealthy.
3. Publish the records, then send one message and read the receiver's authentication results before trusting the path.
4. Leave `DMARC` at `p=none` until aggregate reports are clean, then move to `p=reject`. An environment is not production until it has ([ADR-0307](../../docs/adr/0307-outbound-email.md)).

## What is deliberately absent

- **No `MX` record for the mail subdomain.** The platform sends and does not receive; an `MX` would advertise an inbound path that no listener serves.
- **No MTA-STS policy.** That is published by a receiver, and this platform is not one. maddy *consumes* the policies its recipients publish, which needs no record here.
- **No records for human mailboxes.** They are a separate sender with their own egress IP and `DKIM` selector, and they serve the organisation domain's `MX`. Merging the two forfeits the reputation argument that put platform mail on a subdomain.
