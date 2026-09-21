# mediamtx-dynamic

Thin Go API + [MediaMTX](https://mediamtx.org/docs/kickoff/install#standalone-binary) so any multicast URL like:

```text
udp://224.2.0.1:5493/restream/bbcdcadb-e98e-4650-8eb2-10261f7c9ad0_720
```

is joined dynamically (host:port only; `/restream/...` is just an id) and served as LL-HLS / WebRTC.

**Not a drop-in for `mobileservergo`.** No AI overlay canvas. MediaMTX alone cannot map unlimited UDP sources without its [Control API](https://mediamtx.org/docs/features/control-api) — this repo is that glue.

## Requirements

1. MediaMTX standalone binary ([install](https://mediamtx.org/docs/kickoff/install#standalone-binary))
2. Go 1.22+

## Run

```powershell
# 1) download MediaMTX once
.\scripts\download-mediamtx.ps1

# 2) start MediaMTX (keep this window open)
.\mediamtx.exe mediamtx.yml

# 3) start API
go run ./cmd/server
```

## API

```powershell
# start (HLS)
curl -X POST http://127.0.0.1:8090/stream/start -H "Content-Type: application/json" -d "{\"url\":\"udp://224.2.0.1:5493/restream/bbcdcadb-e98e-4650-8eb2-10261f7c9ad0_720\",\"output_type\":6}"

# start (WebRTC WHEP)
curl -X POST http://127.0.0.1:8090/stream/start -H "Content-Type: application/json" -d "{\"url\":\"udp://224.2.0.1:5493/restream/bbcdcadb-e98e-4650-8eb2-10261f7c9ad0_720\",\"output_type\":7}"

# stop
curl -X POST http://127.0.0.1:8090/stream/stop -H "Content-Type: application/json" -d "{\"url\":\"udp://224.2.0.1:5493/restream/bbcdcadb-e98e-4650-8eb2-10261f7c9ad0_720\"}"
```

Response includes `url` (HLS or WHEP), plus `hls` / `webrtc` always.

Env overrides: `LISTEN`, `MTX_API`, `HLS_BASE`, `WEBRTC_BASE`.

## Notes

- Ingest assumes **RTP H264 PT 96** (`udp+rtp://`), same family as the VMS multicast cameras. If a camera is MPEG-TS over UDP instead, change `source` to `udp+mpegts://host:port` in `cmd/server/main.go`.
- Windows multicast needs a working IGMP path on the NIC that sees the group.
- For remote browsers, set `webrtcAdditionalHosts` in `mediamtx.yml` to this machine's LAN IP.
