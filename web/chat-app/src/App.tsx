import React, { useState, useEffect, useRef, useCallback } from 'react';

type User = {
  user_id: string;
  username: string;
};

type ChatMessage = {
  message_id: string;
  username: string;
  body: string;
};

function App() {
  const [input, setInput] = useState<string>("");
  const [server, setServer] = useState<string>("");
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [user, setUser] = useState<User | null>(null);
  const [username, setUsername] = useState<string>("");
  const [joining, setJoining] = useState<boolean>(false);
  const [error, setError] = useState<string>("");
  const [connected, setConnected] = useState<boolean>(false);

  const wsRef = useRef<WebSocket | null>(null);

  async function handleJoin(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (!username.trim() || joining) return;

    setJoining(true);
    setError("");

    try {
      const response = await fetch("http://localhost:8880/users", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ username: username.trim() }),
      });

      if (!response.ok) throw new Error("Could not create user");

      setUser(await response.json());
    } catch {
      setError("Could not join, Please try again.");
    } finally {
      setJoining(false);
    }
  }

  useEffect(() => {
    if (!user) return;

    let stopped = false;
    let retryDelay = 1_000;
    let retryTimer: ReturnType<typeof setTimeout> | undefined;
    let socket: WebSocket | null = null;

    function connect() {
      if (stopped) return;
      if (!user) return;

      const ws = new WebSocket(`ws://localhost:8880/echo?user_id=${encodeURIComponent(user.user_id)}`);

      socket = ws;
      wsRef.current = ws;

      ws.onopen = () => {
        if (stopped) return;
        retryDelay = 1_000;
        setConnected(true);
      };

      ws.onmessage = (event: MessageEvent<string>) => {
        if (stopped) return;

        try {
          const message = JSON.parse(event.data);

          if (message?.type === "server_info") {
            setServer(`Connected to ${message.server}`);
            return;
          }

          if (
            typeof message?.message_id === "string" &&
            typeof message?.username === "string" &&
            typeof message?.body === "string"
          ) {
            setMessages((prev) => [...prev, message])
          }
        } catch {
          console.error("Received invalid JSON:", event.data)
        }
      };

      ws.onclose = () => {
        if (stopped) return;

        wsRef.current = null;
        setConnected(false);
        setServer("Disconnected. Retrying...")

        retryTimer = setTimeout(
          connect,
          retryDelay + Math.random() * 500
        );
        retryDelay = Math.min(retryDelay * 2, 30_000);
      }

      ws.onerror = () => {
        ws.close();
      }

    };
    connect();

    return () => {
      stopped = true;
      clearTimeout(retryTimer);
      socket?.close();
      wsRef.current = null
    };
  }, [user]);

  const send = useCallback((text: string): boolean => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return false;

    ws.send(text);
    return true;
  }, []);

  const handleSend = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    if (!input.trim()) return;

    if (send(input)) {
      setInput("");
    }
  }

  if (!user) {
    return (
      <form onSubmit={handleJoin}>
        <input
          value={username}
          onChange={(e) => setUsername(e.target.value)}
          placeholder="Enter your username"
          aria-label="Username"
          required
        />
        <button disabled={joining || !username.trim()}>
          {joining ? "Joining..." : "Join chat"}
        </button>
        {error && <p role="alert">{error}</p>}
      </form>
    );
  } else {
    return (
      < div >
        <h1>WebSocket Echo</h1>
        <p>{server}</p>
        <ul>
          {messages.map((message: ChatMessage) => (
            <li key={message.message_id}>
              {message.username} &gt; {message.body}
            </li>
          ))}
        </ul>
        <form onSubmit={handleSend}>
          <input value={input} onChange={(e) => setInput(e.target.value)} placeholder="Type a message..." />
          <button type="submit" disabled={!connected || !input.trim()}>Send</button>
        </form>
      </div >
    );
  }
}

export default App;
