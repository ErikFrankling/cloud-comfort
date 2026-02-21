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

function ChatSegment({ segment }: { segment: MsgSegment }) {
  switch (segment.type) {
    case "text":
      return <Markdown remarkPlugins={[remarkGfm]}>{segment.content}</Markdown>;
    case "reasoning":
      return (
        <details className="reasoning-block">
          <summary>Reasoning</summary>
          <div className="reasoning-content">{segment.content}</div>
        </details>
      );
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
  const [activeTab, setActiveTab] = useState<"graph" | "files" | "deploy">(
    "deploy",
  );
  const [files, setFiles] = useState<FileEntry[]>([]);
  const [selectedFile, setSelectedFile] = useState<SelectedFile>(null);
  const [fileChanged, setFileChanged] = useState(false);
  const [deployStatus, setDeployStatus] = useState<DeployStatus>("idle");
  const [deployOutput, setDeployOutput] = useState<string[]>([]);
  const [deployError, setDeployError] = useState<string | null>(null);
  const [planReady, setPlanReady] = useState(false);
  const uploadRef = useRef<HTMLInputElement>(null);
  const chatEndRef = useRef<HTMLDivElement>(null);

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
    if (activeTab === "files") {
      fetchFiles();
    }
  }, [activeTab, fetchFiles]);

  const sendMessage = async (overrideMsg?: string) => {
    const userMsg = overrideMsg || message;
    if (!userMsg.trim() || sending) return;
    if (!overrideMsg) setMessage("");
    setSending(true);

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
        setSending(false);
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
    } catch {
      setChatLog((prev) => {
        const updated = [...prev];
        updated[assistantIdx] = {
          role: "error",
          segments: [{ type: "text", content: "Failed to reach backend" }],
        };
        return updated;
      });
    }

    setSending(false);
  };

  const viewFile = async (name: string) => {
    try {
      const res = await fetch(
        `/api/terraform/files/${encodeURIComponent(name)}`,
      );
      const content = await res.text();
      setSelectedFile({ name, content });
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
        <div className="chat-panel">
          <div className="chat-log">
            {chatLog.map((msg, i) => (
              <div key={i} className={`chat-msg ${msg.role}`}>
                <div className="chat-msg-role">{msg.role}</div>
                <div className="chat-msg-body">
                  {msg.segments.map((seg, j) => (
                    <ChatSegment key={j} segment={seg} />
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
            <button onClick={() => sendMessage()} disabled={sending}>
              {sending ? "..." : "Send"}
            </button>
          </div>
        </div>

        <div className="right-panel">
          <div className="tab-bar">
            <button
              className={activeTab === "graph" ? "active" : ""}
              onClick={() => setActiveTab("graph")}
            >
              Flow Chart
            </button>
            <button
              className={activeTab === "files" ? "active" : ""}
              onClick={() => setActiveTab("files")}
            >
              Files
            </button>
            <button
              className={activeTab === "deploy" ? "active" : ""}
              onClick={() => setActiveTab("deploy")}
            >
              Deploy
            </button>
          </div>

          <div className="tab-content">
            {activeTab === "graph" && (
              <>
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
              </>
            )}

            {activeTab === "files" && (
              <div className="files-panel">
                <div className="file-actions">
                  <button onClick={fetchFiles}>Refresh</button>
                  <button onClick={() => uploadRef.current?.click()}>
                    Upload
                  </button>
                  <input
                    ref={uploadRef}
                    type="file"
                    accept=".tf,.tfvars"
                    onChange={uploadFile}
                    hidden
                  />
                  {selectedFile && (
                    <button onClick={downloadFile}>Download</button>
                  )}
                </div>

                <div className="files-layout">
                  <div className="files-list">
                    {files.length === 0 && (
                      <p className="empty-state">
                        No .tf files yet. Upload one or ask the AI to create
                        one.
                      </p>
                    )}
                    {files.map((f) => (
                      <div
                        key={f.name}
                        className={`file-item ${selectedFile?.name === f.name ? "active" : ""}`}
                        onClick={() => viewFile(f.name)}
                      >
                        <span className="file-name">{f.name}</span>
                        <span className="file-size">{f.size} B</span>
                      </div>
                    ))}
                  </div>

                  <div className="file-content">
                    {selectedFile ? (
                      <>
                        <div className="file-content-header">
                          {selectedFile.name}
                          {fileChanged && <span className="unsaved">*</span>}
                        </div>
                        <Editor
                          height="100%"
                          language="hcl"
                          theme="vs-dark"
                          value={selectedFile.content}
                          onChange={handleEditorChange}
                          options={{
                            minimap: { enabled: false },
                            fontSize: 13,
                            lineNumbers: "on",
                            scrollBeyondLastLine: false,
                            automaticLayout: true,
                          }}
                        />
                        <div className="file-content-actions">
                          {fileChanged && (
                            <button onClick={saveFile} className="save-btn">
                              Save
                            </button>
                          )}
                        </div>
                      </>
                    ) : (
                      <p className="empty-state">
                        Select a file to view its contents.
                      </p>
                    )}
                  </div>
                </div>
              </div>
            )}

            {activeTab === "deploy" && (
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
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

export default App;
