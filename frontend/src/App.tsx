import { Toaster } from "@/components/ui/toaster";
import { Toaster as Sonner } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Routes, Route } from "react-router-dom";
import Landing from "./pages/Landing";
import Rooms from "./pages/Rooms";
import VideoRoom from "./pages/VideoRoom";
import NotFound from "./pages/NotFound";
import { WebSocketProvider } from "./context/websocketcontext";

const queryClient = new QueryClient();

const App = () => (
  <QueryClientProvider client={queryClient}>
    <TooltipProvider>
      <Toaster />
      <Sonner />
      <BrowserRouter>
        <WebSocketProvider>
          <Routes>
            <Route path="/" element={<Landing />} />
            <Route path="/rooms" element={<Rooms />} />
            <Route path="/room/:roomId" element={<VideoRoom />} />
            <Route path="*" element={<NotFound />} />
          </Routes>
        </WebSocketProvider>
      </BrowserRouter>
    </TooltipProvider>
  </QueryClientProvider>
);

export default App;
