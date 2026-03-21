import os

import numpy as np
import onnxruntime as ort
from tokenizers import Tokenizer


def mean_pool(last_hidden_state: np.ndarray, attention_mask: np.ndarray) -> np.ndarray:
    mask = np.expand_dims(attention_mask, axis=-1).astype(np.float32)
    summed = np.sum(last_hidden_state * mask, axis=1)
    counts = np.clip(np.sum(mask, axis=1), a_min=1e-9, a_max=None)
    pooled = summed / counts
    norms = np.linalg.norm(pooled, axis=1, keepdims=True)
    return pooled / np.clip(norms, 1e-12, None)


def load_tokenizer_json_path(tokenizer_path: str) -> str:
    if os.path.isfile(tokenizer_path):
        return tokenizer_path
    tok_file = os.path.join(tokenizer_path, "tokenizer.json")
    if os.path.isfile(tok_file):
        return tok_file
    raise FileNotFoundError(f"tokenizer.json not found under {tokenizer_path!r}")


class ONNXEmbedder:
    def __init__(self, model_path: str, tokenizer_path: str):
        tok_file = load_tokenizer_json_path(tokenizer_path)
        self.tok = Tokenizer.from_file(tok_file)
        pid = self.tok.token_to_id("<pad>")
        self._pad_id = pid if pid is not None else 1
        self.session = ort.InferenceSession(model_path, providers=["CPUExecutionProvider"])

    def embed(self, texts: list[str]) -> list[list[float]]:
        if not texts:
            return []
        clean: list[str] = []
        for t in texts:
            if t is None:
                continue
            s = t if isinstance(t, str) else str(t)
            s = s.strip()
            if not s:
                continue
            s = s.encode("utf-8", errors="surrogatepass").decode("utf-8", errors="replace")
            clean.append(s)
        if not clean:
            return []
        self.tok.enable_truncation(max_length=512)
        self.tok.enable_padding(pad_id=self._pad_id, length=None)
        encodings = self.tok.encode_batch(clean)
        rows_ids = [list(e.ids) for e in encodings]
        rows_mask = [list(e.attention_mask) for e in encodings]
        input_ids = np.asarray(rows_ids, dtype=np.int64)
        attention_mask = np.asarray(rows_mask, dtype=np.int64)
        outputs = self.session.run(
            None,
            {
                "input_ids": input_ids,
                "attention_mask": attention_mask,
            },
        )
        last_hidden_state = outputs[0]
        vectors = mean_pool(last_hidden_state, attention_mask)
        return vectors.tolist()
