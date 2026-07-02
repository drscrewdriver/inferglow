// Copyright 2026 InferGlow Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package contextmgr

// SummarySystemPrompt steers the LLM to distill older history into a
// structured briefing it can keep relying on after the originals are dropped.
// The section layout mirrors what a coding agent actually needs to resume work
// mid-task: the goal verbatim, the concrete state of the code, and an explicit
// next step — so the post-compaction turn doesn't lose the thread or re-derive
// decisions already made.
//
// Modeled after Reasonix compact.go summarySystemPrompt.
const SummarySystemPrompt = `You are compacting the earlier part of a coding agent's conversation to save context.
The agent keeps your summary alongside the user's own turns (kept verbatim) and the recent tail; your job is to fold the assistant/tool work into a briefing it can resume from.
Write under these exact headings, omitting a heading only if it has no content:

## Standing facts & constraints
Everything the user stated that still governs the work — names, paths, IDs, versions, tokens, preferences, and hard "never do X" rules — in their own words. Be exhaustive; this is the durable contract, so prefer over- to under-including.

## Goal
The user's request and intent.

## Decisions & rationale
Key choices made so far and why — so they are not re-litigated or reversed.

## Files & code
Files read or modified, with the specific facts that matter: signatures, line locations, data shapes, and exact edits applied. Be concrete; this is what lets the agent act without re-reading everything.

## Commands & outcomes
Commands run (builds, tests, git) and their relevant results — what passed, what failed, and the error text that matters.

## Errors & fixes
Problems hit and how they were resolved (or not), so the same dead ends are not repeated.

## Pending & next step
What is still in progress or unstarted, and the single most concrete next action to take.

Rules: be terse — bullet points and fragments, not prose. Preserve identifiers, paths, and numbers exactly. Do NOT invent anything not present in the messages; if something is unknown, leave it out rather than guessing.`

// SummaryTagOpen wraps the compaction summary so the model can distinguish it
// from live user input and later strip or skip it when reasoning about the
// current turn.
const SummaryTagOpen = "<compaction-summary>"

// SummaryTagClose closes the compaction summary block.
const SummaryTagClose = "</compaction-summary>"

// SummaryPrefix is prepended to the summary content inside the tags.
const SummaryPrefix = "Summary of earlier conversation (older messages were compacted to save context):\n"
