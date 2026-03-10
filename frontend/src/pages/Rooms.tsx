import { useEffect, useState, useCallback } from "react";
import { useNavigate } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Users, Video, LogOut, Plus, Trash2 } from "lucide-react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useWebSocket } from "@/context/websocketcontext";

interface Room {
  id: string;
  name: string;
  creator: string;
  viewers: number;
  isLive: boolean;
}

const Rooms = () => {
  const navigate = useNavigate();
  const [displayName, setDisplayName] = useState("");
  const [rooms, setRooms] = useState<Room[]>([]);
  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false);
  const [newRoomName, setNewRoomName] = useState("");
  const { isConnected, isConnecting, connect, sendJson, addMessageListener, removeMessageListener } = useWebSocket();

  const fetchRooms = useCallback(async () => {
    try {
      const res = await fetch("/rooms");
      if (!res.ok) return;
      const data = await res.json();
      setRooms(data ?? []);
    } catch {
      // server unreachable; leave list empty
    }
  }, []);

  useEffect(() => {
    const name = localStorage.getItem("displayName");
    if (!name) {
      navigate("/");
      return;
    }
    setDisplayName(name);
    fetchRooms();

    if (!isConnected && !isConnecting) {
      connect(name);
    }
  }, [navigate, fetchRooms, isConnected, isConnecting, connect]);

  useEffect(() => {
    const interval = setInterval(fetchRooms, 5000);
    return () => clearInterval(interval);
  }, [fetchRooms]);

  // Navigate into a room once the backend confirms creation and returns the roomId
  useEffect(() => {
    const onMessage = (msg: any) => {
      if (msg.type === "room-created" && msg.payload?.roomId) {
        fetchRooms();
        navigate(`/room/${msg.payload.roomId}`);
      }
    };
    addMessageListener(onMessage);
    return () => removeMessageListener(onMessage);
  }, [addMessageListener, removeMessageListener, fetchRooms, navigate]);

  const handleLogout = () => {
    localStorage.removeItem("displayName");
    navigate("/");
  };

  const handleRoomClick = (roomId: string) => {
    navigate(`/room/${roomId}`);
  };

  const handleCreateRoom = () => {
    if (!newRoomName.trim()) return;
    sendJson({ type: "create-room", payload: { roomName: newRoomName.trim() } });
    setNewRoomName("");
    setIsCreateDialogOpen(false);
  };

  const handleDeleteRoom = async (e: React.MouseEvent, roomId: string) => {
    e.stopPropagation();
    try {
      await fetch(`/rooms/${roomId}`, { method: "DELETE" });
      fetchRooms();
    } catch {
      // ignore
    }
  };

  return (
    <div className="min-h-screen bg-background">
      {/* Header */}
      <header className="border-b border-border bg-background sticky top-0 z-50">
        <div className="container mx-auto px-6 py-4 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="w-10 h-10 rounded bg-primary flex items-center justify-center">
              <Video className="w-5 h-5 text-primary-foreground" />
            </div>
            <h1 className="text-2xl font-bold text-foreground">
              Flicker
            </h1>
          </div>
          
          <div className="flex items-center gap-4">
            <span className="text-xs text-muted-foreground/50 font-mono">
              {__BUILD_DATE__}{__BUILD_TIME__ ? ` ${__BUILD_TIME__}` : ""}
            </span>
            <span className="text-sm text-muted-foreground">
              Welcome, <span className="text-foreground font-medium">{displayName}</span>
            </span>
            <Button
              variant="outline"
              size="sm"
              onClick={handleLogout}
              className="border-border hover:bg-secondary"
            >
              <LogOut className="w-4 h-4 mr-2" />
              Logout
            </Button>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main className="container mx-auto px-6 py-8">
        <div className="mb-8 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-3xl font-bold mb-2">Live Rooms</h2>
            <p className="text-muted-foreground">Join a room and watch together</p>
          </div>
          
          <div className="flex items-center gap-3">
          <Dialog open={isCreateDialogOpen} onOpenChange={setIsCreateDialogOpen}>
            <DialogTrigger asChild>
              <Button>
                <Plus className="w-4 h-4 mr-2" />
                Create Room
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Create a New Room</DialogTitle>
                <DialogDescription>
                  Enter a name for your video room. Others can join and watch together.
                </DialogDescription>
              </DialogHeader>
              <div className="py-4">
                <Label htmlFor="room-name" className="text-sm font-medium mb-2 block">
                  Room Name
                </Label>
                <Input
                  id="room-name"
                  placeholder="e.g., Movie Night, Gaming Session"
                  value={newRoomName}
                  onChange={(e) => setNewRoomName(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && newRoomName.trim()) {
                      handleCreateRoom();
                    }
                  }}
                />
              </div>
              <DialogFooter>
                <Button
                  variant="outline"
                  onClick={() => {
                    setIsCreateDialogOpen(false);
                    setNewRoomName("");
                  }}
                >
                  Cancel
                </Button>
                <Button
                  onClick={handleCreateRoom}
                  disabled={!newRoomName.trim()}
                >
                  Create Room
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>
          </div>
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {rooms.map((room) => (
            <button
              key={room.id}
              onClick={() => handleRoomClick(room.id)}
              className="group relative overflow-hidden rounded bg-card border border-border hover:border-primary transition-colors p-6 text-left"
            >
              <div className="flex items-start justify-between mb-4">
                {/* Live Badge */}
                {room.isLive && (
                  <div className="px-3 py-1 rounded bg-destructive text-destructive-foreground text-xs font-semibold flex items-center gap-1">
                    <span className="w-2 h-2 rounded bg-destructive-foreground" />
                    LIVE
                  </div>
                )}

                <div className="flex items-center gap-2 ml-auto">
                  {/* Viewers */}
                  <div className="px-3 py-1 rounded bg-secondary text-xs font-medium flex items-center gap-1">
                    <Users className="w-3 h-3" />
                    {room.viewers}
                  </div>

                  {/* Delete */}
                  <button
                    onClick={(e) => handleDeleteRoom(e, room.id)}
                    className="p-1.5 rounded hover:bg-destructive/10 text-muted-foreground hover:text-destructive transition-colors"
                    title="Delete room"
                  >
                    <Trash2 className="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>

              {/* Room Info */}
              <div>
                <h3 className="text-xl font-semibold mb-2">
                  {room.name}
                </h3>
                <p className="text-sm text-muted-foreground">
                  Created by <span className="text-foreground">{room.creator}</span>
                </p>
              </div>
            </button>
          ))}
        </div>
      </main>
    </div>
  );
};

export default Rooms;
