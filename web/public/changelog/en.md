# Changelog

## 1.1.1 · 2026-11-26

- Fixed: the detail page of a routing rule added from a preset (URLs only) stayed on “Loading”.

## 1.1.0 · 2026-11-23

### Account and security

- Terms of use: read and accepted at sign-up; after an update, accept again to continue.
- Two-step verification policy: administrators can require it for administrators or for everyone.
- Personal access tokens: scripts can read or edit your own configs without being able to take over the account.
- Personal backup and restore: download everything, encrypted with a passphrase; restoring keeps the previous state for 7 days.
- Account deletion with a 7-day grace period; signing in cancels it.
- Avatars and default nicknames: new accounts get a random launch-themed avatar and nickname; change them or upload an image.
- End-to-end encrypted messages: the text is encrypted in the browser and the server stores only ciphertext.

### Configs and nodes

- Import a Clash or mihomo config copied from the web; export the same YAML as the subscription, optionally without nodes or rule sets for sharing.
- Delete protection, undo within 10 seconds of deleting, and batch actions on multiple items.
- Rule source editor: highlighting for toml and ini, inline problems, and a preview of the expanded proxy groups.
- Live preview of node content; restoring a default can be undone.
- A large editor for node content: field and value suggestions per protocol, a minimap, and a mihomo check of every proxy; it cannot be finished while problems remain.
- Proxy groups in rule sources can pick regions, node sources, protocols and tags as clickable conditions.
- Delete moved into the “⋯” menu so it is no longer one click away.

### Subscriptions and alerts

- Subscription link limits: allowed clients, countries or regions, and fetches per day.
- In-app alerts: repeated fetch failures, background checks that fail, and new countries or clients on a subscription.
- Alerts can also go to Webhook, Telegram, Bark and ntfy.
- Subscription analytics can be exported as CSV.

### Interface

- Live updates: changes appear immediately, and contacts show when they are online.
- The status light in the lower corner shows disconnection, unread messages, work in progress and new versions.
- The command palette (Ctrl K) finds your configs, node names, short links and contacts; press ? for all keyboard shortcuts.
- The browser's Back button returns to where you were; leaving with unsaved changes asks first.
- Footprints: firsts, milestones and new places, kept permanently.
- Choose an accent color in Settings; add the site to your home screen.
- Site announcements, a demo page that needs no account, and a Japanese interface (including field descriptions and diagnostics).
- Place names in analytics and sign-in devices follow the interface language; copy buttons also work over plain http.

### Operations

- PostgreSQL is now the default and ships in compose.yaml; existing SQLite data keeps working and one command migrates it (see the deployment guide).
- Core and field catalog follow mihomo v1.19.34: Snell v6, multi-user Shadowsocks inbounds and more.
- Minimal docker compose; the image drops privileges after starting and supports PUID and PGID; hardening is an overlay file.
- /metrics in Prometheus format; the admin page shows background jobs and data size, and retention can be adjusted on the web.
- Configurable PostgreSQL connection pool and statement timeout.
