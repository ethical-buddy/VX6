# Services

VX6 supports three service types.

## Public Services

Public services are looked up by exact service name:

- `alice.web`
- `team.api`

They are discoverable through the DHT and local registry.

## Private Services

Private services do not publish themselves in the public `service/...` namespace.

They are exposed through a per-user private catalog.

That means:

- they do not appear in normal public lookups
- they appear only when checking one user’s private catalog

## Hidden Services

Hidden services use:

- a hidden alias
- a secret lookup component
- blinded rotating DHT keys
- encrypted hidden descriptors
- relay-based access paths

So the user resolves a hidden invite, not a plain public service record.
