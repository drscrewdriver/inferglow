/**
 * fileIcons — per-extension file glyphs in the DSH outline house style
 * (16px viewBox, 1.5 stroke, currentColor), following the DSH-better-sidebar
 * reference: its FileTree uses generic VscFile/VscFolder outline primitives,
 * so the extension identification rides INSIDE the file frame as a compact
 * type mark. Zero dependencies.
 */

import type { JSX } from 'react'

const frame = 'M4 1.75h5L13.5 6v7.75a1.5 1.5 0 01-1.5 1.5H4a1.5 1.5 0 01-1.5-1.5V3.25A1.5 1.5 0 014 1.75z'
const fold = 'M2 3.25A1.25 1.25 0 013.25 2h3l1.5 1.75h5A1.25 1.25 0 0114 5v1.5H2z'

/** 16px file frame with a small colored type mark (text badge or dot). */
function FileGlyph({ mark, color }: { mark?: string; color?: string }) {
  return (
    <svg width={14} height={14} viewBox="0 0 16 16" fill="none" aria-hidden="true" style={{ flexShrink: 0 }}>
      <path d={frame} stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round" />
      {mark && (
        <text
          x={mark.length > 2 ? 4.6 : 5.6}
          y={12.6}
          fontSize={mark.length > 2 ? 5.4 : 6.2}
          fontWeight={700}
          fill={color ?? 'currentColor'}
          stroke="none"
          fontFamily="ui-sans-serif, system-ui, sans-serif"
        >
          {mark}
        </text>
      )}
    </svg>
  )
}

function FolderGlyph({ open }: { open: boolean }) {
  return (
    <svg width={14} height={14} viewBox="0 0 16 16" fill="none" aria-hidden="true" style={{ flexShrink: 0 }}>
      {open ? (
        <path d="M1.75 4A1.25 1.25 0 013 2.75h3l1.4 1.5h4.85A1.25 1.25 0 0113.5 5.5v1M1.75 4v8.25A1.25 1.25 0 003 13.5h8.6a1.25 1.25 0 001.2-.9l1.35-4.6a1 1 0 00-.96-1.3H4.6a1.25 1.25 0 00-1.2.9L1.75 12.3" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round" />
      ) : (
        <path d={`${fold}M1.75 4.75h12.5V6.5H1.75zM1.75 6.5h12.5l-1.3 5.1a1.25 1.25 0 01-1.2.9H4.25a1.25 1.25 0 01-1.2-.9z`} stroke="currentColor" strokeWidth="1.4" strokeLinejoin="round" />
      )}
    </svg>
  )
}

const EXT_MARKS: Record<string, { mark: string; color: string }> = {
  go: { mark: 'GO', color: '#00ADD8' },
  ts: { mark: 'TS', color: '#3178C6' },
  tsx: { mark: 'TS', color: '#3178C6' },
  js: { mark: 'JS', color: '#F7DF1E' },
  mjs: { mark: 'JS', color: '#F7DF1E' },
  cjs: { mark: 'JS', color: '#F7DF1E' },
  jsx: { mark: 'JS', color: '#61DAFB' },
  json: { mark: '{}', color: '#CB8433' },
  md: { mark: 'M', color: '#519ABA' },
  markdown: { mark: 'M', color: '#519ABA' },
  py: { mark: 'PY', color: '#3572A5' },
  html: { mark: '<>', color: '#E34C26' },
  htm: { mark: '<>', color: '#E34C26' },
  css: { mark: '#', color: '#563D7C' },
  scss: { mark: '#', color: '#C6538C' },
  yaml: { mark: 'Y', color: '#CB171E' },
  yml: { mark: 'Y', color: '#CB171E' },
  toml: { mark: 'T', color: '#9C4221' },
  sh: { mark: '>_', color: '#89E051' },
  ps1: { mark: '>_', color: '#012456' },
  bat: { mark: '>_', color: '#C1F12E' },
  sql: { mark: 'DB', color: '#DD7C36' },
  rs: { mark: 'RS', color: '#DEA584' },
  java: { mark: 'J', color: '#B07219' },
  rb: { mark: 'RB', color: '#701516' },
  php: { mark: 'PHP', color: '#4F5D95' },
  c: { mark: 'C', color: '#555555' },
  cpp: { mark: 'C++', color: '#F34B7D' },
  cs: { mark: 'C#', color: '#178600' },
  png: { mark: 'IMG', color: '#A074C4' },
  jpg: { mark: 'IMG', color: '#A074C4' },
  jpeg: { mark: 'IMG', color: '#A074C4' },
  gif: { mark: 'IMG', color: '#A074C4' },
  svg: { mark: 'SVG', color: '#FFB13B' },
  ico: { mark: 'IMG', color: '#A074C4' },
  zip: { mark: 'ZIP', color: '#E9C462' },
  gz: { mark: 'GZ', color: '#E9C462' },
  tar: { mark: 'TAR', color: '#E9C462' },
  exe: { mark: 'EXE', color: '#8A8F98' },
  dll: { mark: 'DLL', color: '#8A8F98' },
  lock: { mark: '🔒', color: '#8A8F98' },
}

function extOf(name: string): string {
  const i = name.lastIndexOf('.')
  return i >= 0 ? name.slice(i + 1).toLowerCase() : ''
}

/** Icon for a tree row: outline folder (open/closed) or typed file glyph. */
export function fileIcon(name: string, isDir: boolean, open = false): JSX.Element {
  if (isDir) return <FolderGlyph open={open} />
  const hit = EXT_MARKS[extOf(name)]
  if (!hit) return <FileGlyph />
  return <FileGlyph mark={hit.mark} color={hit.color} />
}
