import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";

export type WebSocketMessage = {
  type: string;
  payload?: Record<string, unknown>;
};

type MessageListener = (msg: WebSocketMessage) => void;

type WebSocketContextType = {
  socket: WebSocket | null;
  isConnected: boolean;
  isConnecting: boolean;
  error: string | null;
  connect: (displayName: string) => void;
  disconnect: () => void;
  send: (data: string | ArrayBufferLike | Blob | ArrayBufferView) => boolean;
  sendJson: (msg: unknown) => boolean;
  addMessageListener: (fn: MessageListener) => void;
  removeMessageListener: (fn: MessageListener) => void;
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

function isWebSocketMessage(value: unknown): value is WebSocketMessage {
  return typeof value === "object" && value !== null && "type" in value && typeof value.type === "string";
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
  const listenersRef = useRef(new Set<MessageListener>());

  const disconnect = useCallback(() => {
    if (socket) {
      socket.close();
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
        userId = typeof crypto.randomUUID === "function"
          ? crypto.randomUUID()
          : Math.random().toString(36).slice(2);
        localStorage.setItem("userId", userId);
      }

      const url = buildWSUrl(displayName.trim(), userId);

      try {
        // Close any previous socket before creating a new one
        if (socket) {
          socket.close();
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
            const data: unknown = JSON.parse(event.data);
            if (!isWebSocketMessage(data)) {
              console.error("WS message has an invalid shape");
              return;
            }
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
      } catch (error: unknown) {
        setIsConnecting(false);
        setError(error instanceof Error ? error.message : "Failed to connect");
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

  const addMessageListener = useCallback((fn: MessageListener) => {
    listenersRef.current.add(fn);
  }, []);

  const removeMessageListener = useCallback((fn: MessageListener) => {
    listenersRef.current.delete(fn);
  }, []);

  useEffect(() => {
    return () => {
      // Cleanup on unmount
      socket?.close();
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
