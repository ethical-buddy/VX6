import { invoke } from "@tauri-apps/api/core";
import { useState } from "react";

export default function App() {
  const [nodeName, setNodeName] = useState("alice");
  const [status, setStatus] = useState("idle");
  const [output, setOutput] = useState("");

  async function runStatus() {
    setStatus("checking status...");
    try {
      const out = await invoke<string>("vx6_status");
      setOutput(out);
      setStatus("status received");
    } catch (err) {
      setStatus("status failed");
      setOutput(String(err));
    }
  }

  async function initNode() {
    setStatus("initializing...");
    try {
      const out = await invoke<string>("vx6_init", { name: nodeName });
      setOutput(out);
      setStatus("initialized");
    } catch (err) {
      setStatus("init failed");
      setOutput(String(err));
    }
  }

  async function startNode() {
    setStatus("starting node...");
    try {
      const out = await invoke<string>("vx6_node_start");
      setOutput(out);
      setStatus("node command issued");
    } catch (err) {
      setStatus("start failed");
      setOutput(String(err));
    }
  }

  return (
    <main className="app">
      <header className="top">
        <h1>VX6 MeshChat</h1>
        <p>Desktop shell over VX6 protocol runtime</p>
      </header>

      <section className="card">
        <label htmlFor="name">Node name</label>
        <input
          id="name"
          value={nodeName}
          onChange={(e) => setNodeName(e.target.value)}
          placeholder="node name"
        />
        <div className="row">
          <button onClick={initNode}>Init</button>
          <button onClick={runStatus}>Status</button>
          <button onClick={startNode}>Start Node</button>
        </div>
      </section>

      <section className="card">
        <h2>Runtime</h2>
        <p className="status">{status}</p>
        <pre>{output || "No output yet"}</pre>
      </section>
    </main>
  );
}

