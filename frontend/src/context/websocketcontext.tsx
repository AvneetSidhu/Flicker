import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

type WebSocketContextType = {
  socket: WebSocket | null;
  isConnected: boolean;
  isConnecting: boolean;
  error: string | null;
  connect: (displayName: string) => void;
  disconnect: () => void;
  send: (data: string | ArrayBufferLike | Blob | ArrayBufferView) => boolean;
  sendJson: (msg: unknown) => boolean;
  addMessageListener: (fn: (msg: any) => void) => void;
  removeMessageListener: (fn: (msg: any) => void) => void;
};

const WebSocketContext = createContext<WebSocketContextType | undefined>(
  undefined
);

function buildWSUrl(displayName: string, userId: string): string {
  const protocol = window.location.protocol === "https:" ? "wss" : "ws";
  const host = window.location.host; // host includes hostname *and* port automatically
  const params = new URLSearchParams({
    displayname: displayName,
    userid: userId,
  });
  return `${protocol}://${host}/ws?${params.toString()}`;
}

export const WebSocketProvider = ({
  children,
}: {
  children: React.ReactNode;
}) => {
  const [socket, setSocket] = useState<WebSocket | null>(null);
  const [isConnected, setIsConnected] = useState(false);
  const [isConnecting, setIsConnecting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const openListenerRef = useRef<(() => void) | null>(null);
  const listenersRef = useRef(new Set<(msg: any) => void>());

  const disconnect = useCallback(() => {
    if (socket) {
      try {
        socket.close();
      } catch {}
    }
    setIsConnected(false);
    setIsConnecting(false);
    setSocket(null);
  }, [socket]);

  const connect = useCallback(
    (displayName: string) => {
      if (!displayName?.trim()) return;
      setError(null);
      setIsConnecting(true);

      let userId = localStorage.getItem("userId");
      if (!userId) {
        try {
          userId = (crypto as any).randomUUID
            ? crypto.randomUUID()
            : Math.random().toString(36).slice(2);
        } catch {
          userId = Math.random().toString(36).slice(2);
        }
        localStorage.setItem("userId", userId);
      }

      const url = buildWSUrl(displayName.trim(), userId);

      try {
        // Close any previous socket before creating a new one
        if (socket) {
          try {
            socket.close();
          } catch {}
        }

        const ws = new WebSocket(url);
        setSocket(ws);

        ws.onopen = () => {
          setIsConnected(true);
          setIsConnecting(false);
          setError(null);
          if (openListenerRef.current) openListenerRef.current();
        };

        ws.onerror = () => {
          setError("WebSocket error");
        };

        ws.onclose = () => {
          setIsConnected(false);
          setIsConnecting(false);
        };

        ws.onmessage = (event) => {
          try {
            const data = JSON.parse(event.data);
            listenersRef.current.forEach((fn) => {
              try {
                fn(data);
              } catch (err) {
                console.error("WS listener error", err);
              }
            });
          } catch (err) {
            console.error("WS message parse error", err);
          }
        };
      } catch (e: any) {
        setIsConnecting(false);
        setError(e?.message || "Failed to connect");
      }
    },
    [socket]
  );

  const send = useCallback(
    (data: string | ArrayBufferLike | Blob | ArrayBufferView) => {
      if (socket && socket.readyState === WebSocket.OPEN) {
        socket.send(data);
        return true;
      }
      return false;
    },
    [socket]
  );

  const sendJson = useCallback(
    (msg: unknown) => send(JSON.stringify(msg)),
    [send]
  );

  const addMessageListener = useCallback((fn: (msg: any) => void) => {
    listenersRef.current.add(fn);
  }, []);

  const removeMessageListener = useCallback((fn: (msg: any) => void) => {
    listenersRef.current.delete(fn);
  }, []);

  useEffect(() => {
    return () => {
      // Cleanup on unmount
      try {
        socket?.close();
      } catch {}
    };
  }, [socket]);

  const value = useMemo(
    () => ({
      socket,
      isConnected,
      isConnecting,
      error,
      connect,
      disconnect,
      send,
      sendJson,
      addMessageListener,
      removeMessageListener,
    }),
    [
      socket,
      isConnected,
      isConnecting,
      error,
      connect,
      disconnect,
      send,
      sendJson,
      addMessageListener,
      removeMessageListener,
    ]
  );

  return (
    <WebSocketContext.Provider value={value}>
      {children}
    </WebSocketContext.Provider>
  );
};

export const useWebSocket = () => {
  const ctx = useContext(WebSocketContext);
  if (!ctx) {
    throw new Error("useWebSocket must be used within WebSocketProvider");
  }
  return ctx;
};
