import { Box, Text, useInput } from "ink";

interface QuestionItem {
  id: string;
  question: string;
  options: string[];
}

interface Answer {
  questionId: string;
  selectedOption: number;
  customAnswer: string;
}

interface Props {
  questions: QuestionItem[];
  currentIndex: number;
  answers: Answer[];
  step: "questions" | "confirm";
  cursor: number;
  customInput: string;
  onCustomInputChange: (value: string) => void;
  onCustomInputSubmit: (value: string) => void;
  onEscape: () => void;
  columns: number;
}

function getDisplayAnswer(q: QuestionItem, a: Answer): string {
  if (a.selectedOption >= 0 && a.selectedOption < q.options.length) return q.options[a.selectedOption];
  return a.customAnswer || "(未回答)";
}

export default function QuestionAnswerView({
  questions,
  currentIndex,
  answers,
  step,
  cursor,
  customInput,
  onEscape,
  columns,
}: Props) {
  useInput((input, key) => {
    if (key.escape) {
      onEscape();
    }
  });

  const width = Math.min(columns ?? 80, 80);

  if (step === "confirm") {
    return (
      <Box flexDirection="column" paddingX={1} width={width}>
        <Box borderStyle="round" borderColor="cyan" paddingX={1}>
          <Text bold color="cyan">确认回答</Text>
        </Box>
        <Box flexDirection="column" marginTop={1}>
          {questions.map((q, i) => (
            <Box key={q.id} flexDirection="column" marginBottom={1}>
              <Text bold>{i + 1}. {q.question}</Text>
              <Text color="green">  → {getDisplayAnswer(q, answers[i])}</Text>
            </Box>
          ))}
        </Box>
        <Box marginTop={1}>
          <Text color={cursor === 0 ? "cyan" : "gray"}>[Enter] 提交</Text>
          <Text>  </Text>
          <Text color={cursor === 1 ? "yellow" : "gray"}>[Esc] 返回</Text>
        </Box>
      </Box>
    );
  }

  const q = questions[currentIndex];
  if (!q) return <Text>无问题</Text>;
  const currentAnswer = answers[currentIndex];
  const totalOptions = q.options.length + 1; // +1 for custom
  const isCustomSelected = currentAnswer?.selectedOption === -1;

  return (
    <Box flexDirection="column" paddingX={1} width={width}>
      <Box borderStyle="round" borderColor="cyan" paddingX={1}>
        <Text bold color="cyan">澄清问题</Text>
        <Text> ({currentIndex + 1}/{questions.length})</Text>
      </Box>
      <Box flexDirection="column" marginTop={1}>
        <Text bold>{q.question}</Text>
        {q.options.map((opt, oi) => {
          const isSelected = currentAnswer?.selectedOption === oi;
          const isHovered = cursor === oi;
          return (
            <Box key={oi}>
              <Text>{isHovered ? "❯" : " "} </Text>
              <Text color={isSelected ? "green" : isHovered ? "cyan" : "white"}>
                {isSelected ? "●" : "○"} {opt}
              </Text>
            </Box>
          );
        })}
        {/* Custom option */}
        <Box>
          <Text>{cursor === q.options.length ? "❯" : " "} </Text>
          <Text color={isCustomSelected ? "green" : cursor === q.options.length ? "cyan" : "white"}>
            {isCustomSelected ? "●" : "○"} 自定义输入
          </Text>
        </Box>
        {isCustomSelected && (
          <Box marginLeft={2} marginTop={0}>
            <Text color="gray">{customInput || "输入自定义回答..."}</Text>
          </Box>
        )}
      </Box>
      <Box marginTop={1}>
        <Text color="gray">↑↓ 选择  Enter 确认  Esc 取消</Text>
      </Box>
    </Box>
  );
}
