# Proxy chains

A proxy chain sends some nodes **through another node (the front proxy)** first:

this device → your VPS → provider node → website

Typical uses: your network cannot reach the provider directly, or you want websites to see a fixed exit.

## Setting one up

At the bottom of **Nodes & routing**, add a rule under **Proxy chains**:

1. **Front proxy**: a node or proxy group.
2. **Apply to**: pick nodes by subscription, region or name regex; leave empty for all nodes.
3. Rules apply in order; later ones override earlier ones.

Each rule shows the nodes it actually applies to. The config uses mihomo's `dialer-proxy`:

```yaml
proxies:
  - name: HK 01
    type: ss
    dialer-proxy: My VPS
```

## Caveats

- **No loops**: A through B and B through A is a cycle; such a rule is not applied and is reported as an error.
- **Groups can loop**: a front group that contains the chained node only triggers a warning, since it depends on the group's current selection.
- **Subscriptions left to the client** can only take a front proxy as a whole, not by region or name.
