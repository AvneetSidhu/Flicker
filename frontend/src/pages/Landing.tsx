import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Video } from "lucide-react";
import { useWebSocket } from "@/context/websocketcontext";

const Landing = () => {
  const [displayName, setDisplayName] = useState("");
  const navigate = useNavigate();
  const { connect } = useWebSocket();

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (displayName.trim()) {
      localStorage.setItem("displayName", displayName.trim());
      // Initialize WebSocket to Go server after name is set
      connect(displayName.trim());
      navigate("/rooms");
    }
  };

  return (
    <div className="min-h-screen flex items-center justify-center bg-background relative overflow-hidden">
      <div className="absolute inset-0 bg-gradient-hero opacity-60 animate-pulse" />
      <div className="absolute top-0 -left-4 w-96 h-96 bg-primary/20 rounded-full blur-3xl animate-pulse" />
      <div className="absolute bottom-0 -right-4 w-96 h-96 bg-accent/20 rounded-full blur-3xl animate-pulse delay-700" />

      <div className="relative z-10 w-full max-w-md px-6">
        <div className="text-center mb-8 animate-in fade-in slide-in-from-bottom-4 duration-700">
          <div className="inline-flex items-center justify-center w-24 h-24 rounded-2xl bg-gradient-primary mb-6 shadow-glow animate-in scale-in duration-500">
            <Video className="w-12 h-12 text-primary-foreground" />
          </div>
          <h1 className="text-6xl font-bold mb-3 bg-gradient-primary bg-clip-text text-transparent animate-in fade-in duration-700 delay-100">
            Flicker
          </h1>
          <p className="text-muted-foreground text-xl animate-in fade-in duration-700 delay-200">
            Join watch parties with friends
          </p>
        </div>

        <form
          onSubmit={handleSubmit}
          className="space-y-4 animate-in fade-in slide-in-from-bottom-8 duration-700 delay-150"
        >
          <div className="backdrop-blur-xl bg-card/50 border-2 border-primary/20 rounded-2xl p-6 shadow-card hover:border-primary/40 transition-all duration-300">
            <label
              htmlFor="displayName"
              className="block text-sm font-medium mb-2 text-foreground"
            >
              Display Name
            </label>
            <Input
              id="displayName"
              type="text"
              placeholder="Enter your name"
              value={displayName}
              onChange={(e) => setDisplayName(e.target.value)}
              className="bg-background/80 border-primary/30 text-lg h-12 focus:border-primary transition-colors"
              maxLength={20}
              required
            />
          </div>

          <Button
            type="submit"
            className="w-full h-14 text-lg font-semibold bg-gradient-primary hover:scale-105 transition-all duration-300 shadow-glow hover:shadow-[0_0_60px_hsl(263_70%_50%_/_0.6)]"
            disabled={!displayName.trim()}
          >
            Enter Rooms
          </Button>
        </form>

        <p className="text-center text-sm text-muted-foreground mt-6 animate-in fade-in duration-700 delay-300">
          Pick a name and start watching together
        </p>
      </div>
    </div>
  );
};

export default Landing;
