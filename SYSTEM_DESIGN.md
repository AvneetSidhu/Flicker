# Flicker System Design

## Purpose

Flicker lets users view and, one at a time, control a desktop running inside a room-specific virtual machine. It is designed for a small, private deployment where the SFU host and VM network are trusted.

## System Overview

```text
Browser ── HTTPS/WSS ──> Nginx ──> SFU (Go)
  │                                  │
  │ WebRTC media                     │ gRPC control stream
  <────────────────────────────────  │
                                     v
                         Per-room libvirt VM + vmagent
                              │                 │
                         GStreamer RTP      xdotool → X11 desktop
```

- **Frontend:** React/Vite provides room creation, presence, WebRTC playback, and control UI.
- **Nginx:** terminates TLS, serves the static frontend, and proxies `/ws`, `/rooms`, and `/health` to a loopback-only SFU.
- **SFU:** manages rooms, signaling, WebRTC peer connections, RTP fanout, libvirt lifecycle, and control authorization.
- **VM agent:** captures the X11 desktop and PulseAudio with GStreamer, sends RTP to the SFU, receives control commands, and applies them with `xdotool`.

## Room Lifecycle

1. A browser creates a room over WebSocket.
2. The SFU creates a UUID, reserves distinct audio/video UDP listeners, registers ingest, and records the room as `provisioning`.
3. The VM service clones the shut-off base domain, writes `/etc/vmagent.env`, then starts the clone.
4. The VM agent starts GStreamer after the desktop session is ready. It captures X11/PulseAudio and sends Opus and VP8 RTP to the room’s reserved SFU ports.
5. The first valid RTP packet marks the room `ready`. The SFU distributes incoming packets to every joined WebRTC peer.
6. A viewer joins through WebSocket signaling, receives an SDP answer and ICE candidates, then plays the WebRTC stream.
7. A viewer can request exclusive control. Authorized keyboard, pointer, and wheel events go through the SFU’s command channel to the VM agent.
8. Deleting a room, or reaping an empty room, cancels pending VM provisioning, stops ingest, destroys the VM, undefines its domain, and removes its storage.

If provisioning fails, ingest is stopped before the room is exposed as `failed`, preventing a room from appearing failed while still consuming media resources.

## Stack Choices and Tradeoffs

| Area | Choice | Why this choice | Alternatives and tradeoffs |
| --- | --- | --- | --- |
| Signaling/SFU | Go + Pion WebRTC | Low-overhead concurrency, direct RTP handling, and a native WebRTC stack without a separate media server. | Janus, mediasoup, or LiveKit provide more mature operational features but add a new platform/runtime and less application-specific control. |
| Browser media | WebRTC | Low-latency playback, NAT traversal, adaptive browser support, and standard peer feedback. | HLS/DASH scale well for one-way broadcast but add seconds of latency and are unsuitable for interactive control. |
| VM media capture | GStreamer + `ximagesrc`/`pulsesrc` | Mature Linux pipeline, RTP support, and configurable codecs without application-level encoding code. | PipeWire is preferable for Wayland-first desktops; FFmpeg is simpler for fixed pipelines but less composable for RTP graphs. |
| Virtualization | libvirt/QEMU/KVM | Uses Linux host primitives, supports cloning/cleanup, and avoids cloud dependence for short-lived desktop workloads. | Containers are lighter but cannot reliably isolate a full interactive desktop; cloud VMs improve elasticity but increase provisioning latency and cost. |
| Agent control | Long-lived gRPC server-streaming connection | Typed protobuf contract, stream reconnection, backpressure-friendly delivery, and separation from browser signaling. | Sending commands on the signaling WebSocket couples untrusted browser traffic to the VM command plane and makes agent reconnect/authorization harder. HTTP polling has avoidable latency. |
| Private network | Tailnet-bound gRPC | Keeps the agent endpoint off public interfaces while retaining simple VM-to-host routing. | Public gRPC requires stronger certificate management and firewalling; a message broker adds durability but is unnecessary for transient input events. |
| Edge proxy | Nginx | TLS termination, static-file delivery, WebSocket proxying, and a loopback-only Go service. | Exposing Go directly reduces components but mixes TLS/static asset concerns into the SFU. Caddy is a viable simpler certificate-management alternative. |

## Efficiency and Robustness Techniques

- **Dedicated UDP ports per room:** removes the need to inspect RTP extensions for room routing and prevents cross-room media ambiguity.
- **Socket reservation before VM startup:** keeps allocated ports bound until ingest owns them, avoiding allocation races.
- **RTP pooling and reference counting:** reuses packet buffers and tracks fanout ownership so high-rate media does not create unnecessary garbage collection pressure.
- **Bounded per-session media queues:** applies controlled packet drops under slow-consumer pressure rather than blocking the ingest loop for every viewer.
- **Snapshot-based room membership:** permits low-contention media fanout reads while serializing membership updates and preventing duplicates.
- **Lifecycle cancellation:** deleting a room cancels its VM provisioning context before teardown, avoiding orphan VMs after a user exits early.
- **Failure-state ordering:** stops ingest before reporting VM provisioning failure, so external state accurately reflects allocated resources.
- **Control ownership enforcement:** only the room’s active controller may forward input; control is released when that session leaves.
- **Agent stream resilience:** gRPC streams reconnect after transport errors, and commands are dropped safely when no matching agent stream exists.
- **Input validation:** the VM agent only accepts supported key actions, normalized mouse coordinates, recognized pointer actions, and valid scroll directions before executing `xdotool`.
- **Deployment preflight:** the agent fails immediately when `DISPLAY` or `xdotool` is unavailable, making an invalid VM image visible instead of silently accepting unusable control input.
- **Secret handling:** the agent token is required, gRPC binds only to the Tailnet interface, and the VM environment file is written with mode `0600`.

## Security and Current Limits

For private test sessions, Tailnet transport plus the agent bearer token provides a reasonable baseline. Public deployment still requires browser authentication/authorization and should use mTLS or per-VM short-lived credentials for the agent channel. RTP ingest does not currently relay RTCP PLI/NACK feedback to GStreamer, so the VM-to-SFU media path should remain private and reliable until that is implemented.
