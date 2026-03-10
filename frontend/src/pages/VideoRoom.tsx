import { useEffect, useState, useRef, useCallback } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Slider } from "@/components/ui/slider";
import { ArrowLeft, Volume2, VolumeX, Maximize, Users } from "lucide-react";
import { useWebSocket } from "@/context/websocketcontext";

interface Watcher {
  id: string;
  name: string;
  joinedAt: Date;
}

const VideoRoom = () => {
  const navigate = useNavigate();
  const { roomId } = useParams();
  const [displayName, setDisplayName] = useState("");
  const [isMuted, setIsMuted] = useState(false);
  const [volume, setVolume] = useState([75]);
  const [watchers, setWatchers] = useState<Watcher[]>([]);
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

  const handleSignalMessage = useCallback(
    async (msg: any) => {
      console.log("[Signal] received message type:", msg.type);
      if (!pcRef.current) {
        console.warn("[Signal] no PeerConnection yet, ignoring", msg.type);
        return;
      }
      if (msg.type === "answer" && msg.payload?.sdp) {
        console.log("[Signal] setting remote description (answer)");
        const desc = new RTCSessionDescription({
          type: "answer",
          sdp: msg.payload.sdp,
        });
        await pcRef.current.setRemoteDescription(desc);
        console.log("[Signal] remote description set");
      }
      if (msg.type === "ice-candidate" && msg.payload?.candidate) {
        console.log("[Signal] adding remote ICE candidate", msg.payload.candidate);
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
    },
    []
  );

  const waitForRoomLive = useCallback(async (): Promise<boolean> => {
    if (!roomId) return false;
    for (let i = 0; i < 60; i++) {
      try {
        const res = await fetch("/rooms");
        if (res.ok) {
          const rooms: { id: string; isLive: boolean }[] = await res.json();
          const room = rooms.find((r) => r.id === roomId);
          console.log(`[Poll] attempt ${i + 1}: room=${JSON.stringify(room)}`);
          if (room?.isLive) return true;
        }
      } catch (err) {
        console.warn("[Poll] fetch error:", err);
      }
      await new Promise((resolve) => setTimeout(resolve, 2000));
    }
    return false;
  }, [roomId]);

  const startWebRTC = useCallback(async () => {
    if (!roomId || pcRef.current || webrtcStartingRef.current) return;
    webrtcStartingRef.current = true;
    console.log("[WebRTC] waiting for VM to be live...");
    const live = await waitForRoomLive();
    if (!live) {
      console.warn("[WebRTC] timed out waiting for room to go live");
      return;
    }
    console.log("[WebRTC] room is live, starting WebRTC");
    console.log("[WebRTC] creating PeerConnection");
    const pc = new RTCPeerConnection({
      iceServers: [{ urls: "stun:stun.l.google.com:19302" }],
    });

    pc.onicecandidate = (event) => {
      if (event.candidate) {
        console.log("[WebRTC] local ICE candidate", event.candidate.candidate);
        sendJson({
          type: "ice-candidate",
          payload: {
            candidate: event.candidate.candidate,
            sdpMid: event.candidate.sdpMid,
            sdpMLineIndex: event.candidate.sdpMLineIndex,
          },
        });
      } else {
        console.log("[WebRTC] local ICE gathering complete");
      }
    };

    pc.onicecandidateerror = (event) => {
      console.warn("[WebRTC] ICE candidate error", event);
    };

    pc.oniceconnectionstatechange = () => {
      console.log("[WebRTC] ICE connection state:", pc.iceConnectionState);
    };

    pc.onicegatheringstatechange = () => {
      console.log("[WebRTC] ICE gathering state:", pc.iceGatheringState);
    };

    pc.onsignalingstatechange = () => {
      console.log("[WebRTC] signaling state:", pc.signalingState);
    };

    pc.onconnectionstatechange = () => {
      console.log("[WebRTC] connection state:", pc.connectionState);
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
    console.log("[WebRTC] local description set, sending offer");

    sendJson({
      type: "join-room",
      payload: { roomId },
    });

    sendJson({
      type: "offer",
      payload: { sdp: offer.sdp },
    });

    pcRef.current = pc;
  }, [attachStream, roomId, sendJson, waitForRoomLive]);

  useEffect(() => {
    if (!socket || !isConnected) return;
    addMessageListener(handleSignalMessage);
    startWebRTC();
    return () => {
      removeMessageListener(handleSignalMessage);
    };
  }, [socket, isConnected, addMessageListener, removeMessageListener, handleSignalMessage, startWebRTC]);

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

  const handleBack = () => {
    navigate("/rooms");
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

  return (
    <div className="min-h-screen bg-background flex flex-col">
      {/* Header */}
      <header className="border-b border-border backdrop-blur-xl bg-background/80 z-50">
        <div className="container mx-auto px-6 py-4 flex items-center justify-between">
          <Button
            variant="outline"
            size="sm"
            onClick={handleBack}
            className="border-border hover:bg-secondary"
          >
            <ArrowLeft className="w-4 h-4 mr-2" />
            Back to Rooms
          </Button>
          
          <div className="flex items-center gap-2 text-muted-foreground text-sm">
            <Users className="w-4 h-4" />
            <span>{watchers.length} watching</span>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <div className="flex-1 flex overflow-hidden">
        {/* Video Section */}
        <div className="flex-1 flex flex-col p-6 gap-4">
          <div className="flex-1 relative rounded-2xl overflow-hidden bg-black shadow-card">
            <video
              ref={videoRef}
              className="w-full h-full object-contain"
              autoPlay
              playsInline
              muted={isMuted}
            />
          </div>

          {/* Video Controls */}
          <div className="backdrop-blur-xl bg-card/50 border border-border rounded-2xl p-4">
            <div className="flex items-center gap-6">
              <Button
                variant="outline"
                size="icon"
                onClick={toggleMute}
                className="border-border hover:bg-secondary shrink-0"
              >
                {isMuted ? (
                  <VolumeX className="w-5 h-5" />
                ) : (
                  <Volume2 className="w-5 h-5" />
                )}
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
        <aside className="w-80 border-l border-border backdrop-blur-xl bg-card/30 p-6 overflow-y-auto">
          <div className="mb-6">
            <h2 className="text-xl font-bold mb-1">Watchers</h2>
            <p className="text-sm text-muted-foreground">
              {watchers.length} {watchers.length === 1 ? 'person' : 'people'} watching
            </p>
          </div>

          <div className="space-y-2">
            {watchers.map((watcher) => (
              <div
                key={watcher.id}
                className="flex items-center gap-3 p-3 rounded-xl bg-secondary/50 border border-border/50 hover:bg-secondary transition-colors"
              >
                <div className="w-10 h-10 rounded-full bg-gradient-primary flex items-center justify-center text-primary-foreground font-semibold shadow-glow">
                  {watcher.name[0].toUpperCase()}
                </div>
                <div className="flex-1 min-w-0">
                  <p className="font-medium truncate">
                    {watcher.name}
                    {watcher.id === "current" && (
                      <span className="ml-2 text-xs text-muted-foreground">(you)</span>
                    )}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {getTimeAgo(watcher.joinedAt)}
                  </p>
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
