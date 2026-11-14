# Core settings

**Core settings** in the editor lists mihomo options by section: general, inbound, controller, DNS, TUN, sniffer, geodata and more.

## Reading a field card

- **Title and key**: the name and the YAML key, e.g. Enhanced mode `enhanced-mode`.
- **Default on/off**: what the core uses when you leave it unset; unset fields are not written to the config.
- **Recommended / Suggested**: Recommended comes from the mihomo documentation; Suggested is common practice, not an official requirement. Click to apply.
- **Details & advice**: what it is, how to choose, how the options differ, the YAML and a link to the official docs.
- **Reset**: removes your value and returns to the core default.

Hover a field and the preview highlights where it lands in the YAML.

## Settings people change most

- **Inbound → Mixed port**: the port for browsers and the system proxy, commonly 7890.
- **DNS → Enable, Enhanced mode**: usually enable DNS and use fake-ip.
- **TUN → Enable**: route games, CLIs and other apps that ignore the system proxy.
- **Inbound → Allow LAN**: let phones and other devices use this machine; also set a password or source restriction.

## Settings the form lacks

The catalog follows mihomo v1.19.33. For fields it does not list yet, or to override whole blocks, edit YAML directly; it merges after the form, and fields it touches carry a ⚡ mark.

To study every field, browse the **Encyclopedia** by section.
