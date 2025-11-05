import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Users, Video, LogOut, Plus } from "lucide-react";
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

interface Room {
  id: string;
  name: string;
  creator: string;
  viewers: number;
  isLive: boolean;
}

const mockRooms: Room[] = [
  {
    id: "1",
    name: "Movie Night",
    creator: "Alex",
    viewers: 24,
    isLive: true,
  },
  {
    id: "2",
    name: "Gaming Stream",
    creator: "Jordan",
    viewers: 142,
    isLive: true,
  },
  {
    id: "3",
    name: "Chill Vibes",
    creator: "Sam",
    viewers: 56,
    isLive: true,
  },
  {
    id: "4",
    name: "Study Session",
    creator: "Taylor",
    viewers: 89,
    isLive: true,
  },
  {
    id: "5",
    name: "Anime Club",
    creator: "Morgan",
    viewers: 203,
    isLive: true,
  },
  {
    id: "6",
    name: "Workout Together",
    creator: "Casey",
    viewers: 67,
    isLive: true,
  },
];

const Rooms = () => {
  const navigate = useNavigate();
  const [displayName, setDisplayName] = useState("");
  const [rooms, setRooms] = useState<Room[]>(mockRooms);
  const [isCreateDialogOpen, setIsCreateDialogOpen] = useState(false);
  const [newRoomName, setNewRoomName] = useState("");

  useEffect(() => {
    const name = localStorage.getItem("displayName");
    if (!name) {
      navigate("/");
      return;
    }
    setDisplayName(name);
  }, [navigate]);

  const handleLogout = () => {
    localStorage.removeItem("displayName");
    navigate("/");
  };

  const handleRoomClick = (roomId: string) => {
    navigate(`/room/${roomId}`);
  };

  const handleCreateRoom = () => {
    if (!newRoomName.trim()) return;
    
    const newRoom: Room = {
      id: Date.now().toString(),
      name: newRoomName,
      creator: displayName,
      viewers: 1,
      isLive: true,
    };
    
    setRooms([newRoom, ...rooms]);
    setNewRoomName("");
    setIsCreateDialogOpen(false);
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
        <div className="mb-8 flex items-center justify-between">
          <div>
            <h2 className="text-3xl font-bold mb-2">Live Rooms</h2>
            <p className="text-muted-foreground">Join a room and watch together</p>
          </div>
          
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

                {/* Viewers */}
                <div className="px-3 py-1 rounded bg-secondary text-xs font-medium flex items-center gap-1 ml-auto">
                  <Users className="w-3 h-3" />
                  {room.viewers}
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
