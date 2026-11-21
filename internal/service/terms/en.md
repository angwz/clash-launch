# Terms of Use

This site is a Clash (mihomo) config generator: you provide nodes, subscriptions and routing rules, and it produces a well-formed config that your clients load through a long-lived subscription link. By registering and using this site, you confirm that you have read and agree to the following.

## How this site works

- **Official semantics only**: generated configs follow mihomo's official documentation and source code. Anything without a documented basis is not guessed; it is listed as a limitation and reported to you.
- **No third-party services**: no third-party translation, font, analytics or CAPTCHA services are used at runtime. The only connections the server makes on its own are to the subscriptions, rule sources and mail server you configure.
- **What you write is kept as written**: node names, group names, regular expressions and hand-written rules are preserved. If something looks unusual, you get a notice, not a silent rewrite.
- **Nothing is dropped silently**: anything skipped, rewritten, removed or unverifiable is listed in the diagnostics.
- **You can delete at any time**: your node sources, configs, subscription links and short links can be deleted whenever you like.

## What we store

- **Account details**: username, nickname, a hash of your password, hashes of your security answers, and interface preferences.
- **Nodes and configs**: subscription URLs, node content, config documents and their history, generated configs and in-site messages, all encrypted before they are written to the database.
- **Signed-in devices**: device type, the network range of the sign-in IP (not the full address) and an approximate location.
- **Subscription fetch records**: when a subscription link is requested, the client's full IP, region, network operator and client name, so you can see who is using it under Subscription Analytics. Details are kept for {{request_days}}; daily totals are kept indefinitely.
- **Account activity**: security-related actions such as sign-ins and password changes, kept for {{audit_days}}.

**Nodes and configs cannot be end-to-end encrypted.** The server must read them in plain text to refresh subscriptions, generate configs and answer your clients while you are offline, so the decryption key lives on the server. Administrators cannot see your nodes or configs through the site, but whoever controls the server and its key can technically decrypt them. See [Privacy and security](/help?topic=08-privacy) in the help for details.

## Your responsibilities

- Follow the laws and regulations that apply to you, and do not use this site for unlawful purposes.
- You are responsible for the nodes, subscriptions and rules you add.
- A subscription link is as sensitive as a node password: do not share it publicly, and do not share other people's links or credentials. Reset it right away if it leaks.

## About the service

- This site is deployed and maintained by its administrator. Continuous availability is not guaranteed, nor is it guaranteed that generated configs suit every situation.
- Accounts that violate these terms may be disabled.
- When these terms change, you will see the new version the next time you use the site and must agree before continuing.
