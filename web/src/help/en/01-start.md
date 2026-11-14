# Getting started

Clash, Launch! combines your **nodes**, **routing rules** and **core settings** into one mihomo (Clash Meta) config and serves it through a long-lived **config link**. Clients subscribe to the link; whatever you change here reaches them on their next update.

## Four concepts

- **Nodes**: the proxy servers you connect through — from a provider **subscription** or share links you paste manually.
- **Routing rules**: decide which sites go direct, which go through a proxy, and through which proxy group. Use a preset or import your own files.
- **Config**: which nodes, which routing rules and which core settings. One config produces one YAML file.
- **Config link**: the URL a client downloads the config from. A config can have several links, one per device, each reset independently.

## First run

1. On the Configs page, click **Wizard** and choose your client.
2. Enter a subscription URL, or paste node share links.
3. Pick a set of routing rules; if unsure, take the one marked Recommended.
4. Copy the config link and add it as a subscription (or "profile") in your client.

> A config link is as sensitive as your node passwords. If it leaks, click **Reset** on the Config links page and the old link stops working immediately.

## Making changes

Open a config in the editor:

- **Nodes & routing**: add or reorder node sources, switch routing rules, set up proxy chains.
- **Core settings**: DNS, TUN, inbound ports and more. Every field has **Details & advice**; when unsure, click the suggestion to apply it.
- The preview shows the generated YAML, and the top bar runs a kernel check (`mihomo -t`).
- After saving, clients pick up the change on their next update.
