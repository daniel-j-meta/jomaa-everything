# jomaa-home-hub

One hub for every light in the house, Matter or Govee. Send a command to a room
and it goes to every device in that room at once. A sync pass then checks each
device and fixes any that didn't change. Same idea as
[device-panel](https://github.com/jomaa-daniel/device-panel), now running at home
and speaking Matter.

```
phone / web / hubctl.py
        │  HTTP (one API for every brand)
        ▼
   hub (Go) ── rooms, fan-out, sync
     │                    │
     │ WebSocket          │ HTTPS
     ▼                    ▼
 Matter server        Govee cloud
 (Docker)                 │
     │ Matter              ▼
     ▼                 Govee lights
 Matter devices
```

The hub doesn't speak Matter itself. It uses the
[Open Home Foundation Matter server](https://github.com/matter-js/matterjs-server)
(the one Home Assistant uses) for pairing and device sessions, and talks to it
over a local WebSocket.

## Run it

Needs a machine on your home network that stays on (a Pi, NAS or mini PC) with
Docker and Python 3. Thread devices also need a Thread border router on the
network (an Apple TV, HomePod mini or Nest hub).

```bash
cp config.example.json config.json   # list your devices and rooms
cp .env.example .env                 # set HUB_PASSWORD and GOVEE_API_KEY
python3 scripts/hubctl.py up         # starts both services, waits until healthy
python3 scripts/hubctl.py status
```

`hubctl.py dev` runs the Matter server in Docker and the hub from source, for
working on the hub.

## Reach it from anywhere

A Cloudflare Tunnel serves the hub at `https://hub.yourdomain.com` without
opening ports on your router. `cloudflared` runs next to the hub and connects
out to Cloudflare, and Cloudflare provides the HTTPS certificate. Only the hub
is exposed, never the Matter server.

1. Your domain must be on Cloudflare.
2. Create an API token with **Account > Cloudflare Tunnel > Edit**,
   **Zone > DNS > Edit** and **Zone > Zone > Read**.
3. On the hub machine, add these to `.env`, next to `GOVEE_API_KEY`, and use a
   `HUB_PASSWORD` of at least 16 characters:

```
CLOUDFLARE_HOSTNAME=hub.yourdomain.com
CLOUDFLARE_API_TOKEN=...
```

4. Run:

```bash
python3 scripts/hubctl.py up
```

That creates the tunnel, its route and the DNS record if they don't exist,
starts `cloudflared`, and checks that the public URL answers. In the phone
apps, sign in with `https://hub.yourdomain.com`; that address works at home
and away.

`scripts/cloudflare.py status` shows tunnel health. `scripts/cloudflare.py
destroy` removes the tunnel and the DNS record. The `hub-deploy` GitHub
workflow runs the tests and then `cloudflare.py deploy` on every push to
`main`, using the `CLOUDFLARE_HOSTNAME`, `CLOUDFLARE_API_TOKEN` and
`CLOUDFLARE_ACCOUNT_ID` repository secrets, the same way device-panel deploys
its Worker. It skips the deploy until `CLOUDFLARE_HOSTNAME` is set.

Sign-in is rate limited: 5 wrong passwords per address every 15 minutes, and
50 across all addresses.

## Add a Matter device

```bash
python3 scripts/hubctl.py wifi MyNetwork            # once, for Wi-Fi devices
python3 scripts/hubctl.py thread <hex dataset>      # once, for Thread devices
python3 scripts/hubctl.py commission MT:Y.K9042C00KA0648G00
```

It prints the node id. Add the device to `config.json` with
`"driver": "matter", "nodeId": <id>`, put it in a room, and restart.

Brand-new Wi-Fi and Thread devices pair over Bluetooth; turn that on in
`docker-compose.yml`. Without Bluetooth, use `--network-only` for devices already
on your network, for example one shared from Apple Home or Google Home.

## API

Every request except `/healthz` and `/api/login` needs
`Authorization: Bearer <token>` or the login cookie.

| Method | Path | Body | Does |
|---|---|---|---|
| POST | `/api/login` | `{"password"}` | returns `{"token"}` and sets a cookie |
| GET | `/api/targets` | | every device and room, with what each can do |
| GET | `/api/targets/{id}/state` | | room summary plus each device's state |
| POST | `/api/targets/{id}/command` | `{"on","brightness","colorTemperaturePct","color","clientId","revision"}` | 200 when every device took it, 502 with `failed` per device otherwise |
| POST | `/api/targets/{id}/sync` | `{"on","brightness","colorTemperaturePct","clientId","revision"}` | fixes devices that drifted, returns `synchronized` |
| GET | `/api/matter/nodes` | | paired Matter devices |
| POST | `/api/matter/commission` | `{"code","networkOnly"}` | pair a device |
| POST | `/api/matter/wifi` | `{"ssid","password"}` | Wi-Fi credentials for pairing |
| POST | `/api/matter/thread` | `{"dataset"}` | Thread dataset for pairing |
| DELETE | `/api/matter/nodes/{nodeId}` | | unpair |

All command fields are optional. `brightness` is 1-100. `colorTemperaturePct`
runs from the device's warmest white (0) to its coolest (100). `color` is
`"#rrggbb"`. A client sends a rising `revision` with each command, so a slow
sync can't undo a newer command.

## Test

```bash
python3 scripts/verify.py
```

Runs lint, builds and every test. The Python tests build the real hub and
drive it against a fake Govee API.
