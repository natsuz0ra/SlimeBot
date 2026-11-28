export interface SessionSelection {
  id: string;
  name: string;
  generation: number;
}

export function captureSessionRequest(
  selection: { current: SessionSelection },
  sessionId = selection.current.id,
): () => boolean {
  const { generation } = selection.current;
  return () => selection.current.id === sessionId && selection.current.generation === generation;
}

export async function applySessionResponse<T>(
  isCurrent: () => boolean,
  request: () => Promise<T>,
  onSuccess: (result: T) => void,
  onError?: (error: unknown) => void,
): Promise<void> {
  if (!isCurrent()) return;
  try {
    const result = await request();
    if (isCurrent()) onSuccess(result);
  } catch (error) {
    if (isCurrent()) onError?.(error);
  }
}
