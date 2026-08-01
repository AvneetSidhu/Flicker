# Flicker Project Audit

Reviewed: 2026-07-31

## Completed

- Room creation uses UUIDs, dedicated reserved RTP ports, real VM provisioning, failure state, cancellation, and cleanup.
- The browser control path is complete: controller ownership, keyboard/mouse/wheel forwarding, gRPC delivery, and X11 injection.
- Room presence is server-authoritative and rendered live in the frontend.
- gRPC command streams authenticate with `SFU_GRPC_AUTH_TOKEN`, reconnect after transport errors, bind to the SFU Tailnet address, and validate commands before injection.
- Backend lifecycle, control, VM, ingest, session, HTTP, and VM-agent behavior have regression tests. `go vet ./...` and `go test ./...` pass.
- Frontend lint has no errors; eight Fast Refresh warnings remain in generated shadcn-style UI modules.
- The Nginx reverse-proxy contract, VM image requirements, and current architecture are documented in `deploy/nginx/flicker.conf`, `README.md`, and `vm.txt`.

## Remaining Release Blockers

1. **Browser authentication and authorization.** Browser identities are local generated IDs, not verified principals. Choose and integrate an OIDC/JWT provider; validate identities for HTTP room deletion and WebSocket session creation, then derive room ownership/control from token claims.
2. **Agent identity for untrusted networks.** The current Tailnet-plus-shared-token design is suitable for a single private trust domain. Use mTLS or short-lived per-VM credentials if agents or the SFU cross trust boundaries.
3. **RTCP feedback for lossy VM ingest.** RTP ingest is appropriate on the private host/VM network, but PLI/NACK is not relayed to GStreamer. Add encoder feedback and integration tests before placing ingest on a lossy network.
4. **Linux integration validation.** Exercise libvirt clone/delete, X11/PulseAudio capture, GStreamer RTP, WebRTC negotiation, and the Nginx TLS deployment on the target host. Run `go test -race ./...` in Linux CI with a 64-bit C compiler.

## Deferred Product Enhancements

- Retry and user-triggered recovery after failed VM provisioning.
- Push room-index updates instead of five-second polling.
- Replace remaining Fast Refresh lint warnings by separating component exports from shared constants.

`unfinished_features.txt` is archived historical context; this file is the active backlog.
