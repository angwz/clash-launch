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
