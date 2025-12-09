A work-in-progress CI server that aims to keep things simple and keep files cached locally.
WIP documentation is dumped here below.

Depends on binaries: bwrap, cp

`/etc/sysctl.conf`

```
kernel.unprivileged_userns_clone = 1
kernel.apparmor_restrict_unprivileged_userns = 0
```

Systemd service -> see ci.service

Using [Fontawesome](https://fontawesome.com/) icons
