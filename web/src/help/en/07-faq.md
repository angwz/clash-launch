# FAQ

## The client did not change after updating

The editor saves automatically. Update the subscription manually in the client. Some clients cache configs and need the config re-selected.

## The check results show errors

Open **Check results** in the preview and click an entry to jump to the field. With an error, this render fails and the config link keeps serving the last successful version; warnings and notes do not block rendering but are worth a look — anything skipped, rewritten or removed is listed rather than silently dropped.

## What is the kernel check

The server runs the real mihomo core with `mihomo -t` to make sure the config loads. Passing means format and semantics are fine, not that every node is reachable.

## A node shows the wrong flag

Set its region manually in the node details, or adjust the flag options under the source's tidying.

## Can I change fields the form lacks

Yes, edit YAML directly. It shows the full config exactly as in the preview; change it in place. Only your changes are recorded, and they keep applying after subscriptions and rules update.

## History

Every save records a version. The config's History page compares any two versions and can roll back.
