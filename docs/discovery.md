# Discovery

VX6 discovery has two parts.

## Local Registry

The registry is the local working view of the network.

It stores:

- known node records
- known service records

Peers exchange signed snapshots and updates.

## DHT

The DHT is the distributed lookup layer.

It is used when:

- the record is not already in your local registry
- you want a public service by exact name
- you want a private service catalog for one user
- you want a hidden descriptor by invite

## Important Rule

VX6 is not meant to store every user and every service on every node.

Instead:

- routing tables stay small
- records are replicated to a bounded set
- lookups walk toward the key

That is what makes the design scale better than a full global registry.
