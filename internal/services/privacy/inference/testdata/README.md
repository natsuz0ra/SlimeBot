# 推理参考样例

`reference.json` 只包含合成文本与参考结果，不包含用户数据或模型权重。

模型来自 [QuantumNous/astrlink-guard](https://huggingface.co/QuantumNous/astrlink-guard/tree/7216b6a3b897ef903d4b1ba367ae6fb95b0fa5e4)，版本固定为 `7216b6a3b897ef903d4b1ba367ae6fb95b0fa5e4`。各文件的大小与 SHA-256 见上级目录的 `model.go`。

参考结果于 2026-11-04 生成。分词参考使用发布者的 `tokenizer.json` 与 Hugging Face Tokenizers 0.22.2；logits 使用 ONNX Runtime 1.24.4 的 CPU 执行器。生成参考时用过 Python；运行项目和读取这些样例均不需要 Python。

188 组样例中，186 组检查 token ID 和 UTF-8 字节偏移。样例覆盖中、英、日、韩文字、兼容字符、组合音标、特殊 token、空白和未知字符，并包含固定种子的随机组合。12 组保存完整 logits 与默认阈值下的实体。另有 2 组长文本覆盖重叠窗口和 BIO 合并。

运行时用额外的原文覆盖范围处理归一化。参考分词器删除开头的控制字符时，可能将后续实体的偏移前移；组合音标也可能只映射到原文中的第一个字符。脱敏路径修正这些范围，完整覆盖实际原文，同时保持 token ID 不变。测试另行检查这些情况。

浮点归约和动态量化舍入会受到实现与平台影响。测试要求参考语句的分类相同、logits 最大绝对误差不超过 0.5、平均绝对误差不超过 0.1，且实体位置和类别相同。实体概率误差不超过 0.05。这些样例用于兼容性回归，不衡量模型对真实个人信息的整体准确率。

设置 `SLIMEBOT_PRIVACY_TEST_MODEL_DIR` 指向已校验的本地模型目录后，运行：

```powershell
go test ./internal/services/privacy/...
go test -run '^$' -bench BenchmarkGuard -benchmem ./internal/services/privacy/inference
```

测试不会下载模型，也不会连接模型厂商。
