# Privacy and security

Your nodes are private. This site is designed never to read or display them except to generate your configs.

## Encrypted at rest

The following are encrypted with AES-256-GCM before they are written to the database:

- Subscription URLs
- Node content
- Request headers you set for subscriptions
- Fetched subscription content and traffic information
- Config documents and every revision
- Generated configs and check results
- Config link tokens
- Messages and templates

Each ciphertext is bound to its table, column and row, so it can't be decrypted if moved elsewhere.

If the database file or a backup is taken, nothing can be read from it without the key. An automated test enforces this: it writes marked data, then scans every column of every table to confirm no plaintext remains.

## What administrators can see

On this site, administrators see only account metadata and usage: usernames, sign-up dates, how many sources and configs you have, and how much space they use. They cannot see your subscription URLs, nodes, configs or messages. Every admin endpoint is tested to make sure it never returns them.

## One thing you should know

To update subscriptions, generate configs and answer your clients while you're offline, the server has to be able to decrypt your data, so the key lives on the server.

That means **whoever controls this server and its key could technically decrypt your data outside the website**. This is true of every service that builds configs on a server, and we would rather tell you plainly.

## Check it yourself

- The code is fully open source: [github.com/angwz/clash-launch](https://github.com/angwz/clash-launch). Encryption lives in `internal/store` and the admin endpoints in `internal/api/admin.go`, so you can review them yourself.
- If you'd rather not trust anyone, deploy your own copy from the same code; then the key is only in your hands.
- A config link is as sensitive as a node password: don't share it publicly. If it leaks, reset it on the Config links page; the old link stops working immediately.

## Terms of use

The [terms of use](/terms) you agree to when registering list what this site stores, how long each kind of data is kept, and the rules of use. When the terms change, you are asked to read and agree to the new version the next time you use the site.

## Access tokens

On the Account page you can create access tokens so scripts can call the API, for example to refresh node sources on a schedule or read subscription stats:

```sh
curl -H "Authorization: Bearer clt_…" https://your-site/api/v1/profiles
```

- Tokens are read-only or read-write, and can only reach node sources, rule sources, configs, subscription links, short links and stats. Password, two-step verification, signed-in devices, messages and admin features are off limits.
- A token is shown only once, when it is created; the server keeps only its hash. If one leaks, revoke it on the Account page and it stops working immediately.
- All tokens are revoked automatically when your password is recovered or reset by an administrator.

## Two-step verification requirement

Administrators can require two-step verification for admins or for everyone. If it is required and you haven't enabled it, you'll see the setup page after signing in and can continue once it's done; while it is required, you can't turn it off.

## Notifications

The bell at the top right lists things that need your attention; if you have set up alert emails or alert channels, they get it too:

- **A node source keeps failing**: you are told once after two failed refreshes in a row. Subscriptions keep using the last successful nodes.
- **A config fails its check**: after a node source or rule source update, the config was regenerated in the background and did not pass. Subscriptions keep serving the last version that passed.
- **A subscription link has a new source**: the link was fetched for the first time from a new country or region, or by a new client. If that was not you or someone you shared it with, the link may have leaked, and you can replace the link token right from the notification. A link's first source (your own import) does not trigger this, and requests through short links are not counted, since short links are usually shared with others.


## Alert channels

Besides email, the Account page can send the same alerts to a webhook, Telegram, Bark or ntfy — up to 5 channels:

- Each channel is enabled only after a test message gets through; test again after changing its settings.
- URLs must use https, and addresses on private networks are refused (a self-hosted service on a private network needs the administrator to allow it).
- Tokens, device keys and signing secrets are stored encrypted and never shown again.
- Alert contents pass through the service you choose. On the public ntfy server anyone can subscribe to a topic, so pick a hard-to-guess name.

## Subscription link access limits

On the Subscription links page, choose Access limits to allow only certain clients or certain countries and regions to fetch a link, or to cap fetches per day:

- Clients or regions that don't match are blocked (403); requests whose region the server cannot determine are blocked too.
- Once the daily cap is used up, the link answers 429; clients keep their current config and it resets the next day, in your time zone.
- Blocked requests still show up in Subscription Analytics, with the status code telling you why.

## Backup and restore

The Account page downloads an encrypted backup of all your information, which can be restored on any site running this software:

- The backup is encrypted with a passphrase you choose. The server doesn't keep it; if you forget it, the backup can't be restored.
- Restoring replaces your current nodes, configs, links, templates, rule sources and alert settings with the backup. Your current state is saved first and can be brought back within 7 days.
- Subscription and short links come back with their tokens, so clients keep working. If a token or short code is already used by someone else, it is replaced and you are told which ones to update.
- Your username, password, two-step verification, passkeys, signed-in devices and access tokens are not overwritten; messages, friends and grants are not part of the backup.

## Delete your account

At the bottom of the Account page you can delete your account with your current password (and a verification code if two-step verification is on):

- There is a 7-day waiting period and all devices are signed out right away. Subscriptions keep working during that time, and signing in again cancels the deletion.
- When the period ends, your nodes, configs, subscription links, short links, templates and all settings are deleted and subscriptions stop working; grants you gave are revoked and the other person is notified.
- Conversations with others are kept and show you as a deleted user; your username stays reserved for 180 days so nobody can take it right away.
- Consider downloading a backup first.
