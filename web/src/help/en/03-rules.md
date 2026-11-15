# Routing rules

Routing rules decide where each connection goes: direct, reject, or a **proxy group** such as "Proxy", "Streaming" or "Auto".

## Presets

The Routing rules page lists common rule sets; the one marked Recommended is the default. Presets follow their upstream, so there is nothing to maintain.

## Custom rules

Click **Custom routing rules** to use the formats common in the subconverter ecosystem:

- `groups.toml`: proxy groups and which nodes each one uses
- `rulesets.toml`: rule sets and the groups they map to
- external config `ini`: the format used by ACL4SSR and similar projects

Each file can be a **URL** (follows upstream) or **pasted text**; pasted text wins when both exist.

## How groups pick nodes

`groups.toml` can pick nodes by name regex, region or subscription. Picking by subscription uses its number (`!!GROUPID=`): subscriptions are numbered from 0 in config order, disabled ones skipped. When a number is used, the editor shows each subscription's GROUPID.

## Adding your own rules

To put a few rules first (for example, direct access to an intranet), open **Edit YAML directly** and insert them above the first line under `rules:`:

```yaml
rules:
  - DOMAIN-SUFFIX,intranet.example.com,DIRECT
  - …(existing rules)
```

Only the line you added is recorded; it stays first after the rules update.
