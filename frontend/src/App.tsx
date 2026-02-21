import { useCallback, useEffect, useRef, useState } from 'react'
import mermaid from 'mermaid'

mermaid.initialize({ startOnLoad: false, theme: 'dark' })

type FileEntry = { name: string; size: number }
type SelectedFile = { name: string; content: string } | null

function App() {
  const [message, setMessage] = useState('')
  const [chatLog, setChatLog] = useState<{ role: string; text: string }[]>([])
  const [activeTab, setActiveTab] = useState<'graph' | 'files'>('graph')
  const [files, setFiles] = useState<FileEntry[]>([])
  const [selectedFile, setSelectedFile] = useState<SelectedFile>(null)
  const uploadRef = useRef<HTMLInputElement>(null)

  const [diagramLoading, setDiagramLoading] = useState(false)
  const [diagramError, setDiagramError] = useState<string | null>(null)
  const [mermaidCode, setMermaidCode] = useState<string | null>(null)
  const diagramRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!mermaidCode || !diagramRef.current) return
    mermaid.render('diagram', mermaidCode).then(({ svg }) => {
      diagramRef.current!.innerHTML = svg
    }).catch(() => {
      diagramRef.current!.innerHTML = '<p style="color:#ef9a9a;padding:1rem">Failed to render diagram.</p>'
    })
  }, [mermaidCode])

  const generateDiagram = async () => {
    setDiagramLoading(true)
    setDiagramError(null)
    setMermaidCode(null)
    try {
      const res = await fetch('/api/diagram', { method: 'POST' })
      if (!res.ok) throw new Error(await res.text())
      const data = await res.json()
      setMermaidCode(data.mermaid)
    } catch (e) {
      setDiagramError(e instanceof Error ? e.message : 'Unknown error')
    } finally {
      setDiagramLoading(false)
    }
  }

  const fetchFiles = useCallback(async () => {
    try {
      const res = await fetch('/api/terraform/files')
      const data = await res.json()
      setFiles(data)
    } catch {
      setFiles([])
    }
  }, [])

  useEffect(() => {
    if (activeTab === 'files') {
      fetchFiles()
    }
  }, [activeTab, fetchFiles])

  const sendMessage = async () => {
    if (!message.trim()) return

    setChatLog((prev) => [...prev, { role: 'user', text: message }])
    const userMsg = message
    setMessage('')

    try {
      const res = await fetch('/api/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: userMsg }),
      })
      const data = await res.json()
      setChatLog((prev) => [...prev, { role: 'assistant', text: data.reply }])
    } catch {
      setChatLog((prev) => [...prev, { role: 'error', text: 'Failed to reach backend' }])
    }
  }

  const viewFile = async (name: string) => {
    try {
      const res = await fetch(`/api/terraform/files/${encodeURIComponent(name)}`)
      const content = await res.text()
      setSelectedFile({ name, content })
    } catch {
      setSelectedFile({ name, content: 'Failed to load file.' })
    }
  }

  const downloadFile = () => {
    if (!selectedFile) return
    const blob = new Blob([selectedFile.content], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = selectedFile.name
    a.click()
    URL.revokeObjectURL(url)
  }

  const uploadFile = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (!file) return

    const content = await file.text()
    try {
      await fetch(`/api/terraform/files/${encodeURIComponent(file.name)}`, {
        method: 'PUT',
        body: content,
      })
      fetchFiles()
    } catch {
      // silently fail for now
    }

    // reset the input so the same file can be re-uploaded
    if (uploadRef.current) uploadRef.current.value = ''
  }

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
                <strong>{msg.role}:</strong> {msg.text}
              </div>
            ))}
          </div>
          <div className="chat-input">
            <input
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && sendMessage()}
              placeholder="Describe your infrastructure..."
            />
            <button onClick={sendMessage}>Send</button>
          </div>
        </div>

        <div className="right-panel">
          <div className="tab-bar">
            <button
              className={activeTab === 'graph' ? 'active' : ''}
              onClick={() => setActiveTab('graph')}
            >
              Flow Chart
            </button>
            <button
              className={activeTab === 'files' ? 'active' : ''}
              onClick={() => setActiveTab('files')}
            >
              Files
            </button>
          </div>

          <div className="tab-content">
            {activeTab === 'graph' && (
              <>
                <div className="file-actions">
                  <button onClick={generateDiagram} disabled={diagramLoading}>
                    {diagramLoading ? 'Generating...' : 'Generate Diagram'}
                  </button>
                  {diagramError && <span style={{ color: '#ef9a9a', fontSize: '0.8rem' }}>{diagramError}</span>}
                </div>
                {!mermaidCode && !diagramLoading && (
                  <div className="graph-placeholder">
                    <p>Click Generate Diagram to visualize your infrastructure.</p>
                  </div>
                )}
                <div ref={diagramRef} className="diagram-output" />
              </>
            )}

            {activeTab === 'files' && (
              <div className="files-panel">
                <div className="file-actions">
                  <button onClick={fetchFiles}>Refresh</button>
                  <button onClick={() => uploadRef.current?.click()}>Upload</button>
                  <input
                    ref={uploadRef}
                    type="file"
                    accept=".tf,.tfvars"
                    onChange={uploadFile}
                    hidden
                  />
                  {selectedFile && <button onClick={downloadFile}>Download</button>}
                </div>

                <div className="files-layout">
                  <div className="files-list">
                    {files.length === 0 && (
                      <p className="empty-state">No .tf files yet. Upload one to get started.</p>
                    )}
                    {files.map((f) => (
                      <div
                        key={f.name}
                        className={`file-item ${selectedFile?.name === f.name ? 'active' : ''}`}
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
                        <div className="file-content-header">{selectedFile.name}</div>
                        <pre>{selectedFile.content}</pre>
                      </>
                    ) : (
                      <p className="empty-state">Select a file to view its contents.</p>
                    )}
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

export default App
