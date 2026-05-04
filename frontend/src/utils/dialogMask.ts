export interface MaskInteractionState {
  closeOnMask: boolean
  pointerDownStartedOnMask: boolean
  eventTarget: EventTarget | null
  eventCurrentTarget: EventTarget | null
}

export function isMaskSelfEvent(event: Pick<Event, 'target' | 'currentTarget'>): boolean {
  return event.target === event.currentTarget
}

export function shouldCloseOnMaskInteraction(state: MaskInteractionState): boolean {
  return state.closeOnMask && state.pointerDownStartedOnMask && state.eventTarget === state.eventCurrentTarget
}
