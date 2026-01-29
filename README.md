Goals:

Builds as fast as local
Primary goal is speed, secondary goal is simplicity.
No YAML programming

designed to run on a single server with local disk storage only

main mechanism to be fast is to just use the build files cached locally on disk
all branches reuse the build files from the last completed build of the default branch by copying them

architecture overview
processor
builder
mermaid diagram

separate builder processes
updates

also document what build environment looks like, what is allowed and what isn't
sandboxing setup

A work-in-progress CI server that aims to keep things simple and keep files cached locally.
WIP documentation is dumped here below.

running the server

configuration documentation

required environment variables
database setup

Depends on binaries: bwrap, cp

`/etc/sysctl.conf`

```
kernel.unprivileged_userns_clone = 1
kernel.apparmor_restrict_unprivileged_userns = 0
```

Systemd service -> see ci.service

Using [Fontawesome](https://fontawesome.com/) icons
