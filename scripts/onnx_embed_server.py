#!/usr/bin/env python3
"""
ONNX Embedding 服务进程：通过标准输入输出与 Go 主程序通信
用法: python onnx_embed_server.py --model-path <path> --tokenizer-path <path>
输入: 每行一个 JSON 对象 {"texts": ["文本1", "文本2", ...]}
输出: 每行一个 JSON 对象 {"vectors": [[...], [...], ...]} 或 {"error": "..."}
"""
import argparse
import json
import sys

from onnx_embed_lib import ONNXEmbedder


def main() -> int:
    parser = argparse.ArgumentParser(description="ONNX Embedding Server")
    parser.add_argument("--model-path", required=True, help="ONNX model file path")
    parser.add_argument("--tokenizer-path", required=True, help="Tokenizer file path")
    args = parser.parse_args()

    # 初始化嵌入器
    embedder = ONNXEmbedder(args.model_path, args.tokenizer_path)

    # 主循环：从标准输入读取请求并输出响应
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
