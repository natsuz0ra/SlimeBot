// Teleported dialogs share keyboard ownership. An underlying modal must not
// close or steal Tab focus while a higher modal (such as collaboration) is open.
export function topmostModal<T>(panels: Array<{
  panel: T
  layer: number
  visible: boolean
}>): T | undefined {
  let top: {
    panel: T
    layer: number
  } | undefined
  for (const entry of panels)
    if (entry.visible && (!top || entry.layer >= top.layer))
      top = entry
  return top?.panel
}
export function ownsModalKeyboard(panel: HTMLElement | null | undefined): boolean {
  if (!panel)
    return false
  const panels = [...document.querySelectorAll<HTMLElement>('[role="dialog"][aria-modal="true"]')]
  return topmostModal(panels.map(element => ({ panel: element, layer: Number(element.dataset.modalLayer || 200), visible: element.getClientRects().length > 0 }))) === panel
}
export function modalFocusableElements(panel: HTMLElement): HTMLElement[] {
  return [...panel.querySelectorAll<HTMLElement>('button:not(:disabled), a[href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), summary, [tabindex]:not([tabindex="-1"])')]
    .filter(element => element.getClientRects().length > 0 && !element.closest('[hidden]') && [...panel.querySelectorAll('details:not([open])')].every(details => !details.contains(element) || details.querySelector(':scope > summary')?.contains(element)))
}
