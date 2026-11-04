# systemd units

Reference units for distributions without a randamu package (the .deb and
.rpm release assets already install these). They assume a dedicated user:

```bash
useradd --system --home-dir /var/lib/randamu --create-home randamu
```

- `randamu.service` — oneshot run, restarted on failure
- `randamu.timer` — daily schedule with a 1h random delay

The refresh token is not stored here: pass `PIXIV_REFRESH_TOKEN` through an
`EnvironmentFile=` (mode 0600) or let randamu read the rotated copy from the
state directory after the first manual run. See the main README under
"Token sources".

User-level alternatives: copy both files to `~/.config/systemd/user/`, drop
the `User=`/`Group=` lines, and run `systemctl --user enable --now
randamu.timer`. The unit paths and environment variables need adjusting.
