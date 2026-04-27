# VX6 Chat

VX6 Chat is a web chat app that runs on top of the VX6 network.

It gives you:

- direct peer-to-peer chat by VX6 username
- decentralized group chat
- a local web UI
- no central chat server

Each user runs:

1. `vx6 node`
2. `vx6-chat`
3. a browser pointed at the local web UI

## How It Works

- the app exposes a local VX6 service named `chat`
- other VX6 nodes reach `username.chat`
- direct messages go from one user straight to the other user
- group messages are fanned out directly to each group member
- every browser only talks to its own local chat app

This means:

- discovery is decentralized through VX6
- transport is peer-to-peer over VX6 service dialing
- group chat has no central coordinator

## Build

```bash
go build -o ./vx6-chat ./apps/chat
```

## Run

Make sure your normal VX6 node is already initialized and running.

```bash
./vx6 node
./vx6-chat
```

Then open:

```text
http://127.0.0.1:8088
```

By default the chat app uses:

- web UI: `127.0.0.1:8088`
- local chat transport: `127.0.0.1:8787`

You can change them:

```bash
./vx6-chat --http 127.0.0.1:8090 --transport 127.0.0.1:8790
```

## What The App Does Automatically

On startup it:

- adds the local VX6 service `chat`
- republishes the service record to the VX6 discovery network
- stores chat history in your local VX6 config directory

## Direct Message

If you want to message `bob`:

- start a direct chat with `bob`
- the app resolves `bob.chat`
- your message is sent over VX6 directly to Bob's chat app

## Group Chat

Create a group with VX6 usernames, for example:

- `alice`
- `bob`
- `carol`

When Alice sends a group message:

- the app sends that message directly to Bob
- the app sends that message directly to Carol

There is no group server in the middle.

## What You Need On Each Machine

Each machine should have:

- `vx6` initialized
- `vx6 node` running
- `vx6-chat` running

## Storage

The chat app stores its local state next to the VX6 config, under:

```text
~/.config/vx6/chat/state.json
```

If `VX6_CONFIG_PATH` is set, the chat state is stored next to that config path.

## Test

```bash
go test ./apps/chat -v
```
