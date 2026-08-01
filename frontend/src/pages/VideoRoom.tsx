import { useEffect, useState, useRef, useCallback } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Slider } from "@/components/ui/slider";
import { ArrowLeft, Volume2, VolumeX, Maximize, Users, Power, Loader2 } from "lucide-react";
import { type WebSocketMessage, useWebSocket } from "@/context/websocketcontext";

interface Watcher {
  id: string;
  name: string;
  joinedAt: Date;
}

type VMState = "idle" | "connecting" | "connected" | "stopping";

const VideoRoom = () => {
  const navigate = useNavigate();
  const { roomId } = useParams();
  const [displayName, setDisplayName] = useState("");
  const [isMuted, setIsMuted] = useState(false);
  const [volume, setVolume] = useState([75]);
  const [watchers, setWatchers] = useState<Watcher[]>([]);
  const [vmState, setVMState] = useState<VMState>("idle");
  const [isStreaming, setIsStreaming] = useState(false);
  const [hasControl, setHasControl] = useState(false);
  const videoRef = useRef<HTMLVideoElement>(null);
  const pcRef = useRef<RTCPeerConnection | null>(null);
  const remoteStreamRef = useRef<MediaStream | null>(null);
  const webrtcStartingRef = useRef(false);
  const {
    socket,
    isConnected,
    isConnecting,
    connect,
    sendJson,
    addMessageListener,
    removeMessageListener,
  } = useWebSocket();

  useEffect(() => {
    const name = localStorage.getItem("displayName");
    if (!name) {
      navigate("/");
      return;
    }
    setDisplayName(name);
    setWatchers([{ id: "current", name, joinedAt: new Date() }]);
  }, [navigate]);

  useEffect(() => {
    if (!isConnected && !isConnecting && displayName) {
      connect(displayName);
    }
  }, [isConnected, isConnecting, displayName, connect]);

  useEffect(() => {
    if (videoRef.current) {
      videoRef.current.volume = volume[0] / 100;
    }
  }, [volume]);

  const attachStream = useCallback(() => {
    if (videoRef.current && remoteStreamRef.current) {
      videoRef.current.srcObject = remoteStreamRef.current;
    }
  }, []);

  const handleSignalMessage = useCallback(async (msg: WebSocketMessage) => {
    if (msg.type === "control-state") {
      setHasControl(Boolean(msg.payload?.granted));
      return;
    }
    if (msg.type === "room-presence" && Array.isArray(msg.payload?.viewers)) {
      setWatchers(msg.payload.viewers.map((viewer: { id: string; name: string }) => ({
        id: viewer.id,
        name: viewer.name,
        joinedAt: new Date(),
      })));
      return;
    }
    console.log("[Signal] received message type:", msg.type);
    if (!pcRef.current) {
      console.warn("[Signal] no PeerConnection yet, ignoring", msg.type);
      return;
    }
    if (msg.type === "answer" && msg.payload?.sdp) {
      const desc = new RTCSessionDescription({ type: "answer", sdp: msg.payload.sdp });
      await pcRef.current.setRemoteDescription(desc);
    }
    if (msg.type === "ice-candidate" && msg.payload?.candidate) {
      try {
        await pcRef.current.addIceCandidate({
          candidate: msg.payload.candidate,
          sdpMid: msg.payload.sdpMid,
          sdpMLineIndex: msg.payload.sdpMLineIndex,
        });
      } catch (err) {
        console.error("[Signal] error adding ICE candidate", err);
      }
    }
  }, []);

  const toggleControl = useCallback(() => {
    sendJson({ type: hasControl ? "release-control" : "request-control", payload: {} });
  }, [hasControl, sendJson]);

  const startWebRTC = useCallback(async () => {
    if (!roomId || pcRef.current || webrtcStartingRef.current) return;
    webrtcStartingRef.current = true;
    setVMState("connecting");

    const pc = new RTCPeerConnection({
      iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
    });

    pc.onicecandidate = (event) => {
      if (event.candidate) {
        sendJson({
          type: "ice-candidate",
          payload: {
            candidate: event.candidate.candidate,
            sdpMid: event.candidate.sdpMid,
            sdpMLineIndex: event.candidate.sdpMLineIndex,
          },
        });
      }
    };

    pc.oniceconnectionstatechange = () => {
      console.log("[WebRTC] ICE connection state:", pc.iceConnectionState);
    };

    pc.onconnectionstatechange = () => {
      console.log("[WebRTC] connection state:", pc.connectionState);
      if (pc.connectionState === "connected") {
        setVMState("connected");
      } else if (pc.connectionState === "failed" || pc.connectionState === "disconnected") {
        setVMState("idle");
        webrtcStartingRef.current = false;
      }
    };

    pc.ontrack = (event) => {
      console.log("[WebRTC] ontrack fired", event.track.kind, event.track.id);
      if (!remoteStreamRef.current) {
        remoteStreamRef.current = new MediaStream();
      }
      remoteStreamRef.current.addTrack(event.track);
      attachStream();
    };

    pc.addTransceiver("audio", { direction: "recvonly" });
    pc.addTransceiver("video", { direction: "recvonly" });

    const offer = await pc.createOffer();
    await pc.setLocalDescription(offer);

    sendJson({ type: "join-room", payload: { roomId } });
    sendJson({ type: "offer", payload: { sdp: offer.sdp } });

    pcRef.current = pc;
  }, [attachStream, roomId, sendJson]);

  const stopVM = useCallback(async () => {
    if (!roomId) return;
    setVMState("stopping");

    if (pcRef.current) {
      pcRef.current.close();
      pcRef.current = null;
    }
    remoteStreamRef.current = null;
    webrtcStartingRef.current = false;
    setIsStreaming(false);
    if (videoRef.current) {
      videoRef.current.srcObject = null;
    }

    try {
      await fetch(`/rooms/${roomId}`, {
        method: "DELETE",
        headers: { "X-Flicker-User-ID": localStorage.getItem("userId") ?? "" },
      });
    } catch (err) {
      console.error("[VM] failed to delete room", err);
    }

    navigate("/rooms");
  }, [roomId, navigate]);

  // Register signal listener once connected — do NOT auto-start WebRTC
  useEffect(() => {
    if (!socket || !isConnected) return;
    addMessageListener(handleSignalMessage);
    return () => {
      removeMessageListener(handleSignalMessage);
    };
  }, [socket, isConnected, addMessageListener, removeMessageListener, handleSignalMessage]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video || !hasControl) return;

    const onKeyDown = (event: KeyboardEvent) => {
      event.preventDefault();
      sendJson({ type: "keyboard", payload: { key: event.key, action: "down" } });
    };
    const onKeyUp = (event: KeyboardEvent) => {
      event.preventDefault();
      sendJson({ type: "keyboard", payload: { key: event.key, action: "up" } });
    };
    const onMouse = (event: MouseEvent) => {
      const rect = video.getBoundingClientRect();
      if (!rect.width || !rect.height) return;
      sendJson({ type: "mouse", payload: { x: (event.clientX - rect.left) / rect.width, y: (event.clientY - rect.top) / rect.height, action: event.type === "mousemove" ? "move" : event.type, dragging: event.buttons > 0 } });
    };
    const onWheel = (event: WheelEvent) => {
      event.preventDefault();
      sendJson({ type: "scroll", payload: { direction: event.deltaY < 0 ? "up" : "down" } });
    };

    video.tabIndex = 0;
    video.addEventListener("keydown", onKeyDown);
    video.addEventListener("keyup", onKeyUp);
    video.addEventListener("mousemove", onMouse);
    video.addEventListener("mousedown", onMouse);
    video.addEventListener("mouseup", onMouse);
    video.addEventListener("wheel", onWheel, { passive: false });
    return () => {
      video.removeEventListener("keydown", onKeyDown);
      video.removeEventListener("keyup", onKeyUp);
      video.removeEventListener("mousemove", onMouse);
      video.removeEventListener("mousedown", onMouse);
      video.removeEventListener("mouseup", onMouse);
      video.removeEventListener("wheel", onWheel);
    };
  }, [hasControl, sendJson]);

  const toggleMute = () => {
    if (videoRef.current) {
      videoRef.current.muted = !isMuted;
      setIsMuted(!isMuted);
    }
  };

  const toggleFullscreen = () => {
    if (videoRef.current) {
      if (document.fullscreenElement) {
        document.exitFullscreen();
      } else {
        videoRef.current.requestFullscreen();
      }
    }
  };

  useEffect(() => {
    return () => {
      if (pcRef.current) {
        pcRef.current.close();
        pcRef.current = null;
      }
      remoteStreamRef.current = null;
    };
  }, []);

  const canLaunch = isConnected && vmState === "idle";

  return (
    <div className="h-screen overflow-hidden bg-background flex flex-col">
      {/* Header */}
      <header className="shrink-0 border-b border-border backdrop-blur-xl bg-background/80 z-50">
        <div className="px-6 py-3 flex items-center justify-between">
          <Button
            variant="outline"
            size="sm"
            onClick={() => navigate("/rooms")}
            className="border-border hover:bg-secondary"
          >
            <ArrowLeft className="w-4 h-4 mr-2" />
            Back
          </Button>

          <div className="flex items-center gap-3">
            <div className="flex items-center gap-2 text-muted-foreground text-sm">
              <Users className="w-4 h-4" />
              <span>{watchers.length} watching</span>
            </div>
            <Button size="sm" variant={hasControl ? "secondary" : "outline"} onClick={toggleControl} disabled={!isConnected}>
              {hasControl ? "Release Control" : "Request Control"}
            </Button>

            {/* Launch / Stop button */}
            {vmState === "idle" && (
              <Button
                size="sm"
                onClick={startWebRTC}
                disabled={!canLaunch}
                className="gap-2"
              >
                <Power className="w-4 h-4" />
                Launch VM
              </Button>
            )}
            {vmState === "connecting" && (
              <Button size="sm" disabled className="gap-2">
                <Loader2 className="w-4 h-4 animate-spin" />
                Connecting...
              </Button>
            )}
            {vmState === "connected" && (
              <Button
                size="sm"
                variant="destructive"
                onClick={stopVM}
                className="gap-2"
              >
                <Power className="w-4 h-4" />
                Stop VM
              </Button>
            )}
            {vmState === "stopping" && (
              <Button size="sm" variant="destructive" disabled className="gap-2">
                <Loader2 className="w-4 h-4 animate-spin" />
                Stopping...
              </Button>
            )}
          </div>
        </div>
      </header>

      {/* Main Content */}
      <div className="flex-1 flex overflow-hidden min-h-0">
        {/* Video Section */}
        <div className="flex-1 flex flex-col p-4 gap-3 min-w-0">
          {/* Video */}
          <div className="flex-1 relative rounded-2xl overflow-hidden bg-black min-h-0">
            {/* Idle */}
            {vmState === "idle" && (
              <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 text-muted-foreground">
                <Power className="w-10 h-10 opacity-30" />
                <p className="text-sm">Press Launch VM to start the stream</p>
              </div>
            )}

            {/* WebRTC negotiating */}
            {vmState === "connecting" && (
              <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 text-muted-foreground">
                <Loader2 className="w-10 h-10 animate-spin opacity-50" />
                <p className="text-sm">Connecting...</p>
              </div>
            )}

            {/* Connected but VM still booting — no frames yet */}
            {vmState === "connected" && !isStreaming && (
              <div className="absolute inset-0 flex flex-col items-center justify-center gap-4">
                <div className="relative flex items-center justify-center">
                  <div className="w-16 h-16 rounded-full border-4 border-primary/20 border-t-primary animate-spin" />
                  <Power className="absolute w-6 h-6 text-primary" />
                </div>
                <div className="text-center space-y-1">
                  <p className="text-sm font-medium text-foreground">VM is starting up</p>
                  <p className="text-xs text-muted-foreground">Waiting for stream...</p>
                </div>
              </div>
            )}

            <video
              ref={videoRef}
              className="w-full h-full object-contain"
              autoPlay
              playsInline
              muted={isMuted}
              onPlaying={() => setIsStreaming(true)}
            />
          </div>

          {/* Controls */}
          <div className="shrink-0 backdrop-blur-xl bg-card/50 border border-border rounded-2xl px-4 py-3">
            <div className="flex items-center gap-6">
              <Button
                variant="outline"
                size="icon"
                onClick={toggleMute}
                className="border-border hover:bg-secondary shrink-0"
              >
                {isMuted ? <VolumeX className="w-5 h-5" /> : <Volume2 className="w-5 h-5" />}
              </Button>

              <div className="flex-1 flex items-center gap-3">
                <Volume2 className="w-4 h-4 text-muted-foreground shrink-0" />
                <Slider
                  value={volume}
                  onValueChange={setVolume}
                  max={100}
                  step={1}
                  className="flex-1"
                  disabled={isMuted}
                />
                <span className="text-sm text-muted-foreground min-w-[3ch] text-right">
                  {volume[0]}
                </span>
              </div>

              <Button
                variant="outline"
                size="icon"
                onClick={toggleFullscreen}
                className="border-border hover:bg-secondary shrink-0"
              >
                <Maximize className="w-5 h-5" />
              </Button>
            </div>
          </div>
        </div>

        {/* Watchers Sidebar */}
        <aside className="w-72 shrink-0 border-l border-border backdrop-blur-xl bg-card/30 flex flex-col overflow-hidden">
          <div className="p-5 border-b border-border shrink-0">
            <h2 className="text-lg font-bold mb-0.5">Watchers</h2>
            <p className="text-sm text-muted-foreground">
              {watchers.length} {watchers.length === 1 ? "person" : "people"} watching
            </p>
          </div>

          <div className="flex-1 overflow-y-auto p-4 space-y-2">
            {watchers.map((watcher) => (
              <div
                key={watcher.id}
                className="flex items-center gap-3 p-3 rounded-xl bg-secondary/50 border border-border/50"
              >
                <div className="w-9 h-9 rounded-full bg-gradient-primary flex items-center justify-center text-primary-foreground font-semibold text-sm shrink-0">
                  {watcher.name[0].toUpperCase()}
                </div>
                <div className="flex-1 min-w-0">
                  <p className="font-medium text-sm truncate">
                    {watcher.name}
                    {watcher.id === "current" && (
                      <span className="ml-2 text-xs text-muted-foreground">(you)</span>
                    )}
                  </p>
                  <p className="text-xs text-muted-foreground">{getTimeAgo(watcher.joinedAt)}</p>
                </div>
              </div>
            ))}
          </div>
        </aside>
      </div>
    </div>
  );
};

function getTimeAgo(date: Date): string {
  const seconds = Math.floor((Date.now() - date.getTime()) / 1000);
  if (seconds < 60) return "Just now";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return `${minutes}m ago`;
  const hours = Math.floor(minutes / 60);
  return `${hours}h ago`;
}

export default VideoRoom;
