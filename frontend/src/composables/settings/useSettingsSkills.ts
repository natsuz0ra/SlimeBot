import type { Ref } from 'vue'
import { skillsAPI } from '@/api/skills'

type ToastLike = {
  error(message: string): void
  success(message: string): void
}

type Translate = (key: string) => string

type SkillItem = any

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
    const invalidCount = files.filter((file) => !file.name.toLowerCase().endsWith('.zip')).length
    if (invalidCount > 0) {
      toast.error(t('onlyZipAllowed'))
      return
    }

    skillsUploading.value = true
    try {
      const result = await skillsAPI.upload(files)
      const failed = Array.isArray(result?.failed) ? result.failed : []
      if (failed.length > 0) {
        const detail = failed.map((x: any) => `${x.file}: ${x.error}`).join('\n')
        toast.error(`${t('skillsUploadPartial')}\n${detail}`)
      } else {
        toast.success(t('skillsUploadSuccess'))
      }
      await refreshSkills()
    } catch (err: any) {
      const failed = err?.response?.data?.failed
      if (Array.isArray(failed) && failed.length > 0) {
        const detail = failed.map((x: any) => `${x.file}: ${x.error}`).join('\n')
        toast.error(`${t('skillsUploadFailed')}\n${detail}`)
      } else {
        toast.error(err?.response?.data?.error || t('skillsUploadFailed'))
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

  return {
    openSkillsPicker,
    onSkillsInputChange,
    onSkillsDrop,
    onSkillsDragOver,
    onSkillsDragLeave,
    uploadSkills,
    deleteSkill,
  }
}
