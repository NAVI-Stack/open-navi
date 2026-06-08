---
name: summarize
description: "Summarize the content of a URL, file, or pasted text into a concise digest"
metadata:
  navi:
    emoji: "📄"
---
# Summarize

Use this skill when the user asks to summarize, digest, or extract key points from content.

## Supported Inputs

- **URL**: Fetch the page and summarize its main content (articles, docs, GitHub issues, PRs).
- **File path**: Read a local file and summarize it.
- **Pasted text**: Summarize whatever the user has pasted into the conversation.

## Output Format

Provide:
1. A 1–2 sentence TL;DR.
2. 3–5 bullet points covering the key ideas.
3. Any action items or decisions the user should be aware of.

## Rules
- Stay factual — do not add opinions or infer things not in the source.
- For code files, describe what the code does at a high level, not line-by-line.
- For GitHub URLs, use the `gh` CLI if available (`gh issue view`, `gh pr view`) for richer data.
- Keep summaries under 300 words unless the user requests more detail.
