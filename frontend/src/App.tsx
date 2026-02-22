import { useCallback, useEffect, useRef, useState } from "react";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";
import mermaid from "mermaid";

import Editor from "@monaco-editor/react";
import InfraFlow, { ApiNode, ApiEdge } from "./InfraFlow";

mermaid.initialize({ startOnLoad: false, theme: "dark" });

type FileEntry = { name: string; size: number };
type SelectedFile = { name: string; content: string } | null;
type DeployStatus = "idle" | "init" | "apply" | "success" | "error";

type ToolCallInfo = { name: string; filename: string; result: string };

type MsgSegment =
  | { type: "text"; content: string }
  | { type: "reasoning"; content: string }
  | { type: "tool_call"; info: ToolCallInfo }
  | { type: "phase"; label: string; error?: string }
  | { type: "plan_ready"; hasChanges: boolean }
  | { type: "validation"; content: string };

type ChatMsg = { role: "user" | "assistant" | "error"; segments: MsgSegment[] };
type LLMHistory = { role: string; content: string }[];

function ReasoningBlock({ content, isStreaming }: { content: string; isStreaming: boolean }) {
  const [open, setOpen] = useState(false);
  const wasStreaming = useRef(false);

  useEffect(() => {
    if (isStreaming) {
      setOpen(true);
      wasStreaming.current = true;
    } else if (wasStreaming.current) {
      setOpen(false);
      wasStreaming.current = false;
    }
  }, [isStreaming]);

  return (
    <div className={`reasoning-block ${open ? "open" : ""}`}>
      <div className="reasoning-summary" onClick={() => setOpen((o) => !o)}>
        <span className="reasoning-arrow" />
        Reasoning
        {isStreaming && <span className="reasoning-live" />}
      </div>
      {open && <div className="reasoning-content">{content}</div>}
    </div>
  );
}

function ChatSegment({ segment, isStreaming }: { segment: MsgSegment; isStreaming: boolean }) {
  switch (segment.type) {
    case "text":
      return <Markdown remarkPlugins={[remarkGfm]}>{segment.content}</Markdown>;
    case "reasoning":
      return <ReasoningBlock content={segment.content} isStreaming={isStreaming} />;
    case "tool_call":
      return (
        <div className="tool-call-card">
          <span className="tool-call-filename">{segment.info.filename}</span>
          <span className="tool-call-result">{segment.info.result}</span>
        </div>
      );
    case "phase":
      return (
        <div className={`phase-indicator ${segment.error ? "error" : ""}`}>
          {segment.error
            ? `${segment.label} failed: ${segment.error}`
            : segment.label === "init"
              ? "Running terraform init..."
              : "Running terraform plan..."}
        </div>
      );
    case "plan_ready":
      return (
        <div className="plan-ready-indicator">
          Plan succeeded
          {segment.hasChanges ? " — changes detected" : " — no changes"}
        </div>
      );
    case "validation":
      return (
        <div className="validation-block">
          <pre>{segment.content}</pre>
        </div>
      );
  }
}

function App() {
  const [message, setMessage] = useState("");
  const [chatLog, setChatLog] = useState<ChatMsg[]>([]);
  const [history, setHistory] = useState<LLMHistory>([]);
  const [sending, setSending] = useState(false);
  const [activeTab, setActiveTab] = useState<"graph" | "deploy" | "file">("deploy");
  const [files, setFiles] = useState<FileEntry[]>([]);
  const [selectedFile, setSelectedFile] = useState<SelectedFile>(null);
  const [fileChanged, setFileChanged] = useState(false);
  const [deployStatus, setDeployStatus] = useState<DeployStatus>("idle");
  const [deployOutput, setDeployOutput] = useState<string[]>([]);
  const [deployError, setDeployError] = useState<string | null>(null);
  const [planReady, setPlanReady] = useState(false);
  const [filesSidebarOpen, setFilesSidebarOpen] = useState(true);
  const uploadRef = useRef<HTMLInputElement>(null);
  const chatEndRef = useRef<HTMLDivElement>(null);
  const abortRef = useRef<AbortController | null>(null);
  const [chatWidth, setChatWidth] = useState(400);
  const dragging = useRef(false);

  useEffect(() => {
    const onMouseMove = (e: MouseEvent) => {
      if (!dragging.current) return;
      const clamped = Math.min(Math.max(e.clientX, 250), window.innerWidth - 300);
      setChatWidth(clamped);
    };
    const onMouseUp = () => {
      if (dragging.current) {
        dragging.current = false;
        document.body.style.cursor = "";
        document.body.style.userSelect = "";
      }
    };
    window.addEventListener("mousemove", onMouseMove);
    window.addEventListener("mouseup", onMouseUp);
    return () => {
      window.removeEventListener("mousemove", onMouseMove);
      window.removeEventListener("mouseup", onMouseUp);
    };
  }, []);

  const onResizeStart = (e: React.MouseEvent) => {
    e.preventDefault();
    dragging.current = true;
    document.body.style.cursor = "col-resize";
    document.body.style.userSelect = "none";
  };

  const scrollToBottom = () => {
    chatEndRef.current?.scrollIntoView({ behavior: "smooth" });
  };

  useEffect(scrollToBottom, [chatLog]);

  const [diagramLoading, setDiagramLoading] = useState(false);
  const [diagramError, setDiagramError] = useState<string | null>(null);
  const [diagramNodes, setDiagramNodes] = useState<ApiNode[] | null>(null);
  const [diagramEdges, setDiagramEdges] = useState<ApiEdge[]>([]);

  const generateDiagram = async () => {
    setDiagramLoading(true);
    setDiagramError(null);
    setDiagramNodes(null);
    try {
      const res = await fetch("/api/diagram", { method: "POST" });
      if (!res.ok) throw new Error(await res.text());
      const data = await res.json();
      setDiagramNodes(data.nodes ?? []);
      setDiagramEdges(data.edges ?? []);
    } catch (e) {
      setDiagramError(e instanceof Error ? e.message : "Unknown error");
    } finally {
      setDiagramLoading(false);
    }
  };

  const fetchFiles = useCallback(async () => {
    try {
      const res = await fetch("/api/terraform/files");
      const data = await res.json();
      setFiles(data);
    } catch {
      setFiles([]);
    }
  }, []);

  useEffect(() => {
    fetchFiles();
  }, [fetchFiles]);

  const sendMessage = async (overrideMsg?: string) => {
    const userMsg = overrideMsg || message;
    if (!userMsg.trim() || sending) return;
    if (!overrideMsg) setMessage("");
    setSending(true);

    const controller = new AbortController();
    abortRef.current = controller;

    setChatLog((prev) => [
      ...prev,
      { role: "user", segments: [{ type: "text", content: userMsg }] },
    ]);

    // Add placeholder for assistant response
    const assistantIdx = chatLog.length + 1;
    setChatLog((prev) => [...prev, { role: "assistant", segments: [] }]);

    // Helper to update the assistant message's segments
    const updateSegments = (updater: (segs: MsgSegment[]) => MsgSegment[]) => {
      setChatLog((prev) => {
        const updated = [...prev];
        const msg = updated[assistantIdx];
        if (!msg) return prev;
        updated[assistantIdx] = {
          ...msg,
          segments: updater([...msg.segments]),
        };
        return updated;
      });
    };

    let contentAccumulated = "";

    try {
      const res = await fetch("/api/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ message: userMsg, history }),
        signal: controller.signal,
      });

      if (!res.ok) {
        const errText = await res.text();
        setChatLog((prev) => {
          const updated = [...prev];
          updated[assistantIdx] = {
            role: "error",
            segments: [{ type: "text", content: errText }],
          };
          return updated;
        });
        return;
      }

      const reader = res.body?.getReader();
      const decoder = new TextDecoder();
      let buffer = "";

      if (!reader) throw new Error("No reader");

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });

        // Parse SSE events from buffer
        const lines = buffer.split("\n");
        buffer = lines.pop() || ""; // keep incomplete line in buffer

        for (const line of lines) {
          if (!line.startsWith("data: ")) continue;
          const jsonStr = line.slice(6);

          try {
            const event = JSON.parse(jsonStr);

            if (event.reasoning) {
              updateSegments((segs) => {
                const last = segs[segs.length - 1];
                if (last && last.type === "reasoning") {
                  return [
                    ...segs.slice(0, -1),
                    { ...last, content: last.content + event.reasoning },
                  ];
                }
                return [
                  ...segs,
                  { type: "reasoning", content: event.reasoning },
                ];
              });
            }

            if (event.content) {
              contentAccumulated += event.content;
              updateSegments((segs) => {
                const last = segs[segs.length - 1];
                if (last && last.type === "text") {
                  return [
                    ...segs.slice(0, -1),
                    { ...last, content: last.content + event.content },
                  ];
                }
                return [...segs, { type: "text", content: event.content }];
              });
            }

            if (event.tool_call) {
              updateSegments((segs) => [
                ...segs,
                {
                  type: "tool_call",
                  info: {
                    name: event.tool_call.name,
                    filename: event.tool_call.filename || "",
                    result: event.tool_call.result,
                  },
                },
              ]);
              fetchFiles();
            }

            if (event.validation) {
              updateSegments((segs) => [
                ...segs,
                { type: "validation", content: event.validation },
              ]);
            }

            if (event.phase) {
              updateSegments((segs) => [
                ...segs,
                { type: "phase", label: event.phase },
              ]);
            }

            if (event.phase_error) {
              updateSegments((segs) => [
                ...segs,
                { type: "phase", label: event.phase_error, error: event.error },
              ]);
            }

            if (event.plan_ready) {
              setPlanReady(true);
              updateSegments((segs) => [
                ...segs,
                { type: "plan_ready", hasChanges: !!event.has_changes },
              ]);
              fetchFiles();
            }

            if (event.error) {
              setChatLog((prev) => {
                const updated = [...prev];
                updated[assistantIdx] = {
                  role: "error",
                  segments: [{ type: "text", content: event.error }],
                };
                return updated;
              });
            }

            if (event.done) {
              setHistory((prev) => [
                ...prev,
                { role: "user", content: userMsg },
                { role: "assistant", content: contentAccumulated },
              ]);
            }
          } catch {
            // skip unparseable lines
          }
        }
      }
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") {
        // User clicked Stop — save partial content so the LLM has context
        if (contentAccumulated) {
          setHistory((prev) => [
            ...prev,
            { role: "user", content: userMsg },
            { role: "assistant", content: contentAccumulated + "\n[stopped by user]" },
          ]);
        }
      } else {
        setChatLog((prev) => {
          const updated = [...prev];
          updated[assistantIdx] = {
            role: "error",
            segments: [{ type: "text", content: "Failed to reach backend" }],
          };
          return updated;
        });
      }
    } finally {
      abortRef.current = null;
      setSending(false);
    }
  };

  const stopGeneration = () => {
    if (abortRef.current) {
      abortRef.current.abort();
      abortRef.current = null;
    }
  };

  const viewFile = async (name: string) => {
    try {
      const res = await fetch(
        `/api/terraform/files/${encodeURIComponent(name)}`,
      );
      const content = await res.text();
      setSelectedFile({ name, content });
      setActiveTab("file");
    } catch {
      setSelectedFile({ name, content: "Failed to load file." });
    }
  };

  const downloadFile = () => {
    if (!selectedFile) return;
    const blob = new Blob([selectedFile.content], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = selectedFile.name;
    a.click();
    URL.revokeObjectURL(url);
  };

  const handleEditorChange = (value: string | undefined) => {
    if (selectedFile && value !== undefined) {
      setSelectedFile({ ...selectedFile, content: value });
      setFileChanged(true);
    }
  };

  const saveFile = async () => {
    if (!selectedFile) return;
    try {
      await fetch(
        `/api/terraform/files/${encodeURIComponent(selectedFile.name)}`,
        {
          method: "PUT",
          body: selectedFile.content,
        },
      );
      setFileChanged(false);
      fetchFiles();
    } catch {
      // silently fail for now
    }
  };

  const uploadFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    const content = await file.text();
    try {
      await fetch(`/api/terraform/files/${encodeURIComponent(file.name)}`, {
        method: "PUT",
        body: content,
      });
      fetchFiles();
    } catch {
      // silently fail for now
    }

    if (uploadRef.current) uploadRef.current.value = "";
  };

  const runDeployStep = async (
    endpoint: string,
    newStatus: DeployStatus,
  ): Promise<any> => {
    setDeployStatus(newStatus);
    const url = `/api/terraform/${endpoint}`;
    console.log("Fetching:", url);

    try {
      const response = await fetch(url, {
        method: "POST",
      });

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }

      const reader = response.body?.getReader();
      const decoder = new TextDecoder();

      if (!reader) {
        throw new Error("No response body");
      }

      let buffer = "";

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split("\n");
        buffer = lines.pop() || "";

        for (const line of lines) {
          if (line.startsWith("data: ")) {
            try {
              const data = JSON.parse(line.slice(6));
              if (data.line) {
                setDeployOutput((prev) => [...prev, data.line]);
              }
              if (data.error) {
                setDeployError(data.error);
                setDeployOutput((prev) => [...prev, `[ERROR] ${data.error}`]);
                setDeployStatus("error");
                return Promise.reject(new Error(data.error));
              }
              if (data.done) {
                setDeployStatus("success");
                return Promise.resolve(data);
              }
            } catch {
              // ignore parse errors
            }
          }
        }
      }

      setDeployStatus("success");
      return Promise.resolve({});
    } catch (err: any) {
      const errorMsg = err.message || `Failed to connect to ${endpoint}`;
      setDeployError(errorMsg);
      setDeployOutput((prev) => [...prev, `[ERROR] ${errorMsg}`]);
      setDeployStatus("error");
      return Promise.reject(err);
    }
  };

  const handleDeploy = async () => {
    setDeployOutput([]);
    setDeployError(null);
    setPlanReady(false);

    try {
      // Init is safe and idempotent — ensures providers are ready
      await runDeployStep("init", "init");
      // Apply — this is the only step that affects real infrastructure
      await runDeployStep("apply", "apply");
    } catch (err: any) {
      console.error("Deployment failed:", err);
      // Auto-send error to chat so the LLM can fix the terraform files
      const errorMsg = err.message || "Unknown deployment error";
      sendMessage(
        `Terraform apply failed with the following error:\n${errorMsg}\n\nPlease fix the terraform configuration.`,
      );
    }
  };

  const handleReset = () => {
    setDeployStatus("idle");
    setDeployOutput([]);
    setDeployError(null);
  };

  return (
    <div className="app">
      <header>
        <h1>Cloud Comfort</h1>
      </header>
      <div className="panels">
        <div className="chat-panel" style={{ width: chatWidth }}>
          <div className="chat-log">
            {chatLog.map((msg, i) => (
              <div key={i} className={`chat-msg ${msg.role}`}>
                <div className="chat-msg-role">{msg.role}</div>
                <div className="chat-msg-body">
                  {msg.segments.map((seg, j) => (
                    <ChatSegment
                      key={j}
                      segment={seg}
                      isStreaming={
                        sending &&
                        i === chatLog.length - 1 &&
                        msg.role === "assistant" &&
                        j === msg.segments.length - 1 &&
                        seg.type === "reasoning"
                      }
                    />
                  ))}
                  {sending &&
                    i === chatLog.length - 1 &&
                    msg.role === "assistant" && (
                      <span className="chat-spinner" />
                    )}
                </div>
              </div>
            ))}
            <div ref={chatEndRef} />
          </div>
          <div className="chat-input">
            <input
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && sendMessage()}
              placeholder="Describe your infrastructure..."
              disabled={sending}
            />
            {sending ? (
              <button className="stop-btn" onClick={stopGeneration}>Stop</button>
            ) : (
              <button onClick={() => sendMessage()}>Send</button>
            )}
          </div>
        </div>

        <div className="resize-handle" onMouseDown={onResizeStart} />

        <div className="middle-panel">
          <div className="tab-bar">
            <button
              className={activeTab === "graph" ? "active" : ""}
              onClick={() => setActiveTab("graph")}
            >
              Flow Chart
            </button>
            <button
              className={activeTab === "deploy" ? "active" : ""}
              onClick={() => setActiveTab("deploy")}
            >
              Deploy
            </button>
            {selectedFile && (
              <button
                className={activeTab === "file" ? "active" : ""}
                onClick={() => setActiveTab("file")}
              >
                {selectedFile.name}
              </button>
            )}
          </div>

          <div className="tab-content">
            <div style={{ display: activeTab === "graph" ? "flex" : "none", flex: 1, flexDirection: "column", overflow: "hidden" }}>
                <div className="file-actions">
                  <button onClick={generateDiagram} disabled={diagramLoading}>
                    {diagramLoading ? "Generating..." : "Generate Diagram"}
                  </button>
                  {diagramError && (
                    <span style={{ color: "#ef9a9a", fontSize: "0.8rem" }}>
                      {diagramError}
                    </span>
                  )}
                </div>
                {!diagramNodes && !diagramLoading && (
                  <div className="graph-placeholder">
                    <p>
                      Click Generate Diagram to visualize your infrastructure.
                    </p>
                  </div>
                )}
                {diagramNodes && (
                  <div className="diagram-output">
                    <InfraFlow nodes={diagramNodes} edges={diagramEdges} />
                  </div>
                )}
            </div>

            <div style={{ display: activeTab === "deploy" ? "flex" : "none", flex: 1, flexDirection: "column", overflow: "hidden" }}>
              <div className="deploy-panel">
                <div className="deploy-header">
                  <div className={`deploy-status status-${deployStatus}`}>
                    {deployStatus === "idle" &&
                      (planReady
                        ? "Plan ready — click Deploy to apply"
                        : "Ready to deploy")}
                    {deployStatus === "init" && "Running terraform init..."}
                    {deployStatus === "apply" && "Running terraform apply..."}
                    {deployStatus === "success" && "Deployment successful!"}
                    {deployStatus === "error" &&
                      "Deployment failed — sending error to AI..."}
                  </div>
                  <div className="deploy-actions">
                    {deployStatus === "idle" && (
                      <button className="deploy-btn" onClick={handleDeploy}>
                        Deploy
                      </button>
                    )}
                    {deployStatus === "success" && (
                      <button className="deploy-btn" onClick={handleReset}>
                        Reset
                      </button>
                    )}
                    {(deployStatus === "error" ||
                      deployStatus === "success") && (
                      <button className="deploy-btn" onClick={handleDeploy}>
                        Deploy Again
                      </button>
                    )}
                    {(deployStatus === "init" || deployStatus === "apply") && (
                      <button className="deploy-btn" disabled>
                        Running...
                      </button>
                    )}
                  </div>
                </div>

                {deployError && (
                  <div className="deploy-error">
                    <strong>Error:</strong> {deployError}
                  </div>
                )}

                <div className="deploy-output">
                  {deployOutput.length === 0 && deployStatus === "idle" && (
                    <p className="empty-state">
                      {planReady
                        ? "Plan ready. Click Deploy to apply changes."
                        : "Chat with the AI to generate terraform files. Plan runs automatically."}
                    </p>
                  )}
                  {deployOutput.map((line, i) => (
                    <div
                      key={i}
                      className={`output-line ${line.includes("[ERROR]") ? "error" : ""}`}
                    >
                      {line}
                    </div>
                  ))}
                </div>
              </div>
            </div>

            {selectedFile && (
              <div style={{ display: activeTab === "file" ? "flex" : "none", flex: 1, flexDirection: "column", overflow: "hidden" }}>
                <div className="file-editor-panel">
                  <div className="file-editor-header">
                    <span>{selectedFile.name}</span>
                    {fileChanged && <span className="unsaved">*</span>}
                    <button onClick={downloadFile} className="file-action-btn">
                      Download
                    </button>
                    {fileChanged && (
                      <button onClick={saveFile} className="file-action-btn save">
                        Save
                      </button>
                    )}
                  </div>
                  <div className="file-editor-content">
                    <Editor
                      height="100%"
                      language="hcl"
                      theme="vs-dark"
                      value={selectedFile.content}
                      onChange={handleEditorChange}
                      options={{
                        minimap: { enabled: true },
                        fontSize: 14,
                        lineNumbers: "on",
                        scrollBeyondLastLine: false,
                        automaticLayout: true,
                        readOnly: false,
                      }}
                    />
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>

        <button
          className={`sidebar-toggle-btn ${!filesSidebarOpen ? "sidebar-collapsed" : ""}`}
          onClick={() => setFilesSidebarOpen(!filesSidebarOpen)}
          title={filesSidebarOpen ? "Hide files" : "Show files"}
        >
          {filesSidebarOpen ? "▶" : "◀"}
        </button>

        <div className={`files-sidebar ${!filesSidebarOpen ? "collapsed" : ""}`}>
          <div className="files-sidebar-header">
            <span>Files</span>
            <button
              onClick={() => uploadRef.current?.click()}
              style={{
                padding: "0.25rem 0.5rem",
                fontSize: "0.9rem",
                background: "#1a1d27",
                border: "1px solid #2a2d35",
                borderRadius: "4px",
                color: "#e0e0e0",
                cursor: "pointer",
              }}
            >
              + Upload
            </button>
            <input
              ref={uploadRef}
              type="file"
              accept=".tf,.tfvars"
              onChange={uploadFile}
              hidden
            />
          </div>

          <div className="files-list">
            {files.length === 0 && (
              <p className="empty-state">
                No .tf files yet.
              </p>
            )}
            {files.map((f, i) => (
              <div
                key={f.name}
                className={`file-item ${selectedFile?.name === f.name ? "active" : ""}`}
                onClick={() => viewFile(f.name)}
                style={{ animationDelay: `${i * 50}ms` }}
              >
                <span className="file-name">{f.name}</span>
                <span className="file-size">{f.size} B</span>
              </div>
            ))}
          </div>


        </div>
      </div>
    </div>
  );
}

export default App;
