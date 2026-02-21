import { useCallback, useEffect, useRef, useState } from 'react'

type FileEntry = { name: string; size: number }
type SelectedFile = { name: string; content: string } | null
type ChatMsg = { role: string; text: string }
type LLMHistory = { role: string; content: string }[]

function App() {
  const [message, setMessage] = useState('')
  const [chatLog, setChatLog] = useState<ChatMsg[]>([])
  const [history, setHistory] = useState<LLMHistory>([])
  const [sending, setSending] = useState(false)
  const [activeTab, setActiveTab] = useState<'graph' | 'files'>('graph')
  const [files, setFiles] = useState<FileEntry[]>([])
  const [selectedFile, setSelectedFile] = useState<SelectedFile>(null)
  const uploadRef = useRef<HTMLInputElement>(null)
  const chatEndRef = useRef<HTMLDivElement>(null)

  const scrollToBottom = () => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }

  useEffect(scrollToBottom, [chatLog])

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
    if (!message.trim() || sending) return

    const userMsg = message
    setMessage('')
    setSending(true)
    setChatLog((prev) => [...prev, { role: 'user', text: userMsg }])

    // Add placeholder for assistant response
    const assistantIdx = chatLog.length + 1
    setChatLog((prev) => [...prev, { role: 'assistant', text: '' }])

    try {
      const res = await fetch('/api/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: userMsg, history }),
      })

      if (!res.ok) {
        const errText = await res.text()
        setChatLog((prev) => {
          const updated = [...prev]
          updated[assistantIdx] = { role: 'error', text: errText }
          return updated
        })
        setSending(false)
        return
      }

      const reader = res.body?.getReader()
      const decoder = new TextDecoder()
      let accumulated = ''
      let buffer = ''

      if (!reader) throw new Error('No reader')

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })

        // Parse SSE events from buffer
        const lines = buffer.split('\n')
        buffer = lines.pop() || '' // keep incomplete line in buffer

        for (const line of lines) {
          if (!line.startsWith('data: ')) continue
          const jsonStr = line.slice(6)

          try {
            const event = JSON.parse(jsonStr)

            if (event.content) {
              accumulated += event.content
              setChatLog((prev) => {
                const updated = [...prev]
                updated[assistantIdx] = { role: 'assistant', text: accumulated }
                return updated
              })
            }

            if (event.tool_call) {
              accumulated += `\n[${event.tool_call.result}]\n`
              setChatLog((prev) => {
                const updated = [...prev]
                updated[assistantIdx] = { role: 'assistant', text: accumulated }
                return updated
              })
              // Refresh files since a file was likely written
              fetchFiles()
            }

            if (event.error) {
              setChatLog((prev) => {
                const updated = [...prev]
                updated[assistantIdx] = { role: 'error', text: event.error }
                return updated
              })
            }

            if (event.done) {
              // Update history for future requests
              setHistory((prev) => [
                ...prev,
                { role: 'user', content: userMsg },
                { role: 'assistant', content: accumulated },
              ])
            }
          } catch {
            // skip unparseable lines
          }
        }
      }
    } catch {
      setChatLog((prev) => {
        const updated = [...prev]
        updated[assistantIdx] = { role: 'error', text: 'Failed to reach backend' }
        return updated
      })
    }

    setSending(false)
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
            <div ref={chatEndRef} />
          </div>
          <div className="chat-input">
            <input
              value={message}
              onChange={(e) => setMessage(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && sendMessage()}
              placeholder="Describe your infrastructure..."
              disabled={sending}
            />
            <button onClick={sendMessage} disabled={sending}>
              {sending ? '...' : 'Send'}
            </button>
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
              <div className="graph-placeholder">
                <p>Terraform plan visualization will appear here.</p>
              </div>
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
                      <p className="empty-state">No .tf files yet. Upload one or ask the AI to create one.</p>
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
