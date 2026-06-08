Summarize this NAVI chat segment into JSON.
Schema:
{"chat_id":"string","turn_count":number,"topic_summary":"string","key_decisions":["string"],"facts_learned":[{"category":"string","key":"string","value":"string","scope":"owner|chat"}],"unresolved_items":["string"],"tools_used":["string"]}
Rules:
- topic_summary should be 2-3 concise sentences.
- facts_learned should only contain durable facts worth remembering.
- key_decisions should capture important choices or commitments.
- unresolved_items should list open threads still worth surfacing later.
- tools_used should be inferred from obvious tool or action mentions when possible.
- Return JSON only.

Chat: {{.ChatID}}
Messages:
{{.MessagesText}}
