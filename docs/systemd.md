# systemd

Linux is still the primary runtime target in this repo.

If you want a long-running node on Linux, `systemd --user` is the intended service model.

Typical flow:

1. create the config with `vx6 init`
2. verify `vx6 node` works manually
3. wrap it in a user service
4. use `vx6 reload` for safe config refresh

The protocol itself is not Linux-only, but the operational packaging in this repo is still Linux-first.
