<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import AppTextInput from '@/components/ui/AppTextInput.vue'
import AppPasswordInput from '@/components/ui/AppPasswordInput.vue'
import { useToast } from '@/composables/useToast'
import { useI18n } from 'vue-i18n'
import { authAPI } from '@/api/auth'

const props = withDefaults(defineProps<{
  visible: boolean
  forceMode?: boolean
}>(), {
  forceMode: false,
})

const emit = defineEmits<{
  'update:visible': [value: boolean]
  success: []
}>()

const { t } = useI18n()
const toast = useToast()

const submitting = ref(false)
const username = ref('')
const oldPassword = ref('')
const newPassword = ref('')
const oldPasswordError = ref('')
const newPasswordError = ref('')
const oldPasswordShake = ref(false)

const title = computed(() => (props.forceMode ? t('forcePasswordChangeTitle') : t('accountEditTitle')))

function resetForm() {
  username.value = ''
  oldPassword.value = ''
  newPassword.value = ''
  oldPasswordError.value = ''
  newPasswordError.value = ''
  oldPasswordShake.value = false
}

function triggerOldPasswordShake() {
  oldPasswordShake.value = false
  window.setTimeout(() => {
    oldPasswordShake.value = true
    window.setTimeout(() => {
      oldPasswordShake.value = false
    }, 260)
  }, 0)
}

function clearOldPasswordError() {
  oldPasswordError.value = ''
}

function clearNewPasswordError() {
  newPasswordError.value = ''
}

watch(
  () => props.visible,
  (visible) => {
    if (visible) {
      resetForm()
    }
  },
)

async function onConfirm() {
  const nextUsername = username.value.trim()
  const nextNewPassword = newPassword.value.trim()
  const nextOldPassword = oldPassword.value
  const normalizedOldPassword = nextOldPassword.trim()

  clearOldPasswordError()
  clearNewPasswordError()

  if (props.forceMode && nextNewPassword === '') {
    newPasswordError.value = t('newPasswordRequired')
    return
  }
  if (nextUsername === '' && nextNewPassword === '') {
    toast.error(t('accountEditNeedOneField'))
    return
  }
  if (nextNewPassword !== '' && normalizedOldPassword === '') {
    oldPasswordError.value = t('oldPasswordRequired')
    return
  }
  if (nextNewPassword !== '' && nextNewPassword === normalizedOldPassword) {
    newPasswordError.value = t('newPasswordSameAsOld')
    return
  }

  submitting.value = true
  try {
    await authAPI.updateAccount({
      username: nextUsername || undefined,
      oldPassword: nextOldPassword || undefined,
      newPassword: nextNewPassword || undefined,
    })
    toast.success(t('saveSuccess'))
    emit('success')
    emit('update:visible', false)
  } catch (error: any) {
    const responseStatus = error?.response?.status
    const responseError = String(error?.response?.data?.error || '')
    if (
      responseStatus === 401
      || (responseStatus === 400 && responseError.includes('旧密码错误'))
      || responseError.includes('旧密码错误')
    ) {
      oldPasswordError.value = t('oldPasswordIncorrect')
      triggerOldPasswordShake()
      return
    }
    if (responseStatus === 400 && responseError.includes('新密码不能与旧密码相同')) {
      newPasswordError.value = t('newPasswordSameAsOld')
      return
    }
    toast.error(responseError || t('accountEditFailed'))
  } finally {
    submitting.value = false
  }
}

function onCancel() {
  emit('update:visible', false)
}
</script>

<template>
  <AppDialog
    :visible="visible"
    :title="title"
    :confirm-text="t('confirm')"
    :cancel-text="t('cancel')"
    :confirm-loading="submitting"
    :show-close="!forceMode"
    :show-cancel="!forceMode"
    :close-on-mask="!forceMode"
    :close-on-esc="!forceMode"
    width="420px"
    @update:visible="emit('update:visible', $event)"
    @confirm="onConfirm"
    @cancel="onCancel"
  >
    <div class="flex flex-col gap-4">
      <div v-if="forceMode" class="account-force-tip text-xs">
        {{ t('forcePasswordChangeTip') }}
      </div>
      <div class="flex flex-col gap-1.5">
        <label class="text-xs font-medium account-field-label">{{ t('usernameOptional') }}</label>
        <AppTextInput
          v-model="username"
          class="px-3 py-2.5"
          :placeholder="t('usernameOptionalPlaceholder')"
        />
      </div>

      <div class="flex flex-col gap-1.5">
        <label class="text-xs font-medium account-field-label">{{ t('oldPassword') }}</label>
        <AppPasswordInput
          v-model="oldPassword"
          class="px-3 py-2.5"
          :class="{
            'account-input-error': !!oldPasswordError,
            'account-input-shake': oldPasswordShake,
          }"
          :placeholder="t('oldPasswordPlaceholder')"
          :show-label="t('showPassword')"
          :hide-label="t('hidePassword')"
          @update:model-value="clearOldPasswordError"
        />
        <p v-if="oldPasswordError" class="account-error-text text-xs">{{ oldPasswordError }}</p>
      </div>

      <div class="flex flex-col gap-1.5">
        <label class="text-xs font-medium account-field-label">{{ t('newPassword') }}</label>
        <AppPasswordInput
          v-model="newPassword"
          class="px-3 py-2.5"
          :class="{ 'account-input-error': !!newPasswordError }"
          :placeholder="t('newPasswordPlaceholder')"
          :show-label="t('showPassword')"
          :hide-label="t('hidePassword')"
          @update:model-value="clearNewPasswordError"
        />
        <p v-if="newPasswordError" class="account-error-text text-xs">{{ newPasswordError }}</p>
      </div>
    </div>
  </AppDialog>
</template>

<style scoped>
.account-force-tip {
  color: var(--sb-brand);
  background: var(--primary-alpha-08);
  border: 1px solid var(--primary-alpha-20);
  border-radius: 12px;
  padding: 10px 12px;
}

.account-field-label {
  color: var(--text-muted);
}

.account-error-text {
  color: var(--color-danger);
}

:deep(.account-input-error) {
  border-color: color-mix(in srgb, var(--color-danger) 90%, transparent) !important;
  box-shadow: 0 0 0 3px var(--danger-alpha-14) !important;
}

:deep(.account-input-error:focus) {
  border-color: color-mix(in srgb, var(--color-danger) 95%, transparent) !important;
  box-shadow: 0 0 0 3px var(--danger-alpha-16) !important;
}

:deep(.account-input-shake) {
  animation: account-input-shake 0.24s ease-in-out;
}

@keyframes account-input-shake {
  0%,
  100% {
    transform: translateX(0);
  }
  20% {
    transform: translateX(-3px);
  }
  40% {
    transform: translateX(3px);
  }
  60% {
    transform: translateX(-2px);
  }
  80% {
    transform: translateX(2px);
  }
}
</style>
