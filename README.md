# mediamtx-dynamic

MediaMTX for video (LL-HLS / WHEP / RTSP) + thin Go API for start/stop/meta/AI overlay.

Swagger UI: `http://127.0.0.1:8090/api/doc`

Not a full rewrite of `mobileservergo`: video pipeline is MediaMTX; AI metadata is sniffed from the same multicast and served as `GET /stream/overlay/{id}`.

## Requirements

- Go **1.22+** (prefer latest from https://go.dev/dl — Ubuntu `apt` Go is often too old for this module)
- MediaMTX [standalone binary](https://mediamtx.org/docs/kickoff/install#standalone-binary)

## Ubuntu quick start

```bash
git clone https://github.com/abnos5525/mediamtx-dynamic.git
cd mediamtx-dynamic

VER=v1.21.1
curl -L -o mtx.tar.gz \
  "https://github.com/bluenviron/mediamtx/releases/download/${VER}/mediamtx_${VER}_linux_amd64.tar.gz"
tar -xzf mtx.tar.gz

# terminal 1 — if :8000 is busy, mediamtx.yml already uses rtp 18000/18001
./mediamtx mediamtx.yml

# terminal 2
go run ./cmd/server
```

### Start / play / overlay / stop

```bash
curl -s -X POST http://127.0.0.1:8090/stream/start \
  -H 'Content-Type: application/json' \
  -d '{"url":"udp://224.2.0.1:5493/restream/bbcdcadb-e98e-4650-8eb2-10261f7c9ad0_720","output_type":6}'

# HLS from response.url — overlay (same path id):
curl -s 'http://127.0.0.1:8090/stream/overlay/<id>?lag_ms=600'

curl -s -X POST http://127.0.0.1:8090/stream/stop \
  -H 'Content-Type: application/json' \
  -d '{"url":"udp://224.2.0.1:5493/restream/bbcdcadb-e98e-4650-8eb2-10261f7c9ad0_720"}'
```

`output_type` `7` returns MediaMTX WHEP URL (`…/whep`), not a custom pion offer.

Optional body field `format`: `rtp` (default) or `mpegts`.

Env: `LISTEN`, `MTX_API`, `HLS_BASE`, `WEBRTC_BASE`, `HOST_IP` (NIC for AI sniff), `AI_OFFSET_MS`.

## Windows

```powershell
.\scripts\download-mediamtx.ps1
.\mediamtx.exe mediamtx.yml
go run ./cmd/server
```
