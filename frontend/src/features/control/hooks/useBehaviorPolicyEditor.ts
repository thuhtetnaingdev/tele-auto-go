import { useCallback, useState } from 'react'

import {
  defaultBehaviorPolicy,
  formatQuietHoursInput,
  formatTextList,
  normalizeBehaviorPolicy,
  parseQuietHoursInput,
  parseTextList,
} from '@/features/control/utils'
import type { BehaviorPolicy, BehaviorRuntimeState } from '@/features/control/types'

export function useBehaviorPolicyEditor() {
  const [behaviorPolicy, setBehaviorPolicy] = useState<BehaviorPolicy>(defaultBehaviorPolicy)
  const [behaviorLoadedAt, setBehaviorLoadedAt] = useState('')
  const [behaviorPath, setBehaviorPath] = useState('./behavior.yaml')
  const [behaviorRuntimeStates, setBehaviorRuntimeStates] = useState<BehaviorRuntimeState[]>([])
  const [behaviorQuietHoursInput, setBehaviorQuietHoursInput] = useState('')
  const [behaviorAllowTonesInput, setBehaviorAllowTonesInput] = useState('')
  const [behaviorDenyTonesInput, setBehaviorDenyTonesInput] = useState('')
  const [behaviorTriggerKeywordsInput, setBehaviorTriggerKeywordsInput] = useState('')

  const syncBehaviorPolicyState = useCallback((nextPolicy: BehaviorPolicy, meta?: { loadedAt?: string; path?: string; states?: BehaviorRuntimeState[] }) => {
    const normalized = normalizeBehaviorPolicy(nextPolicy)
    setBehaviorPolicy(normalized)
    setBehaviorQuietHoursInput(formatQuietHoursInput(normalized.quietHours))
    setBehaviorAllowTonesInput(formatTextList(normalized.toneRules.allow))
    setBehaviorDenyTonesInput(formatTextList(normalized.toneRules.deny))
    setBehaviorTriggerKeywordsInput(formatTextList(normalized.escalation.triggerKeywords))
    if (meta?.loadedAt !== undefined) {
      setBehaviorLoadedAt(meta.loadedAt)
    }
    if (meta?.path !== undefined) {
      setBehaviorPath(meta.path)
    }
    if (meta?.states !== undefined) {
      setBehaviorRuntimeStates(meta.states)
    }
  }, [])

  const updateBehaviorQuietHoursInput = useCallback((value: string) => {
    setBehaviorQuietHoursInput(value)
    setBehaviorPolicy((prev) => ({
      ...prev,
      quietHours: parseQuietHoursInput(value),
    }))
  }, [])

  const updateBehaviorAllowTonesInput = useCallback((value: string) => {
    setBehaviorAllowTonesInput(value)
    setBehaviorPolicy((prev) => ({
      ...prev,
      toneRules: { ...prev.toneRules, allow: parseTextList(value) },
    }))
  }, [])

  const updateBehaviorDenyTonesInput = useCallback((value: string) => {
    setBehaviorDenyTonesInput(value)
    setBehaviorPolicy((prev) => ({
      ...prev,
      toneRules: { ...prev.toneRules, deny: parseTextList(value) },
    }))
  }, [])

  const updateBehaviorTriggerKeywordsInput = useCallback((value: string) => {
    setBehaviorTriggerKeywordsInput(value)
    setBehaviorPolicy((prev) => ({
      ...prev,
      escalation: { ...prev.escalation, triggerKeywords: parseTextList(value) },
    }))
  }, [])

  return {
    behaviorPolicy,
    setBehaviorPolicy,
    behaviorLoadedAt,
    behaviorPath,
    behaviorRuntimeStates,
    behaviorQuietHoursInput,
    behaviorAllowTonesInput,
    behaviorDenyTonesInput,
    behaviorTriggerKeywordsInput,
    setBehaviorRuntimeStates,
    syncBehaviorPolicyState,
    updateBehaviorQuietHoursInput,
    updateBehaviorAllowTonesInput,
    updateBehaviorDenyTonesInput,
    updateBehaviorTriggerKeywordsInput,
  }
}
