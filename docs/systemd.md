# Running VX6 as a Systemd Service

VX6 is designed to run as a persistent background listener. This allows you to receive files and host services even when you aren't actively using the CLI.

## Installation

The `Makefile` handles the installation of the service file to the standard location:
```bash
sudo make install
```
This installs the binary to `/usr/bin/vx6` and the service unit to `/usr/lib/systemd/user/vx6.service`.

---

## User-Mode Service (Recommended)

Running in user-mode is best for personal machines. It ensures the node runs with your user permissions and stores its identity in your home directory.

### 1. Enable and Start
```bash
systemctl --user daemon-reload
systemctl --user enable vx6
systemctl --user start vx6
```

### 2. Verify Status
You can check the health of the background node in two ways:
```bash
# Via Systemd
systemctl --user status vx6

# Via VX6 CLI
vx6 status
```

### 3. Configuration & Logs
*   **Config Path**: `~/.config/vx6/config.json` (Managed via `VX6_CONFIG_PATH` in the service file)
*   **View Logs**: `journalctl --user -u vx6 -f`

---

## System-Mode Service (Dedicated Servers)

If you are running VX6 on a headless server and want it to start automatically on boot without a user login:

1.  Copy the service file to the system directory:
    ```bash
    sudo cp /usr/lib/systemd/user/vx6.service /etc/systemd/system/vx6.service
    ```
2.  Edit `/etc/systemd/system/vx6.service` to set a specific user:
    ```ini
    [Service]
    User=your-username
    Environment=VX6_CONFIG_PATH=/home/your-username/.config/vx6/config.json
    ```
3.  Enable and Start:
    ```bash
    sudo systemctl daemon-reload
    sudo systemctl enable vx6
    sudo systemctl start vx6
    ```

---

## Troubleshooting

### "Status: OFFLINE"
If `vx6 status` says the node is offline but `systemctl` says it is running:
1.  Check if the `listen_addr` in your `config.json` matches the address the node is actually trying to use.
2.  Ensure you are using the same `VX6_CONFIG_PATH` for both the service and your shell.

### Logs show "address already in use"
This happens if you try to run `vx6 node` manually in a terminal while the systemd service is already running. The background service owns the port (default `:4242`).
