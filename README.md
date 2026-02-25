# Flicker

A proof-of-concept watch-party stack built around a Go WebRTC SFU, a VM-side agent, and a React/Tailwind front-end. Use this doc to navigate the codebase, understand the data paths, and get the services running locally.

## Repository Map
- `cmd/sfu` — entrypoint for the SFU/signaling server (WebSocket + gRPC).
- `cmd/vmagent` — headless agent that streams media to the SFU and applies control input.
- `cmd/mockgrpcserver` — lightweight test server that emits dummy control events.
- `internal/sfu` — SFU domain logic:
  - `server` (WebSocket upgrading & session wiring)
  - `session` (WS protocol, WebRTC peer creation, RTP write pumps)
  - `room` (room roster + RTP fanout)
  - `ingest` (UDP listeners that accept RTP from the agent)
  - `control` (gRPC stream of keyboard/mouse/scroll events)
  - `vm` (stub for VM lifecycle)
- `internal/vmagent` — agent pieces:
  - `control` (gRPC client consuming control stream)
  - `injector` (currently prints control events; placeholder for actual OS input injection)
  - `avservice` (tags & forwards RTP; unused in current main)
- `api/control.proto` + generated `api/controlpb/` — gRPC API for control plane.
- `frontend/` — Vite + React + Tailwind UI for landing/rooms/video pages.
- `.github/workflows/deploy.yml` — self-hosted CI that builds backend, agent, and frontend.

## How the Backend SFU Works
1. **Startup** (`cmd/sfu/main.go`)
   - Loads `.env` for `PORT` (WebSocket), `TAILNET_IP` (NAT 1:1 IP), `LOG_LEVEL`, `SFU_GRPC_PORT` (control gRPC), `INGEST_PORT` (UDP ingest base; currently unused in code), and `SFU_GRPC_PORT`.
   - Spins up:
     - gRPC `InputService` on `SFU_GRPC_PORT` to forward control events to agents.
     - WebSocket server on `/ws` (loopback 127.0.0.1 by default) for browser clients.
     - Room/ingest managers and a shared control event channel.

2. **Signaling & Sessions** (`internal/sfu/server`, `internal/sfu/session`)
   - `WsConnectHandler` upgrades clients; constructs a `Session`.
   - `Session.ReadPump` consumes incoming WS messages:
     - `"offer"` → creates a Pion `PeerConnection`, adds static audio (Opus) and video (VP8) tracks, and replies with `"answer"`.
     - `"ice-candidate"` → added to the peer.
     - `"join-room"` / `"create-room"` → calls into `RoomManager`.
   - `Session.WritePump` sends signaling responses (answers, ICE) back to the client.
   - `StartWritingRTP` drains `AudioBuf` / `VideoBuf` channels and writes RTP into the local tracks; emits RTCP SR every 5s.

3. **Rooms & Media Fanout** (`internal/sfu/room`)
   - `RoomManager` holds up to 10 rooms (room IDs are currently hard-coded `"12345"`).
   - `Room.Broadcast` pushes incoming RTP packets into each active member’s audio/video buffers (with simple backpressure: drop oldest on full buffer).

4. **Media Ingest** (`internal/sfu/ingest`)
   - `IngestManager.AddRoom` spawns an `ingestWorker` that listens on UDP ports (currently fixed to 50005 audio / 50007 video regardless of passed values) and calls `Room.Broadcast` with parsed RTP packets.
   - RTCP ingest is stubbed.

5. **Control Plane** (`internal/sfu/control`)
   - gRPC `InputService.StreamInputs` multiplexes per-room streams (metadata header `room-id`).
   - `CommandEvent` structs (keyboard/mouse/scroll) pulled from `controlMessageEventBus` are serialized and pushed to the appropriate gRPC stream.

## VM Agent (cmd/vmagent)
Purpose: run beside the media source, forward RTP to the SFU, and apply control inputs.

Components:
- **Config** (`internal/vmagent/config.go`): expects `SFU_FORWARD_ADDR`, `LISTEN_PORT`, `SFU_GRPC_ADDR`, `ROOM_ID`, `AUDIO_PORT`, `VIDEO_PORT`.
- **Control service** (`internal/vmagent/control`): gRPC client to SFU `InputService`; streams events into a channel.
- **Injector service** (`internal/vmagent/injector`): currently logs events; replace `handleInput` with real OS input injection.
- **AV service** (`internal/vmagent/avservice`): reads RTP from a local UDP socket, tags with room ID as header extension, and forwards to SFU; not currently wired in `main`.
- **GStreamer helper** (`spawnGStreamer` in `cmd/vmagent/main.go`): demonstrates a test pipeline sending opus (pt=111) and VP8 (pt=96) to SFU; main currently never calls it.

Known gaps/TODOs:
- `main.go` has a placeholder `if err :=  (ctx, cfg, logger);` — intended to call `spawnGStreamer`; wire it up.
- `loadConfig` swaps audio/video env names (`AUDIO_PORT` → `videoPort`, `VIDEO_PORT` → `audioPort`); fix before relying on it.
- Ingest worker ignores per-room port allocation (uses hard-coded 50005/50007); align with negotiated ports.
- Room IDs are static; introduce real IDs and surface them to clients/agent.

## Frontend (frontend/)
- Stack: Vite, React, TypeScript, Tailwind, shadcn/ui.
- Routing (`src/App.tsx`): `/` landing, `/rooms` list, `/room/:roomId` playback, wildcard → 404.
- WebSocket handling (`src/context/websocketcontext.tsx`): builds `ws://<host>/ws?displayname&userid`, maintains connection state, exposes `connect`, `disconnect`, `send`.
- Pages:
  - `Landing` — collect display name and open WS connection.
  - `Rooms` — mock room list with create dialog (no backend yet).
  - `VideoRoom` — plays a sample MP4 with UI chrome; not yet connected to WebRTC stream.

Run locally:
```bash
cd frontend
npm install
npm run dev    # defaults to Vite dev server on 5173
```
Ensure the Go backend is reachable at the same host/port or configure a dev proxy so `/ws` points to `cmd/sfu`’s WebSocket endpoint.

## Local Backend Runbook
1. Copy `.env` and adjust:
   - `PORT` — WebSocket listen port (default 50000 in repo).
   - `TAILNET_IP` — public/NAT IP for ICE host candidates.
   - `SFU_GRPC_PORT` — gRPC control port.
   - `INGEST_PORT` — intended base UDP ingest port (currently unused).
   - For agent: `SFU_FORWARD_ADDR`, `SFU_GRPC_ADDR`, `ROOM_ID`, `AUDIO_PORT`, `VIDEO_PORT`, `LISTEN_PORT`.
2. Start SFU:
   ```bash
   go run ./cmd/sfu
   ```
3. Start VM agent (after fixing the TODOs above):
   ```bash
   go run ./cmd/vmagent
   ```
4. Optionally run `cmd/mockgrpcserver` to simulate control input during development.

## Message/Protocol Cheat Sheet
- **WebSocket messages** (`internal/sfu/session/message.go`):
  - `{"type":"offer","payload":{"sdp":string}}`
  - `{"type":"ice-candidate","payload":{"candidate":string,"sdpMid":string,"sdpMLineIndex":number}}`
  - `{"type":"join-room","payload":{"roomId":string}}`
  - `{"type":"create-room","payload":{"roomName":string}}`
- **gRPC control** (`api/control.proto`):
  - `InputService.StreamInputs(google.protobuf.Empty) returns (stream InputMessage)`
  - `InputMessage` oneof: `KeyboardInput {key, action}`, `MouseInput {x,y,action,dragging}`, `ScrollInput {direction}` plus `roomID`.

## Deployment Notes
- CI workflow builds:
  - `go build ./cmd/sfu` → `backend-binary`
  - `go build ./cmd/vmagent` → `vmagent/vmagent-binary`
  - `npm run build` in `frontend/` → static assets
- Deploy step copies artifacts to `/home/deploy/app` on a self-hosted runner and restarts `myapp` systemd service.

## Quick Orientation Tips
- Start with `cmd/sfu/main.go` to see startup wiring.
- Trace WebSocket handling: `server.WsConnectHandler` → `session.ReadPump`/`WritePump` → `session.StartWritingRTP`.
- RTP path: agent UDP → `ingest.ingestWorker.startRTPIngest` → `room.Room.Broadcast` → per-session track write pumps.
- Control path: UI (future) → `control.CommandEvent` → gRPC `InputService` → agent `control.ControlService` → `injector`.
- Frontend uses mock data; real-time hookup will require integrating WebRTC offer/answer exchange over `/ws` and feeding RTP from the SFU.
