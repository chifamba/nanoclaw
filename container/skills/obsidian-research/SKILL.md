---
name: obsidian-research
description: Perform deep research on a given topic using the Gemini tool, formulate a comprehensive and structured markdown document, and save it to the research folder.
allowed-tools: Bash, mcp__gemini__generate_content
---

# Deep Research

You have the ability to perform **Deep Research** on any topic and save the results as Markdown.

The research output folder is `/workspace/group/Auto-Research/research`.
The Gemini MCP tool is available as `mcp__gemini__generate_content`.

## Core Workflow

1.  **Status Update - Thinking:** Immediately call `mcp__nanoclaw__send_message` with text "Thinking..." to acknowledge the request.
2.  **Understand the Request:** Identify the core subject and specific angles.
3.  **Status Update - Researching:** Call `mcp__nanoclaw__send_message` with "Researching [topic] using Gemini..."
4.  **STRICT Tool Restriction (CRITICAL):**
    - You MUST exclusively use the `mcp__gemini__generate_content` tool for research.
    - **DO NOT USE** the `Agent` tool, `Explore` agent type, `WebSearch`, or `WebFetch`.
5.  **Status Update - Writing:** After Gemini returns, call `mcp__nanoclaw__send_message` with "Sending report to Obsidian second brain..."
6.  **Format and Save (Obsidian API):**
    - Format as valid Markdown with YAML frontmatter.
    - Path: `Auto-Research/research/YYYY/MM/DD/Filename.md`
    - Use `Bash` to send the content to the Obsidian Local REST API via `curl`.
    - Endpoint: `http://$OBSIDIAN_HOST:$OBSIDIAN_PORT/vault/$path`
    - Method: `POST`
    - Header: `Authorization: Bearer $OBSIDIAN_API_KEY`
    - Header: `Content-Type: text/markdown`
    - **CRITICAL:** If `OBSIDIAN_API_KEY` is not set or the API is unreachable, fall back to saving locally at `/workspace/group/Auto-Research/research/YYYY/MM/DD/Filename.md` and notify the user that Obsidian was unreachable.
7.  **Final Notification:** Send a final `mcp__nanoclaw__send_message` or a final response confirming: "Deep research complete! Content sent to Obsidian vault at [path]."

## Silence Rule
**DO NOT** output your internal planning, tool-use strategy, or thoughts directly as text to the user. Use `send_message` ONLY for the status updates listed above. This prevents the user from seeing internal SDK orchestration.

## Example

When the user says: *"Do deep research on how transformers revolutionized NLP"*

1. **Research:** Call `mcp__gemini__generate_content` with a detailed prompt.

2. **Save to Obsidian:**
   ```bash
   current_date=$(date +%Y/%m/%d)
   filename="How_Transformers_Revolutionized_NLP.md"
   vault_path="Auto-Research/research/$current_date/$filename"
   
   # Prepare the payload
   cat << 'EOF' > /tmp/research.md
   ---
   title: "How Transformers Revolutionized NLP"
   date: 2026-03-17
   tags: [deep-research, nlp, transformers, ai]
   ---

   # How Transformers Revolutionized NLP

   ## Executive Summary
   This report details the transformative impact of the transformer architecture on NLP...
   EOF

   if [ -n "$OBSIDIAN_API_KEY" ]; then
     curl -X POST "http://$OBSIDIAN_HOST:$OBSIDIAN_PORT/vault/$vault_path" \
          -H "Authorization: Bearer $OBSIDIAN_API_KEY" \
          -H "Content-Type: text/markdown" \
          --data-binary @/tmp/research.md \
          --fail && echo "Successfully sent to Obsidian" || {
            echo "Failed to send to Obsidian API, saving locally..."
            mkdir -p /workspace/group/Auto-Research/research/$current_date
            cp /tmp/research.md /workspace/group/Auto-Research/research/$current_date/$filename
          }
   else
     echo "OBSIDIAN_API_KEY not set, saving locally..."
     mkdir -p /workspace/group/Auto-Research/research/$current_date
     cp /tmp/research.md /workspace/group/Auto-Research/research/$current_date/$filename
   fi
   ```

3. **Respond:** *"Deep research complete! Content synced to Obsidian at `Auto-Research/research/2026/03/17/How_Transformers_Revolutionized_NLP.md`."*
