import { Box, Text, useInput } from "ink";
import { wrapText } from "../utils/format.js";
import { MENU_ITEM_COLORS } from "./MenuView.js";

interface QuestionItem {
  id: string;
  question: string;
  options: string[];
  option_descriptions?: string[];
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
  return a.customAnswer || "(Unanswered)";
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
          <Text bold color="cyan">Confirm Answers</Text>
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
          <Text color={cursor === 0 ? "cyan" : "gray"}>[Enter] Submit</Text>
          <Text>  </Text>
          <Text color={cursor === 1 ? "yellow" : "gray"}>[Esc] Back</Text>
        </Box>
      </Box>
    );
  }

  const q = questions[currentIndex];
  if (!q) return <Text>No questions</Text>;
  const currentAnswer = answers[currentIndex];
  const totalOptions = q.options.length + 1; // +1 for custom
  const isCustomSelected = currentAnswer?.selectedOption === -1;

  return (
    <Box flexDirection="column" paddingX={1} width={width}>
      <Box borderStyle="round" borderColor="cyan" paddingX={1}>
        <Text bold color="cyan">Questions</Text>
        <Text> ({currentIndex + 1}/{questions.length})</Text>
      </Box>
      <Box flexDirection="column" marginTop={1}>
        <Text bold>{q.question}</Text>
        {q.options.map((opt, oi) => {
          const isSelected = currentAnswer?.selectedOption === oi;
          const isHovered = cursor === oi;
          const desc = q.option_descriptions?.[oi];
          return (
            <Box key={oi} flexDirection="column">
              <Box>
                <Text>{isHovered ? "❯" : " "} </Text>
                <Text color={isSelected ? "green" : isHovered ? "cyan" : "white"}>
                  {isSelected ? "●" : "○"} {opt}
                </Text>
              </Box>
              {desc && wrapText(desc, width - 4).split("\n").map((line, li) => (
                <Text key={`desc-${oi}-${li}`} color={MENU_ITEM_COLORS.description}>
                  {`    ${line}`}
                </Text>
              ))}
            </Box>
          );
        })}
        {/* Custom option */}
        <Box>
          <Text>{cursor === q.options.length ? "❯" : " "} </Text>
          <Text color={isCustomSelected ? "green" : cursor === q.options.length ? "cyan" : "white"}>
            {isCustomSelected ? "●" : "○"} Custom input
          </Text>
        </Box>
        {isCustomSelected && (
          <Box marginLeft={2} marginTop={0}>
            <Text color="gray">{customInput || "Type custom answer..."}</Text>
          </Box>
        )}
      </Box>
      <Box marginTop={1}>
        <Text color="gray">Up/Down to navigate | Enter to select | Esc to cancel</Text>
      </Box>
    </Box>
  );
}
