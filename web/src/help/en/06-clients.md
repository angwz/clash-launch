# Using it in a client

A config link is an ordinary HTTPS URL; any client that subscribes to mihomo (Clash Meta) configs can use it.

## Adding the subscription

1. Copy the link on the config's **Config links** page, or scan its QR code.
2. In your client, find Subscriptions, Configs or Profiles, import from URL and paste the link.
3. Turn on auto-update in the client so changes made here arrive automatically.

## OpenClash (OpenWrt)

Choose the OpenClash preset when creating the config; it is checked against OpenClash's bundled core. Add the link under OpenClash's config subscriptions. On routers it is usually worth setting:

- General → find-process-mode to `off` (recommended by the docs: routers cannot see other devices' processes)
- Keep the default geodata loader `memconservative` to save memory

## One config, many devices

Create a separate link for each device with a note ("Phone", "Router"). The Config links page shows when and by which client each link was last fetched; if a device is lost, reset just its link.
