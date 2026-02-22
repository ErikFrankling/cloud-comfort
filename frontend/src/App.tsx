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
  const [activeTab, setActiveTab] = useState<"graph" | "file">("graph");
  const [logsOpen, setLogsOpen] = useState(false);
  const [files, setFiles] = useState<FileEntry[]>([]);
  const [selectedFile, setSelectedFile] = useState<SelectedFile>(null);
  const [fileChanged, setFileChanged] = useState(false);
  const [deployStatus, setDeployStatus] = useState<DeployStatus>("idle");
  const [deployOutput, setDeployOutput] = useState<string[]>([]);
  const [deployError, setDeployError] = useState<string | null>(null);
  const [failedStep, setFailedStep] = useState<"init" | "apply" | null>(null);
  const [planReady, setPlanReady] = useState(false);
  const [autoDeploy, setAutoDeploy] = useState(false);
  const [deployOutputs, setDeployOutputs] = useState<Record<string, string> | null>(null);
  const [filesSidebarOpen, setFilesSidebarOpen] = useState(true);
  const uploadRef = useRef<HTMLInputElement>(null);
  const chatEndRef = useRef<HTMLDivElement>(null);
  const abortRef = useRef<AbortController | null>(null);
  const autoDeployRef = useRef(false);
  const handleDeployRef = useRef<() => void>(() => {});
  const [chatWidth, setChatWidth] = useState(400);
  const dragging = useRef(false);

  // GitHub repo selection state
  const [repoUrl, setRepoUrl] = useState("");
  const [repoContext, setRepoContext] = useState<any>(null);
  const [verifyingRepo, setVerifyingRepo] = useState(false);
  const [repoError, setRepoError] = useState<string | null>(null);

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

  const logsEndRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (deployStatus === "init" || deployStatus === "apply") setLogsOpen(true);
    if (deployStatus === "success") setLogsOpen(false);
    if (deployStatus === "error") setLogsOpen(true);
  }, [deployStatus]);

  useEffect(() => {
    logsEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [deployOutput]);

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
        body: JSON.stringify({ message: userMsg, history, repo_context: repoContext }),
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
              generateDiagram();
              if (autoDeployRef.current) {
                handleDeployRef.current();
              }
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
                if (data.outputs) {
                  setDeployOutputs(data.outputs);
                }
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
      setFailedStep(endpoint as "init" | "apply");
      return Promise.reject(err);
    }
  };

  const handleDeploy = async () => {
    setDeployOutput([]);
    setDeployError(null);
    setFailedStep(null);
    setPlanReady(false);
    setDeployOutputs(null);

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

  handleDeployRef.current = handleDeploy;

  const handleReset = () => {
    setDeployStatus("idle");
    setDeployOutput([]);
    setDeployError(null);
    setLogsOpen(false);
  };

  const deleteFile = async (name: string, e: React.MouseEvent) => {
    e.stopPropagation();
    await fetch(`/api/terraform/files/${encodeURIComponent(name)}`, { method: "DELETE" });
    if (selectedFile?.name === name) {
      setSelectedFile(null);
    }
    fetchFiles();
  };

  const formatSize = (b: number) => b < 1024 ? `${b}B` : `${(b / 1024).toFixed(1)}K`;

  // Extract owner/repo from GitHub URL
  const extractRepoFromUrl = (url: string): string | null => {
    const match = url.match(/github\.com\/([^\/]+\/[^\/]+)/);
    return match ? match[1].replace(/\.git$/, '') : null;
  };

  // Verify GitHub repo
  const verifyRepo = async () => {
    const repo = extractRepoFromUrl(repoUrl);
    if (!repo) {
      setRepoError("Invalid GitHub URL. Format: https://github.com/owner/repo");
      return;
    }

    setVerifyingRepo(true);
    setRepoError(null);

    try {
      const res = await fetch("/api/github/explore", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ repo, branch: "main" }),
      });

      if (!res.ok) {
        throw new Error(await res.text());
      }

      const data = await res.json();
      
      if (!data.valid) {
        setRepoError(data.error || "Failed to access repository");
        setVerifyingRepo(false);
        return;
      }

      setRepoContext(data);
      setVerifyingRepo(false);
    } catch (err: any) {
      setRepoError(err.message || "Failed to verify repository");
      setVerifyingRepo(false);
    }
  };

  // Reset repo selection
  const resetRepo = () => {
    setRepoUrl("");
    setRepoContext(null);
    setRepoError(null);
    setChatLog([]);
    setHistory([]);
  };

  return (
    <div className="app">
      <header>
        <div className="header-brand">
          <span className="header-icon">☁</span>
          <h1 className="header-title">Cloud Comfort</h1>
          <span className="header-badge">AI · Terraform</span>
        </div>
      </header>
      <div className="panels">
        <div className="chat-panel" style={{ width: chatWidth }}>
          {/* GitHub Repo Input Overlay */}
          {!repoContext && (
            <div className="repo-input-overlay">
              <div className="repo-input-container">
                <h3>🔗 Connect GitHub Repository</h3>
                <p>Paste a GitHub repository URL to get started</p>
                <div className="repo-input-wrapper">
                  <input
                    type="text"
                    value={repoUrl}
                    onChange={(e) => setRepoUrl(e.target.value)}
                    onKeyDown={(e) => e.key === "Enter" && verifyRepo()}
                    placeholder="https://github.com/owner/repo"
                    disabled={verifyingRepo}
                    className="repo-input"
                  />
                  <button 
                    onClick={verifyRepo} 
                    disabled={verifyingRepo || !repoUrl.trim()}
                    className="repo-verify-btn"
                  >
                    {verifyingRepo ? "Verifying..." : "Connect"}
                  </button>
                </div>
                {repoError && (
                  <div className="repo-error">{repoError}</div>
                )}
              </div>
            </div>
          )}

          {/* Active Repo Header */}
          {repoContext && (
            <div className="repo-header">
              <div className="repo-info">
                <span className="repo-icon">📁</span>
                <span className="repo-name">{repoContext.repo}</span>
                <span className="repo-meta">{repoContext.metadata?.language} • {repoContext.file_tree?.length} files</span>
              </div>
              <button onClick={resetRepo} className="repo-reset-btn" title="Switch repository">
                ✕
              </button>
            </div>
          )}

          <div className={`chat-log ${!repoContext ? 'disabled' : ''}`}>
            {chatLog.map((msg, i) => (
              <div key={i} className={`chat-msg ${msg.role}`}>
                <div className="chat-msg-avatar">
                  {msg.role === "user" ? "You" : msg.role === "assistant" ? "AI" : "!"}
                </div>
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
          <div className={`chat-input ${!repoContext ? 'disabled' : ''}`}>
            <div className="chat-input-wrapper">
              <input
                value={message}
                onChange={(e) => setMessage(e.target.value)}
                onKeyDown={(e) => e.key === "Enter" && sendMessage()}
                placeholder={repoContext ? "Describe your infrastructure..." : "Connect a repository to start chatting..."}
                disabled={sending || !repoContext}
              />
              {sending ? (
                <button className="chat-stop-btn" onClick={stopGeneration} title="Stop">
                  <span>■</span>
                </button>
              ) : (
                <button className="chat-send-btn" onClick={() => sendMessage()} title="Send (Enter)">
                  <span>↑</span>
                </button>
              )}
            </div>
          </div>
        </div>

        <div className="resize-handle" onMouseDown={onResizeStart} />

        <div className="middle-panel">
          <div className="pipeline-bar">
            <div className="deploy-pipeline">
              <div className={`pipeline-step ${
                deployStatus === "init" ? "active" :
                (deployStatus === "apply" || deployStatus === "success") ? "done" :
                (deployStatus === "error" && failedStep === "init") ? "error" : ""
              }`}>
                <div className="pipeline-dot" />
                <span className="pipeline-label">Init</span>
              </div>
              <div className={`pipeline-connector ${
                (deployStatus === "apply" || deployStatus === "success") ? "done" : ""
              }`} />
              <div className={`pipeline-step ${
                deployStatus === "apply" ? "active" :
                deployStatus === "success" ? "done" :
                (deployStatus === "error" && failedStep === "apply") ? "error" : ""
              }`}>
                <div className="pipeline-dot" />
                <span className="pipeline-label">Apply</span>
              </div>
              <div className={`pipeline-connector ${deployStatus === "success" ? "done" : ""}`} />
              <div className={`pipeline-step ${deployStatus === "success" ? "done" : ""}`}>
                <div className="pipeline-dot" />
                <span className="pipeline-label">Done</span>
              </div>
            </div>
            <div className="pipeline-bar-actions">
              <button className="show-logs-btn" onClick={() => setLogsOpen((o) => !o)}>
                {logsOpen ? "Hide Logs" : "Deploy Logs"}
              </button>
              <label className="auto-deploy-toggle">
                <input
                  type="checkbox"
                  checked={autoDeploy}
                  onChange={(e) => {
                    setAutoDeploy(e.target.checked);
                    autoDeployRef.current = e.target.checked;
                  }}
                />
                Auto
              </label>
              {deployStatus === "idle" && (
                <button className="deploy-btn" onClick={handleDeploy}>Deploy</button>
              )}
              {deployStatus === "success" && (
                <button className="deploy-btn" onClick={handleReset}>Reset</button>
              )}
              {(deployStatus === "error" || deployStatus === "success") && (
                <button className="deploy-btn" onClick={handleDeploy}>Deploy Again</button>
              )}
              {(deployStatus === "init" || deployStatus === "apply") && (
                <button className="deploy-btn" disabled>Running...</button>
              )}
            </div>
          </div>

          <div className="tab-bar">
            <button
              className={activeTab === "graph" ? "active" : ""}
              onClick={() => setActiveTab("graph")}
            >
              Flow Chart
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
                  <p>Click Generate Diagram to visualize your infrastructure.</p>
                </div>
              )}
              {diagramNodes && (
                <div className="diagram-output">
                  <InfraFlow nodes={diagramNodes} edges={diagramEdges} />
                </div>
              )}
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

        <div className="files-sidebar">
          <div className="files-sidebar-header">
            <span>Files</span>
            <button
              className="sidebar-upload-btn"
              onClick={() => uploadRef.current?.click()}
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
              <div className="empty-state-files">
                <div className="empty-state-icon">{"{}"}</div>
                <p>No .tf files yet</p>
                <p className="empty-state-hint">Chat with the AI to generate your first file</p>
              </div>
            )}
            {files.map((f, i) => (
              <div
                key={f.name}
                className={`file-item ${selectedFile?.name === f.name ? "active" : ""}`}
                onClick={() => viewFile(f.name)}
                style={{ animationDelay: `${i * 50}ms` }}
              >
                <div className="file-icon">{f.name.endsWith(".tfvars") ? "⚙" : "{}"}</div>
                <span className="file-name">{f.name}</span>
                <span className="file-size">{formatSize(f.size)}</span>
                <button
                  className="file-delete-btn"
                  onClick={(e) => deleteFile(f.name, e)}
                  title={`Delete ${f.name}`}
                >✕</button>
              </div>
            ))}
          </div>


        </div>
      </div>
      {logsOpen && (
        <div className="logs-modal-backdrop" onClick={() => setLogsOpen(false)}>
          <div className="logs-modal" onClick={(e) => e.stopPropagation()}>
            <div className="logs-modal-header">
              <span className="logs-modal-title">Deploy Logs</span>
              <button className="logs-modal-close" onClick={() => setLogsOpen(false)}>✕</button>
            </div>
            {deployError && (
              <div className="logs-modal-error">
                <strong>Error:</strong> {deployError}
              </div>
            )}
            <div className="logs-modal-output">
              {deployOutput.length === 0 ? (
                <p className="empty-state">
                  {planReady
                    ? "Plan ready. Click Deploy to apply changes."
                    : "No logs yet — click Deploy to start."}
                </p>
              ) : (
                deployOutput.map((line, i) => (
                  <div key={i} className={`output-line ${line.includes("[ERROR]") ? "error" : ""}`}>
                    {line}
                  </div>
                ))
              )}
              {deployOutputs && Object.keys(deployOutputs).length > 0 && (
                <div className="deploy-outputs">
                  <div className="deploy-outputs-title">Outputs</div>
                  {Object.entries(deployOutputs).map(([key, value]) => (
                    <div key={key} className="deploy-output-item">
                      <span className="deploy-output-key">{key}</span>
                      <span className="deploy-output-value">{value}</span>
                    </div>
                  ))}
                </div>
              )}
              <div ref={logsEndRef} />
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export default App;
