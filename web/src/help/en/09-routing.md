# Routing

The Routing page draws your config's rules and proxy groups as a map: a request comes in on the left, passes a rule, is handed to a proxy group, and leaves through an exit. Open it with “Routing” on the left of the config editor.

If these ideas are new, click the **?** next to the title for a five-step walkthrough of one request.

## Reading the map

- **Rules** are checked from the top; the first rule that recognizes the request wins. Adjacent rule sets with the same target fold into one row; click to expand. Same-named domain, ipcidr and classical files, as published by domainrouter_v2, form a **bundle** and take one row.
- **Proxy groups** are laid out by their default path: until you choose by hand, a group uses its first member, so lines always run left to right. Fallback members are hidden unless you hover or select the group, or turn on “Show all candidates”.
- **Exits** are nodes, Direct or Block. With many nodes they are grouped by region or source.
- The dots are illustrative, not your real traffic: each one comes from a real entry in a rule set and follows the server's match result for it. Purple is proxied, teal goes direct, orange is blocked, grey is not yet matched.

## Tracing an address

Enter a domain or IP under “Trace a request” to see which rules did not match, which one did (and which entry in the rule set), how each proxy group picks, and where the request leaves. A dot on the map checks the rules one by one.

Some rules are marked “cannot be decided” because the server does not have the information:

- Rules about the app (process name): fill in the app under “App and UDP”.
- Rules about ports, source addresses and other connection details.
- Rules that need the IP: when no domain rule matches, the client resolves the domain and checks IP rules. This site does not look up DNS for you; trace the IP directly instead.

**Check common sites** (top right) traces thirty-odd common websites at once.

## Changing routing

- **Change a target**: drag a rule onto another proxy group, or onto Direct or Block.
- **Reorder**: drag onto the top or bottom edge of another rule. The final rule must stay last.
- **Bundle**: drag onto the middle of another rule set. A member can be split out in the details.
- **Add a member**: drag a proxy group or node onto another proxy group. Anything that would make groups refer to each other in a loop is refused; mihomo does not accept it.
- Click a row to open its **details**: you can change targets and order without dragging; for proxy groups you can change the type (manual, fastest, failover, load balance) and parameters. The details say what each type is good and not good for, with notes on the parameters.

Changes go into a **draft** listed below the map. They are written to the config only when you click “Apply”, after which autosave, checks and undo work as usual.

The first time you apply, the config starts **managing its own routing**: the rules are copied into the config, and later rule source updates no longer take effect automatically.

## When the rule source changes

Once a config manages its own routing, the Routing page shows “The rule source has N updates” when the source changes. Click “Review” to go through them: checked ones go into the draft, unchecked ones are ignored. Places you changed yourself are marked, and yours is kept by default.

## Expectations

After tracing an address, click “Pin as expectation”, for example “chatgpt.com always goes through the ChatGPT group”. When later edits or rule source updates break it, the “Expectations” list on the Routing page marks it (✗) and shows where the request actually goes.
