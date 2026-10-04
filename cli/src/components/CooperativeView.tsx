import React from "react";
import { Box, Text, useInput } from "ink";
import type { CooperativeSnapshot } from "../types/cooperative.js";
import { CLI_ACCENT_COLOR } from "../utils/terminal.js";

export default function CooperativeView({snapshot,refresh,initialTab}: {snapshot: CooperativeSnapshot|null;refresh:()=>void;initialTab:number}) {
  const [tab,setTab]=React.useState(initialTab);
  React.useEffect(() => { setTab(initialTab); setCursor(0); setDetail(false); }, [initialTab]);
  const [cursor,setCursor]=React.useState(0);
  const [detail,setDetail]=React.useState(false);
  const rows=tab===0?snapshot?.agents:tab===1?snapshot?.tasks:snapshot?.artifacts;
  useInput((input,key)=>{
    if(key.leftArrow||key.rightArrow){setTab(t=>(t+(key.rightArrow?1:2))%3);setCursor(0);setDetail(false)}
    if(key.upArrow||key.downArrow){setCursor(c=>Math.max(0,Math.min((rows?.length||1)-1,c+(key.downArrow?1:-1))));setDetail(false)}
    if(key.return)setDetail(d=>!d);
    if(input==="r")refresh();
  });
  const selected=rows?.[cursor];
  const turn=selected&&tab===0?snapshot?.turns.find(t=>t.agentId===selected.id):undefined;
  return <Box flexDirection="column" borderStyle="round" borderColor={CLI_ACCENT_COLOR} paddingX={1}>
    <Text bold color={CLI_ACCENT_COLOR}>Collaboration workspace</Text>
    <Box gap={3}>{["Agents","Tasks","Artifacts"].map((name,i)=><Text key={name} bold={tab===i} color={tab===i?"white":"gray"}>{name}</Text>)}</Box>
    {!rows?.length?<Text color="gray">No delegated work yet.</Text>:rows.slice(Math.max(0,cursor-5),cursor+6).map(row=>{
      const status=tab===0?snapshot?.turns.find(t=>t.agentId===row.id)?.status:(row as {status:string}).status;
      return <Text key={row.id} color={row.id===selected?.id?"white":"gray"}>{row.id===selected?.id?"›":" "} {"title" in row?row.title:row.id} · {status||"pending"}</Text>;
    })}
    {selected&&<Box marginTop={1} flexDirection="column"><Text color="gray">ID: {selected.id}</Text>{"workspace" in selected&&<Text color="gray">{selected.workspace}</Text>}
      {turn&&<Text color={turn.status==="failed"?"red":"gray"}>{turn.status} · {turn.inputTokens+turn.outputTokens} tokens</Text>}
      {detail&&turn&&<Text>{(turn.answer||turn.error||"Awaiting report").slice(0,5000)}</Text>}
      {detail&&selected&&"report" in selected&&<Text>{selected.report}</Text>}
    </Box>}
    {!!snapshot?.approvals?.filter(a=>a.status==="pending").length&&<Box flexDirection="column" marginTop={1}>
      <Text bold color="yellow">Pending approvals</Text>
      {snapshot?.approvals.filter(a=>a.status==="pending").map(a=><Box key={a.id} flexDirection="column">
        <Text color="yellow">{a.id} · {approvalSummary(a.payload)}</Text>
        <Text dimColor>Agent: {a.agentId} · /agents approve {a.id} yes|no</Text>
      </Box>)}
    </Box>}
    <Box flexDirection="column" marginTop={1}><Text dimColor>←/→ views · ↑/↓ select · Enter report · r refresh · Esc close</Text>
      <Text dimColor>/agents send &lt;id&gt; message · /agents stop &lt;id|all&gt;</Text>
      <Text dimColor>/agents result &lt;id&gt; [turn-id] · full report</Text>
      <Text dimColor>/agents artifact &lt;id&gt; inspect|integrate|validate command</Text>
    </Box>
  </Box>;
}

function approvalSummary(payload: string): string {
  try { const p = JSON.parse(payload); return [p.toolName, p.command].filter(Boolean).join(" · "); }
  catch { return "Tool execution"; }
}
