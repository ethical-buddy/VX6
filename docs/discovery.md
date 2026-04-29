# VX6 Discovery

VX6 uses a practical discovery model:

- bootstrap nodes for first contact
- signed records for trust
- local registry cache for resilience
- DHT-backed lookup for node and service keys

## What Gets Published

VX6 publishes:

- node records
- direct service records
- hidden service descriptors

Each record is signed by the publishing node.

## Bootstrap Role

A bootstrap node gives a new node its first known VX6 peers.

After that, nodes:

- keep a local registry cache
- sync with known peers
- republish their own node and service records
- answer lookups from their cached state

That means a bootstrap is required for first contact, but not for every later lookup.

## DHT Role

VX6 stores and resolves:

- `node/name/<name>`
- `node/id/<node_id>`
- `service/<user.service>`
- `hidden/<alias>`

## Hidden Services

Hidden services do not publish a direct service endpoint.

They publish:

- alias
- intro points
- standby intro points
- hidden profile

## Practical Model

VX6 is not trying to be bootstrap-free from absolute zero.

The intended model is:

1. first contact through bootstrap or a known peer
2. node and service state spreads across known nodes
3. later updates continue through peers and DHT

## Operator Notes

- if you add bootstraps or services while the node is running, use `vx6 reload`
- if you know a remote IPv6 address already, you can skip discovery entirely and use `--addr`
