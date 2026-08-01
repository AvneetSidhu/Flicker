# Flicker

Flicker provides browser-based viewing and exclusive remote control of a desktop running in a per-room libvirt VM. It consists of a Go WebRTC SFU, a VM-side GStreamer/X11 agent, and a React frontend.

## Architecture

- `cmd/sfu`: loopback HTTP/WebSocket signaling server and Tailnet-bound gRPC control server.
- `cmd/vmagent`: captures X11/PulseAudio, sends RTP to its room's dedicated UDP ports, receives control commands, and injects them with `xdotool`.
- `internal/sfu/room`: room lifecycle, controller ownership, presence, and RTP fanout.
- `internal/sfu/vm`: libvirt clone, configuration, start, destroy, and disk cleanup.
- `frontend`: React client for room creation, WebRTC viewing, presence, and control.

Data paths:

1. Browser WebSocket signaling reaches the SFU through Nginx.
2. The browser receives audio/video through WebRTC.
3. Each VM sends RTP to dedicated SFU UDP ports.
4. Authorized control events flow browser WebSocket → SFU → authenticated gRPC stream → VM agent → X11.

## Required Configuration

SFU environment:

```dotenv
PORT=50000
TAILNET_IP=100.x.y.z
SFU_GRPC_PORT=50051
SFU_GRPC_AUTH_TOKEN=replace-with-a-high-entropy-secret
VM_BASE_NAME=flicker-node
LOG_LEVEL=info
```

`SFU_GRPC_AUTH_TOKEN` is mandatory and must be a high-entropy secret. The SFU binds gRPC only to `TAILNET_IP`; the VM agent receives the token in `/etc/vmagent.env` with mode `0600`.

## Development

```powershell
go test ./...
go vet ./...

cd frontend
npm install
npm run lint
npm test
npm run build
```

The frontend development server needs a proxy for `/ws` and `/rooms`, or the backend must be reachable through the same origin.

## Deployment

The SFU intentionally listens on `127.0.0.1:$PORT`. Deploy `deploy/nginx/flicker.conf` as an Nginx virtual host, replace `flicker.example.com`, configure a TLS certificate, and serve the frontend build from `/home/deploy/app/frontend`.

The workflow in `.github/workflows/deploy.yml` builds and deploys the SFU, frontend, and VM-agent systemd unit. The runner requires libvirt, KVM, libguestfs tools, access to the base domain, and permission to restart the SFU and reload Nginx.

See `vm.txt` for the complete VM-image and host prerequisites.

See `SYSTEM_DESIGN.md` for the architecture, lifecycle, stack tradeoffs, and reliability techniques.

## Security Boundary

- Browsers must connect through HTTPS/WSS at the reverse proxy.
- The current browser identity is an application-generated ID and is **not authentication**. Do not expose Flicker publicly until an OIDC/JWT provider is configured and enforced for HTTP and WebSocket requests.
- SFU-to-agent commands use Tailnet transport plus a bearer token. Use mTLS or per-VM credentials when the agent path crosses an untrusted network or tenant boundary.

## RTCP Scope

RTP ingest runs over the private SFU/VM network. Browser-side WebRTC feedback is handled by Pion for the downstream connection, but feedback is not relayed back to the VM's GStreamer pipeline. For unreliable or internet-facing VM ingest, add RTCP feedback ports and wire PLI/NACK into the encoder pipeline before deployment.

## Tests

See `TESTING.md` for unit coverage and Linux integration requirements.
