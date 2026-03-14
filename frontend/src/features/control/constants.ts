export const API_BASE_RAW = (import.meta.env.VITE_API_BASE_URL as string | undefined)?.trim()
export const API_BASE = API_BASE_RAW ? API_BASE_RAW.replace(/\/+$/, '') : ''

export const ONBOARDING_REQUIRED_KEYS = ['TG_API_ID', 'TG_API_HASH', 'OPENAI_BASE_URL', 'OPENAI_API_KEY', 'OPENAI_MODEL']

export const MAIN_VISIBLE_SETTINGS = [
  'OPENAI_BASE_URL',
  'OPENAI_API_KEY',
  'OPENAI_MODEL',
  'AI_CONTEXT_MESSAGE_LIMIT',
  'AUTO_REPLY_ENABLED',
]

export const booleanSettingKeys = new Set(['AUTO_REPLY_ENABLED'])
export const numericSettingKeys = new Set(['AI_CONTEXT_MESSAGE_LIMIT'])
export const secretSettingKeys = new Set(['OPENAI_API_KEY', 'TG_API_HASH'])

export const settingLabels: Record<string, string> = {
  TG_API_ID: 'Telegram API ID',
  TG_API_HASH: 'Telegram API Hash',
  OPENAI_BASE_URL: 'OpenAI Base URL',
  OPENAI_API_KEY: 'OpenAI API Key',
  OPENAI_MODEL: 'OpenAI Model',
  AI_CONTEXT_MESSAGE_LIMIT: 'Context Message Limit',
  AUTO_REPLY_ENABLED: 'Auto Reply Enabled',
}
