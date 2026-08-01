import { act, render, screen } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import VideoRoom from "./VideoRoom";

const connect = vi.fn();
const sendJson = vi.fn();
const addMessageListener = vi.fn();
const removeMessageListener = vi.fn();

vi.mock("@/context/websocketcontext", () => ({
  useWebSocket: () => ({
    socket: {},
    isConnected: true,
    isConnecting: false,
    connect,
    sendJson,
    addMessageListener,
    removeMessageListener,
  }),
}));

describe("VideoRoom", () => {
  beforeEach(() => {
    localStorage.setItem("displayName", "Viewer");
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    localStorage.clear();
    vi.unstubAllGlobals();
  });

  it("does not delete the shared room when a viewer leaves the page", () => {
    const view = render(
      <MemoryRouter initialEntries={["/room/shared-room"]}>
        <Routes>
          <Route path="/room/:roomId" element={<VideoRoom />} />
        </Routes>
      </MemoryRouter>,
    );

    view.unmount();

    expect(fetch).not.toHaveBeenCalled();
  });

  it("renders server-authoritative room presence", () => {
    render(
      <MemoryRouter initialEntries={["/room/shared-room"]}>
        <Routes>
          <Route path="/room/:roomId" element={<VideoRoom />} />
        </Routes>
      </MemoryRouter>,
    );

    const listener = addMessageListener.mock.calls[0][0];
    act(() => {
      listener({
        type: "room-presence",
        payload: { viewers: [{ id: "one", name: "One" }, { id: "two", name: "Two" }] },
      });
    });

    expect(screen.getByText("2 watching")).toBeInTheDocument();
    expect(screen.getByText("One")).toBeInTheDocument();
    expect(screen.getByText("Two")).toBeInTheDocument();
  });
});
