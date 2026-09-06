/**
 * Markdown renderer - simple but functional.
 * Handles: **bold**, *italic*, `inline code`, ```code blocks``,
 * lists, links, headings, horizontal rules.
 *
 * This is a simplified version of DSH's AssistantMarkdown component.
 */

import React, { useCallback, useRef, useState } from 'react'

interface MarkdownProps {
  content: string
}

export function MarkdownRenderer({ content }: MarkdownProps) {
  const elements = React.useMemo(() => parseMarkdown(content), [content])

  return <>{elements}</>
}

function parseMarkdown(content: string): React.ReactNode[] {
  const lines = content.split('\n')
  const elements: React.ReactNode[] = []
  let inCodeBlock = false
  let codeContent: string[] = []
  let codeLang = ''
  let listItems: string[] = []
  let inList = false

  function flushList() {
    if (listItems.length > 0) {
      elements.push(
        <ul key={`list-${elements.length}`}>
          {listItems.map((item, i) => (
            <li key={i}><span className="dsh-m-list-text">{parseInline(item.trim())}</span></li>
          ))}
        </ul>
      )
      listItems = []
      inList = false
    }
  }

  function flushCodeBlock() {
    if (codeContent.length > 0) {
      const codeText = codeContent.join('\n')
      elements.push(
        <CodeBlockWrapper
          key={`code-${elements.length}`}
          code={codeText}
          lang={codeLang}
        />
      )
      codeContent = []
      codeLang = ''
    }
  }

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]

    // Code block
    if (line.startsWith('```')) {
      if (inCodeBlock) {
        flushCodeBlock()
        inCodeBlock = false
      } else {
        flushList()
        inCodeBlock = true
        codeLang = line.slice(3).trim()
      }
      continue
    }

    // Inside code block
    if (inCodeBlock) {
      codeContent.push(line)
      continue
    }

    // Horizontal rule
    if (/^(-{3,}|_{3,}|\*{3,})$/.test(line.trim())) {
      flushList()
      elements.push(<hr key={`hr-${elements.length}`} className="dsh-m-hr" />)
      continue
    }

    // Headings - use React.createElement for dynamic tag
    const headingMatch = line.match(/^(#{1,6})\s+(.*)$/)
    if (headingMatch) {
      flushList()
      const level = headingMatch[1].length as 1 | 2 | 3 | 4 | 5 | 6
      const tag = `h${level}` as 'h1' | 'h2' | 'h3' | 'h4' | 'h5' | 'h6'
      elements.push(
        React.createElement(tag, { key: `h-${elements.length}`, className: `dsh-m-h${level}` },
          parseInline(headingMatch[2])
        )
      )
      continue
    }

    // Unordered list items
    const listMatch = line.match(/^(\s*)[-*+]\s+(.*)$/)
    if (listMatch) {
      if (!inList) inList = true
      listItems.push(listMatch[2])
      continue
    }

    // Numbered list
    const numListMatch = line.match(/^(\s*)\d+\.\s+(.*)$/)
    if (numListMatch) {
      if (!inList) inList = true
      listItems.push(numListMatch[2])
      continue
    }

    // Flush list on non-list line
    if (inList && line.trim() !== '') {
      flushList()
    }

    // Empty line
    if (line.trim() === '') {
      flushList()
      continue
    }

    // Regular paragraph
    elements.push(
      <p key={`p-${elements.length}`}>
        {parseInline(line)}
      </p>
    )
  }

  // Flush remaining
  flushList()
  flushCodeBlock()

  return elements
}

/* ── Inline parsing ── */
function parseInline(text: string): React.ReactNode {
  const parts: React.ReactNode[] = []
  let remaining = text
  let key = 0

  while (remaining.length > 0) {
    // Inline code
    const codeIdx = remaining.indexOf('`')
    if (codeIdx !== -1) {
      if (codeIdx > 0) {
        parts.push(remaining.slice(0, codeIdx))
      }
      const endIdx = remaining.indexOf('`', codeIdx + 1)
      if (endIdx !== -1) {
        parts.push(
          <code key={`ic-${key++}`} className="dsh-m-code">
            {remaining.slice(codeIdx + 1, endIdx)}
          </code>
        )
        remaining = remaining.slice(endIdx + 1)
      } else {
        parts.push(remaining[codeIdx])
        remaining = remaining.slice(codeIdx + 1)
      }
    } else {
      // Bold + italic
      const biIdx = remaining.indexOf('***')
      // Bold
      const bIdx = remaining.indexOf('**')
      // Italic
      const iIdx = remaining.indexOf('*')

      // Find the earliest
      let minIdx = remaining.length
      let tag = ''
      let tagLen = 0

      if (biIdx !== -1 && biIdx < minIdx) { minIdx = biIdx; tag = '***'; tagLen = 3 }
      else if (bIdx !== -1 && bIdx < minIdx) { minIdx = bIdx; tag = '**'; tagLen = 2 }
      else if (iIdx !== -1 && iIdx < minIdx) { minIdx = iIdx; tag = '*'; tagLen = 1 }

      if (minIdx < remaining.length) {
        parts.push(remaining.slice(0, minIdx))
        const closeIdx = remaining.indexOf(tag, minIdx + tagLen)
        if (closeIdx !== -1) {
          const inner = remaining.slice(minIdx + tagLen, closeIdx)
          if (tagLen === 3) {
            parts.push(React.createElement('strong', { key: `bi-${key++}` },
              React.createElement('em', null, parseInline(inner))
            ))
          } else if (tagLen === 2) {
            parts.push(React.createElement('strong', { key: `b-${key++}` },
              parseInline(inner)
            ))
          } else {
            parts.push(React.createElement('em', { key: `i-${key++}` },
              parseInline(inner)
            ))
          }
          remaining = remaining.slice(closeIdx + tagLen)
        } else {
          parts.push(tag)
          remaining = remaining.slice(minIdx + tagLen)
        }
      } else {
        parts.push(remaining)
        remaining = ''
      }
    }
  }

  return parts.length === 1 ? parts[0] : parts
}

/* ── Code block wrapper with language banner and copy button ── */
function CodeBlockWrapper({ code, lang }: { code: string; lang: string }) {
  const preRef = useRef<HTMLPreElement>(null)
  const [copied, setCopied] = useState(false)

  const onCopy = useCallback(() => {
    if (copied) return
    const text = preRef.current?.textContent ?? code
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true)
      window.setTimeout(() => setCopied(false), 1500)
    })
  }, [copied, code])

  const trimmed = code.endsWith('\n') ? code.slice(0, -1) : code

  return (
    <div className="dsh-m-codeblock">
      <div className="dsh-m-codeblock-banner">
        {lang && <span className="dsh-m-codeblock-lang">{lang.toUpperCase()}</span>}
        <button
          type="button"
          className="dsh-m-codeblock-copy"
          onClick={onCopy}
          title={copied ? '已复制' : '复制代码'}
        >
          {copied ? '已复制' : '复制'}
        </button>
      </div>
      <pre className="dsh-m-codeblock-body" ref={preRef}>
        <code>{trimmed}</code>
      </pre>
    </div>
  )
}
