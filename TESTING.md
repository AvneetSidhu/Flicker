# Testing Flicker

Run the backend test suite from the repository root:

```powershell
go test ./...
```

Run frontend unit tests and the production build from `frontend/`:

```powershell
npm test
npm run build
```

`npm run lint` completes with zero errors. The remaining Fast Refresh warnings are limited to shadcn-style UI component modules.

## Current coverage

- Control serialization, missing-agent handling, gRPC stream metadata, and forwarding.
- Room membership, duplicate prevention, concurrent joins, RTP fanout, and room lifecycle using fake ingest/VM services.
- Session protocol parsing, room joins, room-created replies, and cleanup after a room has already been deleted.
- VM command orchestration using a fake command runner; tests never invoke libvirt.
- Ingest pooled-buffer reference release and WebSocket request validation.
- VM-agent control stream forwarding, reconnect cancellation, room metadata, and bearer-token propagation.
- VM-agent injector validation for unsupported input commands and missing X11 dependencies.
- Video-room regression: unmounting a viewer does not delete the shared room.

## Integration coverage still required

Libvirt provisioning, real GStreamer capture, browser WebRTC negotiation, and UDP/RTCP behavior require a Linux integration environment with libvirt and media devices. Exercise the deployment path there before release. The VM base image must contain `xdotool`, allow local X11 access, and expose the active display as `DISPLAY=:0` to the agent service.

Set a high-entropy `SFU_GRPC_AUTH_TOKEN` in the SFU environment. VM provisioning writes that token into `/etc/vmagent.env` with mode `0600`; do not commit it or reuse a development token in production.

`go test -race ./...` requires a working 64-bit C compiler on Windows. The configured compiler currently does not support 64-bit mode, so run that command in CI or a properly provisioned development environment.
