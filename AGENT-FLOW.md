# 对话执行控制

## 运行中追加变更

Web 和桌面端生成回复时，可以继续在输入框输入文字。按 Enter 或点击发送，会将文字追加到当前任务。停止按钮仍独立可用。CLI 在执行中也接受文本输入。运行中暂不支持追加附件。

服务端返回 `start.turnId`。客户端提交 `steer` 时，必须带上会话 ID、目标 turn ID 和独立的 submission ID。相同 submission ID 的重试只保存一次；目标轮次已结束时会拒绝变更，不会将文字自动转交给下一轮。

已接收的文字先写入会话历史，再返回 `steer_accepted`。模型在下一次请求中读取变更。正在执行的工具先返回结果，尚未执行的旧工具调用会跳过。待审批的操作会退出等待，让模型根据最新要求重新决定。

追加变更不会更改本轮的模型、工作目录、计划模式或权限设置。停止任务后，已接收的文字仍保留在历史中。Web 拒绝的变更显示为发送失败；CLI 保留未发送的文字，便于重新提交。

实现入口：`internal/services/chat/steering.go`、`internal/services/chat/agent.go` 和 `internal/server/ws/ws_controller.go`。

行为参考 [Codex App Server 的 turn/steer 协议](https://learn.chatgpt.com/docs/app-server#steer-an-active-turn)。SlimeBot 使用自己的 Go 执行循环与会话存储，不依赖 Codex 服务。

验证覆盖目标轮次、重试去重、会话互斥、最终回复前到达的变更、旧工具调用和审批等待。Web 与 CLI 协议测试确认追加变更不会结束当前流。
