#!/usr/bin/env node
// CJS 包装器，用于调用 ESM 入口点
// 这解决了 Node.js ESM 不支持 shebang 的问题

// 动态导入 ESM 模块
async function main() {
  await import('./dist/index.js');
}

main().catch((error) => {
  console.error('Failed to start CLI:', error);
  process.exit(1);
});
