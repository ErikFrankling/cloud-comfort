import { useState } from 'react'

function App() {
  const [message, setMessage] = useState('')
  const [chatLog, setChatLog] = useState<{ role: string; text: string }[]>([])

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
        <div className="graph-panel">
          <p>Terraform plan visualization will appear here.</p>
        </div>
      </div>
    </div>
  )
}

export default App
