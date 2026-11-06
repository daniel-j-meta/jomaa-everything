# Home hub design

## Why

device-panel controls Govee lights from Cloudflare. The goal is one hub at home
that controls Matter devices and Govee lights through a single interface, keeping
device-panel's shape: rooms that pass commands to every light in them, and a sync
pass that fixes lights that didn't change.

## Decisions

- **Go for the hub, Python for startup and control.** The user picked these.
- **The hub hands Matter to the Open Home Foundation Matter server.** Go has no
  mature Matter library. The Matter server is certified, used by Home Assistant,
  ships as a Docker image, and has a stable WebSocket API. The hub keeps a live
  copy of each node's attributes from that connection, so reading state costs
  nothing.
- **It runs at home, not in the cloud.** Matter only works on the local network.
  Both services use host networking. The Matter server listens on localhost only.
- **One `Device` interface. Rooms implement it too.** A room sends each command
  to all members at once. A failing bulb doesn't block the others, and failures
  are reported per device. Rooms can contain rooms.
- **Sync checks every bulb.** device-panel compared one representative bulb.
  The hub checks each bulb and corrects only the fields that drifted. Levels are
  never corrected when the room should be off, so a sync can't relight a lamp.
- **The client owns the desired state**, as in device-panel. The hub only tracks
  the newest revision per client. Any newer command from that client makes an
  older sync stand down.
- **Devices report what they can do.** A Matter plug has no brightness, so
  brightness commands skip it and sync doesn't try to fix it.
- **The layout lives in `config.json`.** Pairing a device doesn't add it to a
  room. You add it to the config, so names and rooms stay under your control.
- **Login is one shared password**, as in device-panel. The password is exchanged
  for a token derived from it. The cookie and the phone apps store the token, not
  the password.

## Not in this version

- Making Govee lights appear in Apple Home or Google Home (a Matter bridge).
- Phone apps (`jomaa-home-client`), which are on hold.
- Pushing live updates to clients. Clients poll state.
- Govee's local network API. The hub uses Govee's cloud, as device-panel did.
