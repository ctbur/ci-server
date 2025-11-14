A work-in-progress CI server that aims to keep things simple and keep files cached locally.
WIP documentation is dumped here below.

Depends on binaries: bwrap, cp

`/etc/sysctl.conf`

```
kernel.unprivileged_userns_clone = 1
kernel.apparmor_restrict_unprivileged_userns = 0
```

Systemd service

```ini
[Unit]
Description=Continuous Integration Server
After=network.target

[Service]
User=ci-server
Group=ci-server
Type=simple
# To stop, only kill the parent. This is required to keep the builder processes
# running, to allow for upgrades without stopping builds.
KillMode=process

WorkingDirectory=/
ExecStart=/usr/local/bin/ci-server --config /usr/local/etc/ci-server --lib /usr/local/share/ci-server
Environment=PATH='/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/games:/usr/local/games:/snap/bin'
EnvironmentFile=/usr/local/etc/ci-server/server-secrets.env

StandardOutput=journal
StandardError=inherit

Restart=always
RestartSec=5

ProtectHome=true
ProtectSystem=full
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

Using [Fontawesome](https://fontawesome.com/) icons in internal/web/ui/fontawesome.go
