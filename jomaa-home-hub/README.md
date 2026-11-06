# jomaa-home-hub

A Matter controller hub that also controls Govee lights. It follows the same
shape as [device-panel](https://github.com/jomaa-daniel/device-panel): a command
sent to a room goes to every device in that room, and the hub then checks that
each device actually changed and corrects any that drifted.

It has to run on the home network (a Pi, NAS or mini PC), not in the cloud,
because Matter devices are found and controlled locally. Thread devices also
need a Thread border router on the network.

Design notes live in `docs/specs/`.
