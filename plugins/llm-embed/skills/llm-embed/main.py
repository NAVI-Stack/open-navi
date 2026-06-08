"""
navi.llm.embed — text embedder skill
Invoked by NAVI's Python subprocess runner via JSON stdio.

Contract:
- Input:  {"text": str, "model"?: str} on stdin
- Output: SkillResult envelope on stdout (written by runner wrapper, not here)
- This module only implements the function; the runner handles the envelope.
"""

from __future__ import annotations

import os
import tiktoken
from openai import OpenAI


def embed(params: dict) -> dict:
    """
    Embed a text string using an OpenAI embedding model.

    Args:
        params: {
            "text":  str   — text to embed (required)
            "model": str   — embedding model name (optional, default: text-embedding-3-small)
        }

    Returns:
        {
            "vector":      list[float]
            "model":       str
            "token_count": int
        }
    """
    text: str = params["text"]
    model: str = params.get("model", "text-embedding-3-small")

    # Count tokens before the API call so callers can gate on cost
    try:
        enc = tiktoken.encoding_for_model(model)
        token_count = len(enc.encode(text))
    except KeyError:
        # Unknown model; fall back to approximate count
        token_count = len(text) // 4

    client = OpenAI(api_key=os.environ["OPENAI_API_KEY"])
    response = client.embeddings.create(input=text, model=model)

    vector: list[float] = response.data[0].embedding

    return {
        "vector": vector,
        "model": model,
        "token_count": token_count,
    }
