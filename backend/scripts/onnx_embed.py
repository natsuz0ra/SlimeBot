#!/usr/bin/env python3
"""
Local ONNX embedding helper.

Example:
python ./scripts/onnx_embed.py \
  --model-path ./models/bge-m3/model.onnx \
  --tokenizer-path ./models/bge-m3/tokenizer.json \
  --texts-json "[\"hello\",\"world\"]"
"""

import argparse
import json
import os
import sys

import numpy as np
import onnxruntime as ort
from transformers import AutoTokenizer, PreTrainedTokenizerFast


def mean_pool(last_hidden_state: np.ndarray, attention_mask: np.ndarray) -> np.ndarray:
    mask = np.expand_dims(attention_mask, axis=-1).astype(np.float32)
    summed = np.sum(last_hidden_state * mask, axis=1)
    counts = np.clip(np.sum(mask, axis=1), a_min=1e-9, a_max=None)
    pooled = summed / counts
    norms = np.linalg.norm(pooled, axis=1, keepdims=True)
    return pooled / np.clip(norms, 1e-12, None)


def run(model_path: str, tokenizer_path: str, texts: list[str]) -> list[list[float]]:
    if not texts:
        return []

    tokenizer = load_tokenizer(tokenizer_path)
    ensure_padding_token(tokenizer)
    encoded = tokenizer(
        texts,
        padding=True,
        truncation=True,
        max_length=512,
        return_tensors="np",
    )

    session = ort.InferenceSession(model_path, providers=["CPUExecutionProvider"])
    outputs = session.run(
        None,
        {
            "input_ids": encoded["input_ids"].astype(np.int64),
            "attention_mask": encoded["attention_mask"].astype(np.int64),
        },
    )
    # bge family generally returns hidden states in first output.
    last_hidden_state = outputs[0]
    vectors = mean_pool(last_hidden_state, encoded["attention_mask"])
    return vectors.astype(np.float32).tolist()


def load_tokenizer(tokenizer_path: str):
    # Support both a tokenizer directory and a single tokenizer.json file.
    if os.path.isfile(tokenizer_path):
        return PreTrainedTokenizerFast(tokenizer_file=tokenizer_path)
    return AutoTokenizer.from_pretrained(tokenizer_path, local_files_only=True)


def ensure_padding_token(tokenizer) -> None:
    if tokenizer.pad_token is not None:
        return
    vocab = tokenizer.get_vocab()
    if "<pad>" in vocab:
        tokenizer.pad_token = "<pad>"
        return
    if tokenizer.eos_token is not None:
        tokenizer.pad_token = tokenizer.eos_token
        return
    tokenizer.add_special_tokens({"pad_token": "[PAD]"})


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--model-path", required=True)
    parser.add_argument("--tokenizer-path", required=True)
    parser.add_argument("--texts-json", required=True)
    args = parser.parse_args()

    try:
        texts = json.loads(args.texts_json)
        if not isinstance(texts, list):
            raise ValueError("texts-json must be a JSON array")
        vectors = run(args.model_path, args.tokenizer_path, [str(x) for x in texts])
        print(json.dumps({"vectors": vectors}, ensure_ascii=False))
        return 0
    except Exception as exc:  # pylint: disable=broad-except
        print(json.dumps({"error": str(exc)}, ensure_ascii=False), file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
