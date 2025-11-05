import { createRoot } from "react-dom/client";
import App from "./App";
import "./index.css";
import { WebSocketProvider } from "./context/websocketcontext";

createRoot(document.getElementById("root")!).render(
  <WebSocketProvider>
    <App />
  </WebSocketProvider>
);
