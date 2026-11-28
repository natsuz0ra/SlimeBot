import type { Ref } from 'vue'
import { skillsAPI } from '@/api/skills'
import type { SkillItem } from '@/types/settings'

type ToastLike = {
  error(message: string): void
  success(message: string): void
}

type Translate = (key: string) => string

type SkillUploadFailure = {
  file: string
  error: string
}

function formatUploadFailures(failed: SkillUploadFailure[]) {
  return failed.map((item) => `${item.file}: ${item.error}`).join('\n')
}

export function useSettingsSkills(options: {
  skillsList: Ref<SkillItem[]>
  skillsUploading: Ref<boolean>
  skillsDropActive: Ref<boolean>
  skillsFileInputRef: Ref<HTMLInputElement | null>
  toast: ToastLike
  t: Translate
}) {
  const { skillsList, skillsUploading, skillsDropActive, skillsFileInputRef, toast, t } = options

  function openSkillsPicker() {
    skillsFileInputRef.value?.click()
  }

  function getZipFiles(fileList: FileList | null | undefined) {
    if (!fileList) return []
    return Array.from(fileList).filter((file) => file.name.toLowerCase().endsWith('.zip'))
  }

  async function refreshSkills() {
    skillsList.value = await skillsAPI.list()
  }

  async function uploadSkills(files: File[]) {
    if (!files.length) return
    skillsUploading.value = true
    try {
      const result = await skillsAPI.upload(files)
      const failed = Array.isArray(result?.failed) ? result.failed as SkillUploadFailure[] : []
      if (failed.length > 0) {
        toast.error(`${t('skillsUploadPartial')}\n${formatUploadFailures(failed)}`)
      } else {
        toast.success(t('skillsUploadSuccess'))
      }
      await refreshSkills()
    } catch (err: unknown) {
      const response = err as { response?: { data?: { failed?: unknown; error?: string } } }
      const failed = response.response?.data?.failed
      if (Array.isArray(failed) && failed.length > 0) {
        toast.error(`${t('skillsUploadFailed')}\n${formatUploadFailures(failed as SkillUploadFailure[])}`)
      } else {
        toast.error(response.response?.data?.error || t('skillsUploadFailed'))
      }
    } finally {
      skillsUploading.value = false
      if (skillsFileInputRef.value) skillsFileInputRef.value.value = ''
    }
  }

  function onSkillsInputChange(event: Event) {
    const target = event.target as HTMLInputElement
    const files = getZipFiles(target.files)
    if (!files.length && target.files?.length) {
      toast.error(t('onlyZipAllowed'))
      return
    }
    void uploadSkills(files)
  }

  function onSkillsDrop(event: DragEvent) {
    event.preventDefault()
    skillsDropActive.value = false
    const files = getZipFiles(event.dataTransfer?.files)
    if (!files.length && event.dataTransfer?.files?.length) {
      toast.error(t('onlyZipAllowed'))
      return
    }
    void uploadSkills(files)
  }

  function onSkillsDragOver(event: DragEvent) {
    event.preventDefault()
    skillsDropActive.value = true
  }

  function onSkillsDragLeave(event: DragEvent) {
    event.preventDefault()
    skillsDropActive.value = false
  }

  async function deleteSkill(id: string) {
    await skillsAPI.remove(id)
    await refreshSkills()
  }

  async function setSkillEnabled(id: string, enabled: boolean) {
    const previous = skillsList.value
    skillsList.value = skillsList.value.map((skill) => (skill.id === id ? { ...skill, enabled } : skill))
    try {
      await skillsAPI.setEnabled(id, enabled)
      await refreshSkills()
    } catch (err: unknown) {
      skillsList.value = previous
      const response = err as { response?: { data?: { error?: string } } }
      toast.error(response.response?.data?.error || t('skillsToggleFailed'))
    }
  }

  return {
    openSkillsPicker,
    onSkillsInputChange,
    onSkillsDrop,
    onSkillsDragOver,
    onSkillsDragLeave,
    uploadSkills,
    deleteSkill,
    setSkillEnabled,
  }
}
