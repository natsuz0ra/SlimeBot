#!/usr/bin/env python3
import argparse
import json
import sys

from onnx_embed_lib import ONNXEmbedder


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--model-path", required=True)
    parser.add_argument("--tokenizer-path", required=True)
    args = parser.parse_args()
    embedder = ONNXEmbedder(args.model_path, args.tokenizer_path)
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            req = json.loads(line)
            texts = req.get("texts")
            if not isinstance(texts, list):
                raise ValueError("texts must be an array")
            vectors = embedder.embed(texts)
            print(json.dumps({"vectors": vectors}, ensure_ascii=False), flush=True)
        except Exception as exc:
            print(json.dumps({"error": str(exc)}, ensure_ascii=False), flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
