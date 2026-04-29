# VX6 with systemd

VX6 can run as a persistent background node.

The recommended mode is a user service:

- it runs as your user
- it uses your normal VX6 config
- it can be reloaded with `vx6 reload`

## Install

Build and install:

```bash
sudo make install
```

This installs:

- `vx6` to `/usr/bin/vx6`
- the unit file to `/usr/lib/systemd/user/vx6.service`

## User Service

Reload systemd and enable VX6:

```bash
systemctl --user daemon-reload
systemctl --user enable --now vx6
```

Check status:

```bash
systemctl --user status vx6
vx6 status
```

Follow logs:

```bash
journalctl --user -u vx6 -f
```

## Reload After Config Changes

If you add a service or edit `~/.config/vx6/config.json`:

```bash
vx6 reload
```

Or:

```bash
systemctl --user reload vx6
```

VX6 reload does not restart the process. It asks the running node to refresh config and republish immediately.

## Stop and Start

```bash
systemctl --user stop vx6
systemctl --user start vx6
systemctl --user restart vx6
```

## System Service

For a server, you can also run VX6 as a system service:

```bash
sudo cp /usr/lib/systemd/user/vx6.service /etc/systemd/system/vx6.service
sudoedit /etc/systemd/system/vx6.service
```

Set a real user and config path:

```ini
[Service]
User=alice
Environment=VX6_CONFIG_PATH=/home/alice/.config/vx6/config.json
```

Then enable it:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now vx6
```

## Runtime Files

Default paths:

```text
~/.config/vx6/config.json
~/.config/vx6/identity.json
~/.config/vx6/node.pid
~/.local/share/vx6
~/Downloads
```

## Notes

- do not run `vx6 node` manually while the systemd service is already active
- always quote IPv6 endpoints in shell commands
- if you change `listen_addr`, restart the service
- if you change services, bootstraps, advertise address, or hidden-service settings, `vx6 reload` is enough
