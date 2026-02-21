import { useCallback, useEffect, useRef, useState } from 'react'
import mermaid from 'mermaid'

mermaid.initialize({ startOnLoad: false, theme: 'dark' })

type FileEntry = { name: string; size: number }
type SelectedFile = { name: string; content: string } | null
type DeployStatus = 'idle' | 'init' | 'plan' | 'apply' | 'success' | 'error'

type ChatMsg = { role: string; text: string }
type LLMHistory = { role: string; content: string }[]

function App() {
  const [message, setMessage] = useState('')
  const [chatLog, setChatLog] = useState<{ role: string; text: string }[]>([])
  const [activeTab, setActiveTab] = useState<'graph' | 'files' | 'deploy'>('deploy')
  const [message, setMessage] = useState('')
  const [chatLog, setChatLog] = useState<ChatMsg[]>([])
  const [history, setHistory] = useState<LLMHistory>([])
  const [sending, setSending] = useState(false)
  const [activeTab, setActiveTab] = useState<'graph' | 'files'>('graph')
  const [files, setFiles] = useState<FileEntry[]>([])
  const [selectedFile, setSelectedFile] = useState<SelectedFile>(null)
  const [deployStatus, setDeployStatus] = useState<DeployStatus>('idle')
  const [deployOutput, setDeployOutput] = useState<string[]>([])
  const [deployError, setDeployError] = useState<string | null>(null)
  const [planHasChanges, setPlanHasChanges] = useState<boolean | null>(null)
  const uploadRef = useRef<HTMLInputElement>(null)
  const chatEndRef = useRef<HTMLDivElement>(null)

  const scrollToBottom = () => {
    chatEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }

  useEffect(scrollToBottom, [chatLog])

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

  const runDeployStep = async (endpoint: string, newStatus: DeployStatus): Promise<any> => {
    setDeployStatus(newStatus)
    const url = `/api/terraform/${endpoint}`
    console.log('Fetching:', url)
    
    try {
      const response = await fetch(url, {
        method: 'POST',
      })
      
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`)
      }
      
      const reader = response.body?.getReader()
      const decoder = new TextDecoder()
      
      if (!reader) {
        throw new Error('No response body')
      }
      
      let buffer = ''
      
      while (true) {
        const { done, value } = await reader.read()
        if (done) break
        
        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() || ''
        
        for (const line of lines) {
          if (line.startsWith('data: ')) {
            try {
              const data = JSON.parse(line.slice(6))
              if (data.line) {
                setDeployOutput(prev => [...prev, data.line])
              }
              if (data.error) {
                setDeployError(data.error)
                setDeployOutput(prev => [...prev, `[ERROR] ${data.error}`])
                setDeployStatus('error')
                return Promise.reject(new Error(data.error))
              }
              if (data.done) {
                setDeployStatus('success')
                return Promise.resolve(data)
              }
              if (data.has_changes !== undefined) {
                setPlanHasChanges(data.has_changes)
              }
            } catch {
              // ignore parse errors
            }
          }
        }
      }
      
      setDeployStatus('success')
      return Promise.resolve({})
    } catch (err: any) {
      const errorMsg = err.message || `Failed to connect to ${endpoint}`
      setDeployError(errorMsg)
      setDeployOutput(prev => [...prev, `[ERROR] ${errorMsg}`])
      setDeployStatus('error')
      return Promise.reject(err)
    }
  }

  const handleDeploy = async () => {
    setDeployOutput([])
    setDeployError(null)
    setPlanHasChanges(null)
    setDeployStatus('init')
    
    try {
      await runDeployStep('init', 'init')
      const planResult = await runDeployStep('plan', 'plan')
      
      if (planResult?.has_changes === false) {
        setDeployOutput(prev => [...prev, 'No changes to apply.'])
        return
      }
      
      await runDeployStep('apply', 'apply')
    } catch (err: any) {
      // error already handled in runDeployStep
      console.error('Deployment failed:', err)
    }
  }

  const handleReset = () => {
    setDeployStatus('idle')
    setDeployOutput([])
    setDeployError(null)
    setPlanHasChanges(null)
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
            <button
              className={activeTab === 'deploy' ? 'active' : ''}
              onClick={() => setActiveTab('deploy')}
            >
              Deploy
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

            {activeTab === 'deploy' && (
              <div className="deploy-panel">
                <div className="deploy-header">
                  <div className={`deploy-status status-${deployStatus}`}>
                    {deployStatus === 'idle' && 'Ready to deploy'}
                    {deployStatus === 'init' && 'Running terraform init...'}
                    {deployStatus === 'plan' && 'Running terraform plan...'}
                    {deployStatus === 'apply' && 'Running terraform apply...'}
                    {deployStatus === 'success' && 'Deployment successful!'}
                    {deployStatus === 'error' && 'Deployment failed'}
                  </div>
                  <div className="deploy-actions">
                    {deployStatus === 'idle' && (
                      <button className="deploy-btn" onClick={handleDeploy}>
                        Deploy
                      </button>
                    )}
                    {deployStatus === 'success' && (
                      <button className="deploy-btn" onClick={handleReset}>
                        Reset
                      </button>
                    )}
                    {(deployStatus === 'error' || deployStatus === 'success') && (
                      <button className="deploy-btn" onClick={handleDeploy}>
                        Deploy Again
                      </button>
                    )}
                    {(deployStatus === 'init' || deployStatus === 'plan' || deployStatus === 'apply') && (
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
                  {deployOutput.length === 0 && deployStatus === 'idle' && (
                    <p className="empty-state">Click Deploy to run terraform init, plan, and apply.</p>
                  )}
                  {deployOutput.map((line, i) => (
                    <div key={i} className={`output-line ${line.includes('[ERROR]') ? 'error' : ''}`}>{line}</div>
                  ))}
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
