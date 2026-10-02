import { useEffect, useRef, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { aiApi } from '../services/api'
import Icon from './Icon'
import './AIShoppingAssistant.css'

const productQuickPrompts = [
  ['Product rundown', 'Explain the product I am viewing using its listed facts. Cover useful pros, trade-offs, fit, and value.'],
  ['Compare alternatives', 'Compare relevant alternatives in the current catalog. Use listed prices and features, and explain what is unknown.'],
  ['Review information', 'Summarize verified customer reviews for this product. If no review data is available, say so clearly.'],
]
const catalogQuickPrompts = [
  ['Find products', 'Find a few relevant products in the current catalog and tell me their listed prices and stock.'],
  ['Compare options', 'Compare relevant products in the current catalog using their listed prices and features.'],
  ['Review information', 'Summarize verified customer reviews for a product I name. If no review data is available, say so clearly.'],
]

export default function AIShoppingAssistant() {
  const location = useLocation()
  const [open, setOpen] = useState(false)
  const [messages, setMessages] = useState([])
  const [draft, setDraft] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const conversationRef = useRef(null)

  const productID = location.pathname.match(/^\/products\/([^/]+)$/)?.[1] || ''
  const quickPrompts = productID ? productQuickPrompts : catalogQuickPrompts
  const routeParams = new URLSearchParams(location.search)
  const searchQuery = routeParams.get('q') || ''
  const searchContext = [
    searchQuery && `Search: ${searchQuery}`,
    routeParams.get('category') && `Category: ${routeParams.get('category')}`,
    routeParams.get('brand') && `Brand: ${routeParams.get('brand')}`,
  ].filter(Boolean).join('; ')
  const pageContext = `${location.pathname}?${location.search}`

  useEffect(() => {
    setMessages([])
    setError('')
  }, [pageContext])

  useEffect(() => {
    conversationRef.current?.scrollTo({ top: conversationRef.current.scrollHeight, behavior: 'smooth' })
  }, [messages, busy, open])

  const sendMessage = async (text = draft) => {
    const content = text.trim()
    if (!content || busy) return

    const nextMessages = [...messages, { role: 'user', content }].slice(-8)
    const requestMessages = nextMessages.slice(-6).map((message) => {
      if (message.role !== 'assistant' || message.content.length <= 1200) return message
      return { ...message, content: `${message.content.slice(0, 1200)}\n[Earlier response shortened for context.]` }
    })
    setMessages(nextMessages)
    setDraft('')
    setError('')
    setBusy(true)

    try {
      const response = await aiApi.suggest({
        messages: requestMessages,
        search_query: searchContext,
        product_id: productID,
      })
      setMessages([...nextMessages, { role: 'assistant', content: response.data.suggestion }].slice(-8))
    } catch (requestError) {
      setError(requestError.response?.data?.error || 'The assistant could not respond. Please try again.')
    } finally {
      setBusy(false)
    }
  }

  const toggleAssistant = () => {
    if (open) {
      setOpen(false)
      return
    }
    setOpen(true)
    if (!messages.length && (productID || searchContext)) {
      const prompt = productID
        ? 'Analyze the product I am viewing. Summarize its listed features, benefits, trade-offs, value, and relevant catalog alternatives. Tell me what details are missing.'
        : `Analyze my current search and filters (${searchContext}). Find relevant catalog matches, compare useful alternatives, and explain what details are missing.`
      void sendMessage(prompt)
    }
  }

  return (
    <div className="ai-assistant">
      {open && (
        <section className="ai-panel" role="dialog" aria-modal="false" aria-labelledby="ai-title">
          <header className="ai-panel-header">
            <div className="ai-panel-mark"><Icon name="sparkles" size={18} /></div>
            <div className="ai-panel-heading">
              <h2 id="ai-title">Shopping assistant</h2>
              <span>Catalog-grounded guidance</span>
            </div>
            <button className="ai-close" type="button" onClick={() => setOpen(false)} aria-label="Close AI assistant">
              <Icon name="close" size={18} />
            </button>
          </header>

          <div className="ai-conversation" ref={conversationRef} aria-live="polite">
            {!messages.length && (
              <div className="ai-welcome">
                <span className="ai-welcome-icon"><Icon name="sparkles" size={20} /></span>
                <h3>What are you shopping for?</h3>
                <p>Ask for product details, comparisons, or value guidance. I use catalog information and label what is not verified.</p>
                <p className="ai-review-note">This store has no verified customer-review feed, so I will not invent review summaries.</p>
                <div className="ai-quick-prompts">
                  {quickPrompts.map(([label, prompt]) => (
                    <button key={label} type="button" onClick={() => sendMessage(prompt)} disabled={busy}>
                      {label}<Icon name="arrow" size={14} />
                    </button>
                  ))}
                </div>
              </div>
            )}
            {messages.map((message, index) => (
              <div className={`ai-message ${message.role}`} key={`${message.role}-${index}`}>
                <span>{message.role === 'user' ? 'You' : 'FlashCart AI'}</span>
                <p>{message.content}</p>
              </div>
            ))}
            {busy && <div className="ai-thinking"><span /><span /><span />Checking the catalog</div>}
            {error && <div className="ai-error" role="alert">{error}</div>}
          </div>

          <form className="ai-composer" onSubmit={(event) => { event.preventDefault(); sendMessage() }}>
            <textarea
              value={draft}
              onChange={(event) => setDraft(event.target.value)}
              onKeyDown={(event) => {
                if (event.key === 'Enter' && !event.shiftKey) {
                  event.preventDefault()
                  sendMessage()
                }
              }}
              maxLength={1500}
              rows={2}
              placeholder={searchQuery ? `Ask about ${searchQuery} or compare options...` : 'Ask about a product or tell me what you need...'}
              aria-label="Ask the shopping assistant"
            />
            <button type="submit" disabled={busy || !draft.trim()} aria-label="Send message" title="Send message">
              <Icon name="send" size={17} />
            </button>
            <small>Sent with relevant catalog context to Groq. Do not share personal or payment details.</small>
          </form>
        </section>
      )}

      <button
        className={`ai-fab ${open ? 'active' : ''}`}
        type="button"
        onClick={toggleAssistant}
        aria-haspopup="dialog"
        aria-expanded={open}
        aria-label={open ? 'Close AI shopping assistant' : 'Open AI suggestion assistant'}
      >
        <Icon name={open ? 'close' : 'sparkles'} size={19} />
        <span>{open ? 'Close' : 'AI Suggestion'}</span>
      </button>
    </div>
  )
}