# Nodes: subscriptions and manual nodes

The Nodes page is your node library. Each entry is either a **subscription** or **manual nodes**.

## Subscriptions

Enter the URL from your provider; the server fetches it on a schedule. Formats are detected automatically:

- share links, one per line (`ss://`, `vmess://`, `vless://`, `trojan://` and more)
- base64-encoded subscriptions
- Clash / mihomo YAML
- sing-box JSON

If a fetch fails, the page shows since when, configs keep using the last successful fetch, and the check results mention it.

## Manual nodes

Paste one or more share links, YAML or JSON — handy for your own VPS or nodes you are testing.

## Tidying

Each source can tidy its node names: add or replace flags, keep or drop nodes by regex, rename and prefix, and remove info entries (remaining traffic, expiry) and duplicates. Regions are detected from flags and keywords; override them in the node details when detection is wrong.

## Using them in a config

Add sources under **Nodes & routing** in the editor. Top to bottom is the node order; drag to reorder. Each subscription can be written as:

- **Written into config**: nodes go into `proxies`, so tidying, renaming and proxy chains all apply.
- **Left to client**: written as `proxy-providers` and fetched by the client. Smaller configs, but tidying the client cannot express is skipped (listed in the check results).

Which to pick: to rename, add flags, group by region or chain, write into config; for very large or frequently updated subscriptions, or providers that only allow client fetching, leave it to the client. When unsure, write into config.

Newly added subscriptions use the default chosen by the administrator. Admins and invited users can change it per source; accounts from open registration always use the default, and the check results say so.
