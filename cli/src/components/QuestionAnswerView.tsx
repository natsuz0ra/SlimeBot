import { Box, Text, useInput } from "ink";
import { wrapText } from "../utils/format.js";
import { MENU_ITEM_COLORS } from "./MenuView.js";

export const QUESTION_ANSWER_COLORS = {
  title: "#67e8f9",
  activeCursor: "#22d3ee",
  activeText: "#f8fafc",
  inactiveText: "#cbd5e1",
  description: "#94a3b8",
  answered: "#34d399",
  warning: "#fbbf24",
  hint: "#64748b",
} as const;

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

export function getQuestionAnswerTitle(currentIndex: number, totalQuestions: number): string {
  return `Answer questions  Question ${currentIndex + 1} of ${totalQuestions}`;
}

export function getQuestionAnswerProgress(currentIndex: number, totalQuestions: number): string[] {
  return Array.from({ length: totalQuestions }, (_, index) => index <= currentIndex ? "●" : "○");
}

export function getQuestionAnswerViewHints(step: "questions" | "confirm"): string {
  if (step === "confirm") {
    return "↑/↓ select | Enter edit/send | Esc back";
  }
  return "Enter select & next | ←/→ prev/next | C custom | Esc cancel";
}

export default function QuestionAnswerView({
  questions,
  currentIndex,
  answers,
  step,
  cursor,
  customInput,
  onCustomInputChange,
  onCustomInputSubmit,
  onEscape,
  columns,
}: Props) {
  const width = Math.min(columns ?? 80, 80);
  const q = questions[currentIndex];
  const currentAnswer = q ? answers[currentIndex] : undefined;
  const isCustomSelected = q ? currentAnswer?.selectedOption === -1 : false;
  const isCustomCursor = q ? cursor === q.options.length : false;
  const customDisplayValue = currentAnswer?.customAnswer || customInput;

  useInput((input, key) => {
    if (key.escape) {
      onEscape();
      return;
    }
    if (step !== "questions" || !q || (!isCustomSelected && !isCustomCursor)) {
      return;
    }
    if (key.return) {
      onCustomInputSubmit(customInput);
      return;
    }
    if (key.backspace || key.delete) {
      onCustomInputChange(customInput.slice(0, -1));
      return;
    }
    if (key.ctrl || key.meta || key.tab || key.upArrow || key.downArrow || key.leftArrow || key.rightArrow) {
      return;
    }
    if (input && input >= " " && input !== "\x7f") {
      onCustomInputChange(customInput + input);
    }
  });

  if (step === "confirm") {
    return (
      <Box flexDirection="column" paddingX={1} width={width}>
        <Text bold color={QUESTION_ANSWER_COLORS.title}>Review before sending</Text>
        <Box flexDirection="column" marginTop={1}>
          {questions.map((q, i) => {
            const active = cursor === i;
            const answer = answers[i];
            const answered = Boolean(answer && answer.selectedOption !== -2 && getDisplayAnswer(q, answer) !== "(Unanswered)");
            return (
              <Box key={q.id} flexDirection="column" marginBottom={1}>
                <Text>
                  <Text color={active ? QUESTION_ANSWER_COLORS.activeCursor : QUESTION_ANSWER_COLORS.hint}>
                    {active ? "❯ " : "  "}
                  </Text>
                  <Text color={answered ? QUESTION_ANSWER_COLORS.answered : QUESTION_ANSWER_COLORS.hint}>
                    {answered ? "✓ " : "○ "}
                  </Text>
                  <Text bold color={active ? QUESTION_ANSWER_COLORS.warning : QUESTION_ANSWER_COLORS.inactiveText}>
                    {i + 1}. {q.question}
                  </Text>
                </Text>
                <Text color={QUESTION_ANSWER_COLORS.activeText}>     {getDisplayAnswer(q, answers[i])}</Text>
              </Box>
            );
          })}
        </Box>
        <Box marginTop={1}>
          <Text color={cursor === questions.length ? QUESTION_ANSWER_COLORS.activeCursor : QUESTION_ANSWER_COLORS.hint}>
            {cursor === questions.length ? "❯ " : "  "}
          </Text>
          <Text bold={cursor === questions.length} color={cursor === questions.length ? QUESTION_ANSWER_COLORS.activeText : QUESTION_ANSWER_COLORS.inactiveText}>
            Send answers
          </Text>
          <Text color={QUESTION_ANSWER_COLORS.hint}>  or select any row to edit</Text>
        </Box>
        <Box marginTop={1}>
          <Text color={QUESTION_ANSWER_COLORS.hint}>{getQuestionAnswerViewHints("confirm")}</Text>
        </Box>
      </Box>
    );
  }

  if (!q) return <Text>No questions</Text>;

  return (
    <Box flexDirection="column" paddingX={1} width={width}>
      <Text bold color={QUESTION_ANSWER_COLORS.title}>
        {getQuestionAnswerTitle(currentIndex, questions.length)}
      </Text>
      <Text color={QUESTION_ANSWER_COLORS.hint}>
        Progress{"  "}
        {getQuestionAnswerProgress(currentIndex, questions.length).map((dot, index) => (
          <Text key={index} color={dot === "●" ? QUESTION_ANSWER_COLORS.answered : QUESTION_ANSWER_COLORS.hint}>
            {dot}{index < questions.length - 1 ? " " : ""}
          </Text>
        ))}
      </Text>
      <Box flexDirection="column" marginTop={1}>
        <Text bold color={QUESTION_ANSWER_COLORS.activeText}>{q.question}</Text>
        {q.options.map((opt, oi) => {
          const isSelected = currentAnswer?.selectedOption === oi;
          const isHovered = cursor === oi;
          const desc = q.option_descriptions?.[oi];
          return (
            <Box key={oi} flexDirection="column">
              <Box>
                <Text color={isHovered ? QUESTION_ANSWER_COLORS.activeCursor : QUESTION_ANSWER_COLORS.hint}>
                  {isHovered ? "❯" : " "} {oi + 1}{" "}
                </Text>
                <Text color={isSelected || isHovered ? QUESTION_ANSWER_COLORS.activeText : QUESTION_ANSWER_COLORS.inactiveText}>
                  {opt}
                </Text>
                {isSelected && <Text color={QUESTION_ANSWER_COLORS.answered}> selected</Text>}
              </Box>
              {desc && wrapText(desc, width - 4).split("\n").map((line, li) => (
                <Text key={`desc-${oi}-${li}`} color={MENU_ITEM_COLORS.description}>
                  {`      ${line}`}
                </Text>
              ))}
            </Box>
          );
        })}
        <Box>
          <Text color={isCustomCursor ? QUESTION_ANSWER_COLORS.activeCursor : QUESTION_ANSWER_COLORS.hint}>
            {isCustomCursor ? "❯" : " "} C{" "}
          </Text>
          <Text color={isCustomSelected || isCustomCursor ? QUESTION_ANSWER_COLORS.activeText : QUESTION_ANSWER_COLORS.inactiveText}>
            Custom answer...
          </Text>
          {isCustomSelected && <Text color={QUESTION_ANSWER_COLORS.answered}> selected</Text>}
        </Box>
        {(isCustomSelected || isCustomCursor) && (
          <Box marginLeft={4} marginTop={0}>
            <Text color={customDisplayValue ? QUESTION_ANSWER_COLORS.activeText : QUESTION_ANSWER_COLORS.description}>
              {customDisplayValue || "Type custom answer..."}
            </Text>
          </Box>
        )}
      </Box>
      <Box marginTop={1}>
        <Text color={QUESTION_ANSWER_COLORS.hint}>{getQuestionAnswerViewHints("questions")}</Text>
      </Box>
    </Box>
  );
}
