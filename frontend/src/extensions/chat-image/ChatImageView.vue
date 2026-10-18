<template>
  <AppLayout>
    <section class="chat-image-shell" :class="{ 'chat-image-fullscreen': fullscreen }">
      <aside class="conversation-sidebar" :class="{ 'conversation-sidebar-open': mobileSidebar }">
        <div class="sidebar-heading">
          <div>
            <h1>对话画图</h1>
            <p>{{ conversationTotal }} 个会话</p>
          </div>
          <button class="icon-button" title="新建对话" :disabled="historyLoading" @click="newConversation">
            <Icon name="plus" />
          </button>
        </div>

        <label class="search-box">
          <Icon name="search" size="sm" />
          <input v-model="search" placeholder="搜索对话" />
        </label>

        <div class="conversation-list">
          <button
            v-for="conversation in conversations"
            :key="conversation.id"
            class="conversation-item"
            :class="{ active: conversation.id === activeId }"
            @click="selectConversation(conversation.id)"
          >
            <span class="conversation-title">{{ conversation.title }}</span>
            <span class="conversation-meta">
              {{ conversation.messageCount }} 条 · {{ formatDate(conversation.updatedAt) }}
            </span>
            <span v-if="conversationBadge(conversation)" class="conversation-badge" :class="conversationBadge(conversation)?.tone">
              {{ conversationBadge(conversation)?.label }}
            </span>
          </button>
          <button v-if="hasMoreConversations" class="load-more" :disabled="historyLoading" @click="loadMoreConversations">
            {{ historyLoading ? '加载中' : '加载更多' }}
          </button>
        </div>

        <button
          v-if="activeConversation"
          class="delete-conversation"
          @click="deleteConversation(activeConversation.id)"
        >
          <Icon name="trash" size="sm" />
          删除当前会话
        </button>
      </aside>

      <main class="conversation-main">
        <header class="conversation-header">
          <div class="header-copy">
            <button class="icon-button mobile-only" title="打开会话列表" @click="mobileSidebar = true">
              <Icon name="menu" />
            </button>
            <div>
              <h2>{{ activeConversation?.title || '新对话' }}</h2>
              <p>{{ mode === 'chat' ? `gpt-pro · ${chatModel}` : `gpt-Image-2 · ${imageOptions.model}` }}</p>
            </div>
          </div>
          <div class="header-actions">
            <a class="command-button" href="/fastaiui/recharge" style="text-decoration:none">余额充值</a>
            <button class="command-button gallery-button" @click="galleryOpen = true">
              <Icon name="grid" size="sm" />图库
            </button>
            <button class="command-button" :disabled="historyLoading" @click="newConversation">
              <Icon name="plus" size="sm" />新对话
            </button>
            <button class="command-button" :disabled="!activeConversation?.messageCount || historyLoading" @click="clearConversation">
              <Icon name="trash" size="sm" />清空
            </button>
            <button class="icon-button" :title="fullscreen ? '退出全屏' : '全屏'" @click="fullscreen = !fullscreen">
              <Icon :name="fullscreen ? 'x' : 'externalLink'" />
            </button>
          </div>
        </header>

        <div v-if="setupError" class="setup-error">
          <Icon name="exclamationCircle" />
          <div>
            <strong>初始化失败</strong>
            <p>{{ setupError }}</p>
          </div>
          <button class="command-button" @click="initialize">重试</button>
        </div>

        <div v-else-if="historyError" class="setup-error">
          <Icon name="exclamationCircle" />
          <div>
            <strong>记录服务异常</strong>
            <p>{{ historyError }}</p>
          </div>
          <button class="command-button" @click="reloadHistory">重试</button>
        </div>

        <div ref="messagePane" class="message-pane">
          <div v-if="historyLoading && !activeConversation?.loaded" class="empty-state">
            <span class="empty-icon"><Icon name="sparkles" size="xl" /></span>
            <h3>正在加载会话</h3>
          </div>
          <div v-else-if="!activeConversation?.messages.length" class="empty-state">
            <span class="empty-icon"><Icon name="sparkles" size="xl" /></span>
            <h3>{{ mode === 'chat' ? '开始一次对话' : '描述你想生成的画面' }}</h3>
            <p>{{ mode === 'chat' ? '适合文案、分析、创作与日常问答' : '支持文生图，也可以上传参考图进行编辑' }}</p>
            <div class="quick-prompts">
              <button v-for="preset in PROMPT_PRESETS.slice(0, 3)" :key="preset.title" @click="usePreset(preset.prompt)">
                <span>{{ preset.category }}</span>
                {{ preset.title }}
              </button>
            </div>
          </div>

          <article
            v-for="message in activeConversation?.messages || []"
            :key="message.id"
            class="message-row"
            :class="message.role"
          >
            <MessageAvatar :role="message.role" />
            <div class="message-stack" :class="message.kind">
              <div class="message-body" :class="message.kind">
                <p v-if="message.content">{{ message.content }}</p>
                <ImageResultGrid
                  v-if="imageSources(message).length"
                  :sources="imageSources(message)"
                  @preview="openImagePreview(imageSources(message), $event)"
                  @reference="useGeneratedImageAsReference(imageSources(message)[$event], $event)"
                />
                <div v-if="fileResources(message).length" class="file-list">
                  <a
                    v-for="file in fileResources(message)"
                    :key="file.id"
                    :href="file.url"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    <Icon name="externalLink" size="sm" />
                    <span>{{ file.displayName || '查看文件' }}</span>
                    <small v-if="file.sizeBytes">{{ formatBytes(file.sizeBytes) }}</small>
                  </a>
                </div>
              </div>
                <MessageCopyButton
                v-if="message.role === 'assistant' && message.content"
                :content="message.content"
              />
            </div>
          </article>

          <article v-if="streamingContent" class="message-row assistant">
            <MessageAvatar role="assistant" />
            <div class="message-stack text">
              <div class="message-body text"><p>{{ streamingContent }}</p></div>
            </div>
          </article>

          <div v-if="submitting && !streamingContent" class="message-row assistant">
            <MessageAvatar role="assistant" />
            <ImageResultGrid
              v-if="mode === 'image' || imageTaskProgress"
              :sources="[]"
              :pending-count="pendingImageCount"
              :pending-label="imageTaskProgress?.label || '等待账号'"
              :task-id="imageTaskProgress?.taskId || ''"
              :size="pendingImageSize"
            />
            <div v-else class="message-body loading-message">
              <span></span><span></span><span></span>
              <em>正在生成回复</em>
            </div>
          </div>
        </div>

        <footer class="composer-wrap">
          <div v-if="referencePreview" class="reference-chip">
            <img :src="referencePreview" alt="参考图" />
            <span>{{ referenceName }}</span>
            <button title="移除参考图" @click="clearReference"><Icon name="x" size="sm" /></button>
          </div>

          <div v-if="settingsOpen && mode === 'image'" ref="settingsPanel" class="settings-panel">
            <button class="settings-close" type="button" title="关闭参数设置" @click="settingsOpen = false">
              <Icon name="x" />
            </button>
            <div class="setting-field full">
              <label>模型</label>
              <select v-model="imageOptions.model">
                <option v-for="model in imageModels" :key="model" :value="model">{{ model }}</option>
              </select>
            </div>
            <div class="setting-field">
              <label>质量</label>
              <div class="segmented">
                <button v-for="quality in qualities" :key="quality.value" :class="{ active: imageOptions.quality === quality.value }" @click="imageOptions.quality = quality.value">
                  {{ quality.label }}
                </button>
              </div>
            </div>
            <div class="setting-field">
              <label>数量</label>
              <div class="segmented">
                <button v-for="count in [1, 2, 3, 4] as const" :key="count" :class="{ active: imageOptions.count === count }" @click="imageOptions.count = count">
                  {{ count }} 张
                </button>
              </div>
            </div>
            <div class="setting-field full">
              <label>比例</label>
              <div class="segmented ratio-options">
                <button v-for="ratio in IMAGE_RATIOS" :key="ratio.value" :class="{ active: imageRatio === ratio.value }" @click="setImageRatio(ratio.value)">
                  {{ ratio.label }}
                </button>
              </div>
            </div>
            <div class="setting-field full">
              <label>分辨率</label>
              <div class="segmented resolution-options">
                <button
                  v-for="resolution in IMAGE_RESOLUTIONS"
                  :key="resolution.value"
                  :class="{ active: imageResolution === resolution.value }"
                  :disabled="!isResolutionAvailable(imageRatio, resolution.value)"
                  @click="setImageResolution(resolution.value)"
                >
                  {{ resolution.label }}
                </button>
              </div>
              <small class="size-summary">{{ imageSizeSummary(imageOptions.size) }}</small>
            </div>
          </div>

          <div class="composer">
            <textarea
              v-model="prompt"
              :placeholder="mode === 'chat' ? '输入消息，Shift + Enter 换行' : '描述你想生成的画面，也可以上传参考图'"
              rows="2"
              :disabled="submitting || initializing"
              @keydown.enter.exact.prevent="submit"
            ></textarea>
            <div class="composer-toolbar">
              <div class="composer-tools">
                <label v-if="mode === 'image'" class="icon-button" title="上传参考图">
                  <Icon name="upload" />
                  <input ref="referenceInput" type="file" accept="image/png,image/jpeg,image/webp" hidden @change="onReferenceChange" />
                </label>
                <div class="mode-switch">
                  <button :class="{ active: mode === 'chat' }" @click="mode = 'chat'">
                    <Icon name="chat" size="sm" />对话
                  </button>
                  <button :class="{ active: mode === 'image' }" @click="mode = 'image'">
                    <Icon name="sparkles" size="sm" />画图
                  </button>
                </div>
                <button class="tool-button" @click="presetOpen = true">
                  <Icon name="book" size="sm" />提示词
                </button>
                <button v-if="mode === 'image'" ref="settingsTrigger" class="tool-button" :class="{ active: settingsOpen }" @click="settingsOpen = !settingsOpen">
                  <Icon name="cog" size="sm" />{{ imageSizeSummary(imageOptions.size) }}
                </button>
                <select v-else v-model="chatModel" class="model-select" title="聊天模型">
                  <option v-for="model in chatModels" :key="model" :value="model">{{ model }}</option>
                </select>
              </div>
              <button class="send-button" title="发送" :disabled="!canSubmit" @click="submit">
                <Icon name="arrowUp" />
              </button>
            </div>
          </div>
          <p class="billing-note">对话按 gpt-pro 分组计费，画图按 gpt-Image-2 分组计费</p>
        </footer>
      </main>
    </section>

    <div v-if="mobileSidebar" class="mobile-overlay" @click="mobileSidebar = false"></div>

    <PromptLibraryModal v-if="presetOpen" @close="presetOpen = false" @select="useLibraryPrompt" />

    <ImageGalleryDialog
      :show="galleryOpen"
      @close="galleryOpen = false"
      @deleted="handleGalleryDeleted"
    />

    <ImageLightbox
      v-if="previewImages.length"
      :sources="previewImages"
      :index="previewImageIndex"
      @update:index="previewImageIndex = $event"
      @close="closeImagePreview"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import { DEFAULT_CHAT_MODEL, DEFAULT_IMAGE_MODEL, PROMPT_PRESETS } from './config'
import { generateImages, listModels, sendChatStream } from './api'
import ImageLightbox from './ImageLightbox.vue'
import ImageResultGrid from './ImageResultGrid.vue'
import MessageAvatar from './MessageAvatar.vue'
import MessageCopyButton from './MessageCopyButton.vue'
import ImageGalleryDialog from './gallery/ImageGalleryDialog.vue'
import type { GalleryDeleteResult } from './gallery/types'
import { normalizeImageURL } from './imageUrl'
import PromptLibraryModal from './PromptLibraryModal.vue'
import type { PromptLibraryItem } from './promptLibrary'
import {
  IMAGE_RATIOS,
  IMAGE_RESOLUTIONS,
  imageSizeFor,
  imageSizeSummary,
  isResolutionAvailable,
  selectionForImageSize,
  type ImageRatio,
  type ImageResolution
} from './imageOptions'
import { ensureFeatureKeys } from './keys'
import {
  appendMessage as appendHistoryMessage,
  clearConversation as clearHistoryConversation,
  createConversation as createHistoryConversation,
  deleteConversation as deleteHistoryConversation,
  getConversation,
  listConversations
} from './history'
import type {
  ChatImageConversation,
  ChatImageMessage,
  ChatImageMessageInput,
  ChatImageResource,
  FeatureKeys,
  ImageOptions,
  ImageTaskProgress
} from './types'

const PAGE_SIZE = 30
const conversations = ref<ChatImageConversation[]>([])
const activeId = ref<number | null>(null)
const search = ref('')
const prompt = ref('')
const mode = ref<'chat' | 'image'>('chat')
const submitting = ref(false)
const streamingContent = ref('')
const initializing = ref(false)
const setupError = ref('')
const historyError = ref('')
const historyLoading = ref(false)
const conversationTotal = ref(0)
const conversationPage = ref(1)
const conversationPages = ref(1)
const fullscreen = ref(false)
const mobileSidebar = ref(false)
const settingsOpen = ref(false)
const presetOpen = ref(false)
const galleryOpen = ref(false)
const previewImages = ref<string[]>([])
const previewImageIndex = ref(0)
const messagePane = ref<HTMLElement | null>(null)
const referenceInput = ref<HTMLInputElement | null>(null)
const settingsPanel = ref<HTMLElement | null>(null)
const settingsTrigger = ref<HTMLElement | null>(null)
const featureKeys = ref<FeatureKeys | null>(null)
const chatModels = ref<string[]>([DEFAULT_CHAT_MODEL])
const imageModels = ref<string[]>([DEFAULT_IMAGE_MODEL])
const chatModel = ref(DEFAULT_CHAT_MODEL)
const referenceImage = ref<File | null>(null)
const referenceImageURL = ref('')
const referencePreview = ref('')
const referenceName = ref('')
const imageRatio = ref<ImageRatio>('auto')
const imageResolution = ref<ImageResolution>('auto')
const imageTaskProgress = ref<ImageTaskProgress | null>(null)
const pendingImageCount = ref(1)
const pendingImageSize = ref('auto')
let searchTimer: ReturnType<typeof setTimeout> | undefined
let listController: AbortController | undefined
let generationController: AbortController | undefined
let listSequence = 0
const imageOptions = reactive<ImageOptions>({
  model: DEFAULT_IMAGE_MODEL,
  quality: 'auto',
  count: 1,
  size: 'auto'
})

const qualities = [
  { value: 'auto', label: '自动' },
  { value: 'low', label: '低' },
  { value: 'medium', label: '中' },
  { value: 'high', label: '高' }
] as const

const activeConversation = computed(() =>
  conversations.value.find((item) => item.id === activeId.value)
)
const hasMoreConversations = computed(() => conversationPage.value < conversationPages.value)
const canSubmit = computed(
  () => Boolean(prompt.value.trim()) && !submitting.value && !initializing.value && !historyLoading.value && !setupError.value && !historyError.value
)

function id(): string {
  if (crypto.randomUUID) return crypto.randomUUID()
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (character) => {
    const random = Math.floor(Math.random() * 16)
    const value = character === 'x' ? random : (random & 0x3) | 0x8
    return value.toString(16)
  })
}

function isImageModel(model: string): boolean {
  const normalized = model.toLocaleLowerCase()
  return normalized.startsWith('gpt-image-') || normalized.startsWith('grok-imagine')
}

function setImageRatio(value: ImageRatio): void {
  imageRatio.value = value
  if (value === 'auto') imageResolution.value = 'auto'
  else if (imageResolution.value === 'auto') imageResolution.value = '1K'
  else if (!isResolutionAvailable(value, imageResolution.value)) imageResolution.value = '1K'
  imageOptions.size = imageSizeFor(imageRatio.value, imageResolution.value)
}

function setImageResolution(value: ImageResolution): void {
  if (!isResolutionAvailable(imageRatio.value, value)) return
  imageResolution.value = value
  if (value === 'auto') imageRatio.value = 'auto'
  else if (imageRatio.value === 'auto') imageRatio.value = '1:1'
  imageOptions.size = imageSizeFor(imageRatio.value, imageResolution.value)
}

function closeImagePreview(): void {
  previewImages.value = []
  previewImageIndex.value = 0
}

function openImagePreview(sources: string[], index: number): void {
  previewImages.value = [...sources]
  previewImageIndex.value = Math.max(0, Math.min(index, sources.length - 1))
}

function closeSettingsOnOutsideClick(event: PointerEvent): void {
  if (!settingsOpen.value || !(event.target instanceof Node)) return
  if (settingsPanel.value?.contains(event.target) || settingsTrigger.value?.contains(event.target)) return
  settingsOpen.value = false
}

function closeSettingsOnEscape(event: KeyboardEvent): void {
  if (event.key === 'Escape') settingsOpen.value = false
}

function applyImageSize(size: string): void {
  const selection = selectionForImageSize(size)
  imageRatio.value = selection.ratio
  imageResolution.value = selection.resolution
  imageOptions.size = imageSizeFor(selection.ratio, selection.resolution)
}

async function newConversation(): Promise<void> {
  historyLoading.value = true
  historyError.value = ''
  try {
    const conversation = await createHistoryConversation()
    conversations.value.unshift(conversation)
    conversationTotal.value += 1
    activeId.value = conversation.id
    prompt.value = ''
    mobileSidebar.value = false
    clearReference()
  } catch (error) {
    historyError.value = extractApiErrorMessage(error, '新建会话失败')
  } finally {
    historyLoading.value = false
  }
}

async function selectConversation(conversationId: number): Promise<void> {
  activeId.value = conversationId
  mobileSidebar.value = false
  const conversation = conversations.value.find((item) => item.id === conversationId)
  if (conversation?.loaded) {
    scrollToBottom()
    return
  }
  historyLoading.value = true
  historyError.value = ''
  try {
    const loaded = await getConversation(conversationId)
    replaceConversation(loaded)
    scrollToBottom()
  } catch (error) {
    historyError.value = extractApiErrorMessage(error, '加载会话失败')
  } finally {
    historyLoading.value = false
  }
}

async function deleteConversation(conversationId: number): Promise<void> {
  if (!window.confirm('确定删除当前会话及全部记录吗？此操作无法撤销。')) return
  historyLoading.value = true
  historyError.value = ''
  try {
    await deleteHistoryConversation(conversationId)
    conversations.value = conversations.value.filter((item) => item.id !== conversationId)
    conversationTotal.value = Math.max(0, conversationTotal.value - 1)
    if (!conversations.value.length) await newConversation()
    else if (activeId.value === conversationId) await selectConversation(conversations.value[0].id)
  } catch (error) {
    historyError.value = extractApiErrorMessage(error, '删除会话失败')
  } finally {
    historyLoading.value = false
  }
}

async function clearConversation(): Promise<void> {
  if (!activeConversation.value) return
  if (!window.confirm('确定清空当前会话的全部消息吗？')) return
  historyLoading.value = true
  historyError.value = ''
  try {
    const cleared = await clearHistoryConversation(activeConversation.value.id)
    replaceConversation({ ...cleared, loaded: true, messages: [] })
  } catch (error) {
    historyError.value = extractApiErrorMessage(error, '清空会话失败')
  } finally {
    historyLoading.value = false
  }
}

function replaceConversation(conversation: ChatImageConversation): void {
  const index = conversations.value.findIndex((item) => item.id === conversation.id)
  if (index === -1) conversations.value.unshift(conversation)
  else conversations.value[index] = conversation
}

async function persistMessage(
  conversationId: number,
  input: ChatImageMessageInput,
  transientImages: string[] = []
): Promise<ChatImageMessage> {
  const result = await appendHistoryMessage(conversationId, input)
  const conversation = conversations.value.find((item) => item.id === conversationId)
  if (conversation) {
    const existing = conversation.messages.findIndex((item) => item.clientId === result.message.clientId)
    const message = { ...result.message, ...(transientImages.length ? { transientImages } : {}) }
    if (existing === -1) conversation.messages.push(message)
    else conversation.messages[existing] = message
    conversation.title = result.title
    conversation.messageCount = result.messageCount
    conversation.updatedAt = result.updatedAt
    conversation.preview = message.content
    conversation.loaded = true
  }
  scrollToBottom()
  return result.message
}

async function loadConversationPage(page: number, append: boolean): Promise<void> {
  const sequence = ++listSequence
  listController?.abort()
  listController = new AbortController()
  historyLoading.value = true
  historyError.value = ''
  try {
    const result = await listConversations(page, PAGE_SIZE, search.value.trim(), listController.signal)
    if (sequence !== listSequence) return
    const priorActive = activeId.value
    conversations.value = append
      ? [
          ...conversations.value,
          ...result.items.filter(
            (item) => !conversations.value.some((existing) => existing.id === item.id)
          )
        ]
      : result.items
    conversationTotal.value = result.total
    conversationPage.value = result.page
    conversationPages.value = result.pages
    if (!append) {
      if (priorActive && conversations.value.some((item) => item.id === priorActive)) {
        activeId.value = priorActive
      } else if (conversations.value.length) {
        await selectConversation(conversations.value[0].id)
      } else if (!search.value.trim()) {
        await newConversation()
      } else {
        activeId.value = null
      }
    }
  } catch (error: any) {
    if (error?.code !== 'ERR_CANCELED') {
      historyError.value = extractApiErrorMessage(error, '加载会话列表失败')
    }
  } finally {
    if (sequence === listSequence) historyLoading.value = false
  }
}

async function loadMoreConversations(): Promise<void> {
  if (!hasMoreConversations.value || historyLoading.value) return
  await loadConversationPage(conversationPage.value + 1, true)
}

async function reloadHistory(): Promise<void> {
  await loadConversationPage(1, false)
}

async function initialize(): Promise<void> {
  initializing.value = true
  setupError.value = ''
  try {
    const keys = await ensureFeatureKeys()
    featureKeys.value = keys
    const [chat, image] = await Promise.all([listModels(keys.chat), listModels(keys.image)])
    const availableChatModels = chat.filter((model) => !isImageModel(model))
    const availableImageModels = image.filter(isImageModel)
    if (availableChatModels.length) chatModels.value = availableChatModels
    if (availableImageModels.length) imageModels.value = availableImageModels
    chatModel.value = chatModels.value.find((item) => item.toLowerCase() === DEFAULT_CHAT_MODEL) || chatModels.value[0]
    imageOptions.model = imageModels.value.find((item) => item.toLowerCase() === DEFAULT_IMAGE_MODEL) || imageModels.value[0]
  } catch (error) {
    setupError.value = extractApiErrorMessage(error, '初始化失败')
  } finally {
    initializing.value = false
  }
}

async function submit(): Promise<void> {
  const text = prompt.value.trim()
  if (!canSubmit.value || !featureKeys.value || !text) return

  const requestKeys = featureKeys.value
  const requestMode = mode.value
  const requestModel = requestMode === 'chat' ? chatModel.value : imageOptions.model
  const requestImageOptions = { ...imageOptions, model: requestModel }
  const requestReference = referenceImage.value || referenceImageURL.value || null
  const requestReferenceURL = referenceImageURL.value
  const requestReferenceName = referenceName.value

  // The request owns the snapshots above; the composer is immediately ready for the next input.
  prompt.value = ''
  clearReference()
  settingsOpen.value = false
  if (requestMode === 'image') {
    pendingImageCount.value = requestImageOptions.count
    pendingImageSize.value = requestImageOptions.size
  }

  let conversation = activeConversation.value
  if (!conversation) {
    await newConversation()
    conversation = activeConversation.value
  }
  if (!conversation) return
  if (!conversation.loaded) {
    await selectConversation(conversation.id)
    conversation = activeConversation.value
  }
  if (!conversation) return
  const conversationId = conversation.id
  submitting.value = true
  imageTaskProgress.value = null
  historyError.value = ''
  try {
    await persistMessage(conversationId, {
      clientId: id(),
      role: 'user',
      kind: 'text',
      content: text,
      model: requestModel,
      resources: requestMode === 'image' && requestReferenceURL
        ? [{ type: 'image', url: requestReferenceURL, displayName: requestReferenceName }]
        : undefined
    })
    const requestConversation = conversations.value.find((item) => item.id === conversationId)
    if (requestMode === 'chat') {
      streamingContent.value = ''
      const reply = await sendChatStream(
        requestKeys.chat,
        requestModel,
        requestConversation?.messages || [],
        (delta) => {
          streamingContent.value += delta
          scrollToBottom()
        }
      )
      const resources = reply.resources
        .filter((resource) => isPersistentResourceURL(resource.url))
        .slice(0, 16)
      await persistMessage(conversationId, {
        clientId: id(),
        role: 'assistant',
        kind: !reply.content && resources.length
          ? (resources.every((resource) => resource.type === 'image') ? 'images' : 'files')
          : 'text',
        content: reply.content,
        model: requestModel,
        resources
      })
    } else {
      generationController = new AbortController()
      const result = await generateImages(
        requestKeys.image,
        text,
        requestImageOptions,
        requestReference,
        (progress) => {
          imageTaskProgress.value = progress
          scrollToBottom()
        },
        generationController.signal
      )
      const images = result.images
      const persistentImages = images.filter(isPersistentResourceURL)
      const transientImages = images.filter((image) => !isPersistentResourceURL(image))
      await persistMessage(
        conversationId,
        {
          clientId: id(),
          role: 'assistant',
          kind: 'images',
          content: persistentImages.length ? '' : '图片已生成，但渠道未返回可持久化的资源 URL',
          model: requestModel,
          resources: persistentImages.map((url) => ({ type: 'image', url }))
        },
        transientImages
      )
    }
  } catch (error) {
    const message = extractApiErrorMessage(error, '请求失败，请稍后重试')
    try {
      await persistMessage(conversationId, {
        clientId: id(),
        role: 'assistant',
        kind: 'error',
        content: message,
        model: requestModel
      })
    } catch {
      historyError.value = `记录写入失败：${message}`
    }
  } finally {
    prompt.value = ''
    submitting.value = false
    streamingContent.value = ''
    imageTaskProgress.value = null
    generationController = undefined
    scrollToBottom()
  }
}

function isPersistentResourceURL(value: string): boolean {
  if (value.startsWith('/') && !value.startsWith('//')) return true
  try {
    const parsed = new URL(value)
    return parsed.protocol === 'http:' || parsed.protocol === 'https:'
  } catch {
    return false
  }
}

function imageSources(message: ChatImageMessage): string[] {
  return [
    ...message.resources.filter((resource) => resource.type === 'image').map((resource) => resource.url),
    ...(message.transientImages || [])
  ]
}

function fileResources(message: ChatImageMessage): ChatImageResource[] {
  return message.resources.filter((resource) => resource.type === 'file')
}

function handleGalleryDeleted(result: GalleryDeleteResult): void {
  const deletedIds = new Set(result.deletedIds)
  const deletedUrls = new Set(result.deletedUrls)
  for (const conversation of conversations.value) {
    if (!conversation.loaded) continue
    for (const message of conversation.messages) {
      message.resources = message.resources.filter(
        (resource) => !deletedIds.has(resource.id) && !deletedUrls.has(resource.url)
      )
    }
  }
}

function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / (1024 * 1024)).toFixed(1)} MB`
}

function usePreset(value: string): void {
  prompt.value = value
  mode.value = 'image'
  presetOpen.value = false
}

function useLibraryPrompt(item: PromptLibraryItem): void {
  prompt.value = item.prompt
  mode.value = 'image'
  if (item.image_model && imageModels.value.includes(item.image_model)) {
    imageOptions.model = item.image_model
  }
  if (item.image_count && item.image_count >= 1 && item.image_count <= 4) {
    imageOptions.count = item.image_count as ImageOptions['count']
  }
  if (item.image_size) applyImageSize(item.image_size)
  presetOpen.value = false
}

function conversationBadge(conversation: ChatImageConversation): { label: string; tone: string } | null {
  if (conversation.id === activeId.value && imageTaskProgress.value) return { label: '处理中 1', tone: 'processing' }
  if (!conversation.loaded) return null
  const last = conversation.messages.at(-1)
  return last?.kind === 'error' ? { label: '失败', tone: 'failed' } : null
}

function onReferenceChange(event: Event): void {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (!file) return
  clearReference()
  referenceImage.value = file
  referencePreview.value = URL.createObjectURL(file)
  referenceName.value = file.name
  mode.value = 'image'
}

function useGeneratedImageAsReference(source: string | undefined, index: number): void {
  const url = normalizeImageURL(source || '')
  if (!url) return
  clearReference()
  referenceImageURL.value = url
  referencePreview.value = url
  referenceName.value = `结果 ${index + 1}.png`
  mode.value = 'image'
  settingsOpen.value = false
  scrollToBottom()
}

function clearReference(): void {
  if (referencePreview.value.startsWith('blob:')) URL.revokeObjectURL(referencePreview.value)
  if (referenceInput.value) referenceInput.value.value = ''
  referenceImage.value = null
  referenceImageURL.value = ''
  referencePreview.value = ''
  referenceName.value = ''
}

function formatDate(value: string): string {
  const date = new Date(value)
  const today = new Date()
  if (date.toDateString() === today.toDateString()) {
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  }
  return date.toLocaleDateString([], { month: '2-digit', day: '2-digit' })
}

function scrollToBottom(): void {
  nextTick(() => messagePane.value?.scrollTo({ top: messagePane.value.scrollHeight, behavior: 'smooth' }))
}

watch(search, () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => loadConversationPage(1, false), 300)
})

watch(mode, (value) => {
  if (value !== 'image') settingsOpen.value = false
})

onMounted(async () => {
  document.addEventListener('pointerdown', closeSettingsOnOutsideClick)
  document.addEventListener('keydown', closeSettingsOnEscape)
  await Promise.all([loadConversationPage(1, false), initialize()])
})

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', closeSettingsOnOutsideClick)
  document.removeEventListener('keydown', closeSettingsOnEscape)
  clearReference()
  listController?.abort()
  generationController?.abort()
  if (searchTimer) clearTimeout(searchTimer)
})
</script>

<style scoped>
.chat-image-shell { height: calc(100vh - 8.5rem); min-height: 680px; display: grid; grid-template-columns: 280px minmax(0, 1fr); grid-template-rows: minmax(0, 1fr); overflow: hidden; border: 1px solid rgb(229 231 235); border-radius: 8px; background: rgb(255 255 255); color: rgb(17 24 39); }
:global(.dark .chat-image-shell) { border-color: rgb(55 65 81); background: rgb(17 24 39); color: rgb(243 244 246); }
.chat-image-fullscreen { position: fixed; inset: 0; z-index: 60; width: 100vw; height: 100vh; min-height: 0; border: 0; border-radius: 0; }
.conversation-sidebar { display: flex; min-width: 0; flex-direction: column; border-right: 1px solid rgb(229 231 235); background: rgb(249 250 251); padding: 18px 14px; }
:global(.dark .chat-image-shell .conversation-sidebar) { border-color: rgb(55 65 81); background: rgb(12 18 29); }
.sidebar-heading, .conversation-header, .header-copy, .header-actions, .composer-toolbar, .composer-tools, .reference-chip { display: flex; align-items: center; }
.sidebar-heading { justify-content: space-between; margin-bottom: 18px; }
.sidebar-heading h1, .conversation-header h2 { margin: 0; font-size: 16px; font-weight: 700; }
.sidebar-heading p, .conversation-header p { margin: 3px 0 0; font-size: 12px; color: rgb(107 114 128); }
.icon-button { display: inline-flex; width: 38px; height: 38px; flex: 0 0 38px; align-items: center; justify-content: center; border: 1px solid rgb(209 213 219); border-radius: 8px; background: transparent; color: inherit; cursor: pointer; }
.icon-button:hover, .command-button:hover, .tool-button:hover { background: rgb(243 244 246); }
:global(.dark .chat-image-shell .icon-button), :global(.dark .chat-image-shell .command-button), :global(.dark .chat-image-shell .tool-button) { border-color: rgb(75 85 99); }
:global(.dark .chat-image-shell .icon-button:hover), :global(.dark .chat-image-shell .command-button:hover), :global(.dark .chat-image-shell .tool-button:hover) { background: rgb(31 41 55); }
.search-box { display: flex; align-items: center; gap: 8px; height: 42px; padding: 0 12px; border: 1px solid rgb(209 213 219); border-radius: 8px; color: rgb(107 114 128); }
.search-box input { width: 100%; border: 0; outline: 0; background: transparent; color: inherit; font-size: 14px; }
.conversation-list { flex: 1; min-height: 0; overflow-y: auto; padding-top: 12px; }
.conversation-item { display: block; width: 100%; margin-bottom: 6px; padding: 11px 12px; border: 1px solid transparent; border-radius: 7px; background: transparent; color: inherit; text-align: left; cursor: pointer; }
.conversation-item:hover { background: rgb(243 244 246); }
.conversation-item.active { border-color: rgb(156 163 175); background: rgb(229 231 235); }
:global(.dark .chat-image-shell .conversation-item:hover) { background: rgb(31 41 55); }
:global(.dark .chat-image-shell .conversation-item.active) { border-color: rgb(75 85 99); background: rgb(39 46 58); }
.conversation-title { display: block; overflow: hidden; font-size: 14px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.conversation-meta { display: block; margin-top: 5px; font-size: 11px; color: rgb(107 114 128); }
.conversation-badge { display: inline-flex; margin-top: 7px; padding: 2px 8px; border-radius: 999px; font-size: 11px; font-weight: 600; }
.conversation-badge.processing { background: rgb(219 234 254); color: rgb(29 78 216); }
.conversation-badge.failed { background: rgb(254 226 226); color: rgb(185 28 28); }
:global(.dark .chat-image-shell .conversation-badge.processing) { background: rgb(30 58 138 / .45); color: rgb(191 219 254); }
:global(.dark .chat-image-shell .conversation-badge.failed) { background: rgb(127 29 29 / .45); color: rgb(254 202 202); }
.load-more { width: 100%; min-height: 34px; border: 0; background: transparent; color: rgb(107 114 128); font-size: 12px; cursor: pointer; }
.load-more:hover { color: inherit; }
.delete-conversation { display: flex; align-items: center; justify-content: center; gap: 7px; padding: 9px; border: 0; background: transparent; color: rgb(107 114 128); cursor: pointer; }
.delete-conversation:hover { color: rgb(220 38 38); }
.conversation-main { position: relative; display: grid; min-width: 0; min-height: 0; overflow: hidden; grid-template-columns: minmax(0, 1fr); grid-template-rows: auto auto minmax(0, 1fr) auto; }
.conversation-header { grid-row: 1; }
.setup-error { grid-row: 2; }
.message-pane { grid-row: 3; }
.composer-wrap { grid-row: 4; }
.conversation-header { min-height: 68px; justify-content: space-between; gap: 12px; padding: 10px 18px; border-bottom: 1px solid rgb(229 231 235); }
:global(.dark .chat-image-shell .conversation-header) { border-color: rgb(55 65 81); }
.header-copy { min-width: 0; gap: 10px; }
.header-copy > div { min-width: 0; }
.header-copy h2 { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.header-actions { gap: 8px; }
.command-button, .tool-button { display: inline-flex; min-height: 36px; align-items: center; justify-content: center; gap: 6px; padding: 0 12px; border: 1px solid rgb(209 213 219); border-radius: 8px; background: transparent; color: inherit; font-size: 13px; cursor: pointer; }
button:disabled { cursor: not-allowed; opacity: .45; }
.setup-error { display: flex; align-items: center; gap: 12px; margin: 12px 18px 0; padding: 12px 14px; border: 1px solid rgb(254 202 202); border-radius: 7px; background: rgb(254 242 242); color: rgb(185 28 28); }
:global(.dark .chat-image-shell .setup-error) { border-color: rgb(127 29 29); background: rgb(69 10 10 / .45); color: rgb(254 202 202); }
.setup-error div { flex: 1; }
.setup-error p { margin: 2px 0 0; font-size: 12px; }
.message-pane { min-width: 0; min-height: 0; overflow-x: hidden; overflow-y: auto; padding: 28px max(20px, 4vw) 36px; }
.empty-state { display: flex; min-height: 100%; flex-direction: column; align-items: center; justify-content: center; text-align: center; }
.empty-icon { display: inline-flex; width: 58px; height: 58px; align-items: center; justify-content: center; border: 1px solid rgb(209 213 219); border-radius: 50%; }
.empty-state h3 { margin: 18px 0 5px; font-size: 20px; }
.empty-state p { margin: 0; color: rgb(107 114 128); font-size: 14px; }
.quick-prompts { display: grid; width: min(680px, 100%); grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 10px; margin-top: 28px; }
.quick-prompts button { padding: 14px; border: 1px solid rgb(229 231 235); border-radius: 7px; background: transparent; color: inherit; text-align: left; cursor: pointer; }
.quick-prompts button:hover { border-color: rgb(107 114 128); }
.quick-prompts span { display: block; margin-bottom: 4px; color: rgb(107 114 128); font-size: 11px; }
:global(.dark .chat-image-shell .quick-prompts button) { border-color: rgb(55 65 81); }
.message-row { display: flex; width: 100%; align-items: flex-start; gap: 10px; margin: 0 auto 24px; }
.message-row.user { flex-direction: row-reverse; }
.message-stack { display: flex; max-width: min(780px, 82%); min-width: 0; flex-direction: column; align-items: flex-start; }
.message-row.user .message-stack { align-items: flex-end; }
.message-body { max-width: 100%; padding: 11px 14px; border: 1px solid rgb(229 231 235); border-radius: 8px; background: rgb(249 250 251); }
.message-row.user .message-body { background: rgb(243 244 246); }
:global(.dark .chat-image-shell .message-body) { border-color: rgb(55 65 81); background: rgb(23 31 43); }
:global(.dark .chat-image-shell .message-row.user .message-body) { background: rgb(39 46 58); }
.message-body p { margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; font-size: 14px; line-height: 1.65; }
.message-body.error { border-color: rgb(254 202 202); background: rgb(254 242 242); color: rgb(185 28 28); }
:global(.dark .chat-image-shell .message-body.error) { border-color: rgb(127 29 29); background: rgb(69 10 10 / .45); color: rgb(254 202 202); }
.message-stack.images { width: min(416px, 82%); max-width: min(416px, 82%); }
.message-body.images { width: 100%; max-width: 100%; padding: 0; border: 0; background: transparent; }
.file-list { display: grid; min-width: min(420px, 65vw); gap: 8px; }
.file-list a { display: grid; grid-template-columns: auto minmax(0, 1fr) auto; align-items: center; gap: 8px; padding: 9px 10px; border: 1px solid rgb(209 213 219); border-radius: 7px; color: inherit; text-decoration: none; }
:global(.dark .chat-image-shell .file-list a) { border-color: rgb(75 85 99); }
.file-list span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.file-list small { color: rgb(107 114 128); }
.loading-message { display: flex; align-items: center; gap: 5px; }
.loading-message span { width: 6px; height: 6px; border-radius: 50%; background: rgb(107 114 128); animation: pulse 1.2s infinite; }
.loading-message span:nth-child(2) { animation-delay: .15s; }
.loading-message span:nth-child(3) { animation-delay: .3s; }
.loading-message em { margin-left: 6px; color: rgb(107 114 128); font-size: 12px; font-style: normal; }
@keyframes pulse { 0%, 70%, 100% { opacity: .3; transform: translateY(0); } 35% { opacity: 1; transform: translateY(-2px); } }
.composer-wrap { position: relative; padding: 0 max(18px, 4vw) 16px; }
.composer { border: 1px solid rgb(209 213 219); border-radius: 8px; background: rgb(255 255 255); box-shadow: 0 10px 30px rgb(0 0 0 / .06); }
:global(.dark .chat-image-shell .composer) { border-color: rgb(75 85 99); background: rgb(17 24 39); box-shadow: 0 10px 30px rgb(0 0 0 / .2); }
.composer textarea { display: block; width: 100%; min-height: 70px; resize: none; border: 0; outline: 0; padding: 14px 16px 8px; background: transparent; color: inherit; font-size: 16px; line-height: 1.5; }
.composer-toolbar { justify-content: space-between; gap: 8px; padding: 8px 10px; border-top: 1px solid rgb(229 231 235); }
:global(.dark .chat-image-shell .composer-toolbar) { border-color: rgb(55 65 81); }
.composer-tools { min-width: 0; gap: 7px; }
.mode-switch { display: inline-flex; padding: 3px; border: 1px solid rgb(209 213 219); border-radius: 8px; }
:global(.dark .chat-image-shell .mode-switch) { border-color: rgb(75 85 99); }
.mode-switch button { display: inline-flex; height: 30px; align-items: center; gap: 5px; padding: 0 10px; border: 0; border-radius: 6px; background: transparent; color: rgb(107 114 128); cursor: pointer; }
.mode-switch button.active, .tool-button.active { background: rgb(17 24 39); color: white; }
:global(.dark .chat-image-shell .mode-switch button.active), :global(.dark .chat-image-shell .tool-button.active) { background: white; color: rgb(17 24 39); }
.model-select { max-width: 190px; height: 36px; border: 1px solid rgb(209 213 219); border-radius: 8px; padding: 0 8px; background: transparent; color: inherit; }
:global(.dark .chat-image-shell .model-select) { border-color: rgb(75 85 99); background: rgb(17 24 39); color: white; }
.send-button { display: inline-flex; width: 38px; height: 38px; flex: 0 0 38px; align-items: center; justify-content: center; border: 0; border-radius: 50%; background: rgb(17 24 39); color: white; cursor: pointer; }
:global(.dark .chat-image-shell .send-button) { background: white; color: rgb(17 24 39); }
.billing-note { margin: 7px 0 0; color: rgb(107 114 128); text-align: center; font-size: 11px; }
.reference-chip { width: fit-content; max-width: 100%; gap: 8px; margin-bottom: 8px; padding: 6px 8px; border: 1px solid rgb(209 213 219); border-radius: 7px; background: rgb(249 250 251); font-size: 12px; }
:global(.dark .chat-image-shell .reference-chip) { border-color: rgb(55 65 81); background: rgb(23 31 43); }
.reference-chip img { width: 34px; height: 34px; border-radius: 4px; object-fit: cover; }
.reference-chip span { max-width: 260px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.reference-chip button { border: 0; background: transparent; color: inherit; cursor: pointer; }
.settings-panel { position: absolute; z-index: 4; right: max(18px, 4vw); bottom: calc(100% + 8px); display: grid; width: min(700px, calc(100% - 36px)); max-height: min(680px, 72vh); overflow-y: auto; grid-template-columns: 1fr 1fr; gap: 16px; padding: 18px 54px 18px 18px; border: 1px solid rgb(209 213 219); border-radius: 8px; background: white; box-shadow: 0 18px 50px rgb(0 0 0 / .16); }
:global(.dark .chat-image-shell .settings-panel) { border-color: rgb(75 85 99); background: rgb(17 24 39); }
.settings-close { position: absolute; top: 10px; right: 10px; display: inline-flex; width: 34px; height: 34px; align-items: center; justify-content: center; border: 1px solid transparent; border-radius: 7px; background: transparent; color: inherit; cursor: pointer; }
.settings-close:hover { border-color: rgb(209 213 219); background: rgb(243 244 246); }
:global(.dark .chat-image-shell .settings-close:hover) { border-color: rgb(75 85 99); background: rgb(31 41 55); }
.setting-field.full { grid-column: 1 / -1; }
.setting-field label { display: block; margin-bottom: 7px; font-size: 12px; font-weight: 600; }
.setting-field select { width: 100%; height: 38px; border: 1px solid rgb(209 213 219); border-radius: 7px; padding: 0 10px; background: transparent; color: inherit; }
.segmented { display: grid; grid-template-columns: repeat(4, 1fr); gap: 6px; }
.segmented button { min-height: 36px; border: 1px solid rgb(209 213 219); border-radius: 7px; background: transparent; color: inherit; cursor: pointer; }
.segmented button.active { border-color: rgb(17 24 39); background: rgb(17 24 39); color: white; }
:global(.dark .chat-image-shell .segmented button), :global(.dark .chat-image-shell .setting-field select) { border-color: rgb(75 85 99); }
:global(.dark .chat-image-shell .segmented button.active) { border-color: white; background: white; color: rgb(17 24 39); }
.ratio-options { grid-template-columns: repeat(4, 1fr); }
.resolution-options { grid-template-columns: repeat(4, 1fr); }
.size-summary { display: block; margin-top: 8px; color: rgb(107 114 128); }
.preset-modal { width: min(1000px, 100%); max-height: min(760px, 90vh); overflow: hidden; border: 1px solid rgb(75 85 99); border-radius: 8px; background: white; color: rgb(17 24 39); }
:global(.dark .preset-modal) { background: rgb(17 24 39); color: rgb(243 244 246); }
.preset-modal > header { display: flex; align-items: center; justify-content: space-between; padding: 18px 20px; border-bottom: 1px solid rgb(229 231 235); }
:global(.dark .preset-modal > header) { border-color: rgb(55 65 81); }
.preset-modal h2, .preset-modal h3 { margin: 0; }
.preset-modal header p { margin: 4px 0 0; color: rgb(107 114 128); font-size: 12px; }
.preset-grid { display: grid; max-height: calc(90vh - 90px); overflow-y: auto; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 12px; padding: 18px; }
.preset-grid article { display: flex; min-height: 210px; flex-direction: column; padding: 16px; border: 1px solid rgb(229 231 235); border-radius: 7px; }
:global(.dark .preset-grid article) { border-color: rgb(55 65 81); }
.preset-grid article > span { color: rgb(107 114 128); font-size: 11px; }
.preset-grid h3 { margin-top: 8px; font-size: 16px; }
.preset-grid article p { flex: 1; color: rgb(107 114 128); font-size: 13px; line-height: 1.6; }
.preset-grid article button { align-self: flex-end; min-width: 64px; height: 34px; border: 0; border-radius: 7px; background: rgb(17 24 39); color: white; cursor: pointer; }
:global(.dark .preset-grid article button) { background: white; color: rgb(17 24 39); }
.mobile-only { display: none; }
.mobile-overlay { display: none; }
@media (max-width: 900px) {
  .chat-image-shell { height: calc(100vh - 7.5rem); min-height: 620px; grid-template-columns: 1fr; }
  .conversation-sidebar { position: fixed; z-index: 75; top: 0; bottom: 0; left: 0; width: min(310px, 86vw); transform: translateX(-100%); transition: transform .2s ease; }
  .conversation-sidebar-open { transform: translateX(0); }
  .mobile-overlay { position: fixed; inset: 0; z-index: 70; display: block; background: rgb(0 0 0 / .55); }
  .mobile-only { display: inline-flex; }
  .header-actions .command-button { display: none; }
  .header-actions .gallery-button { display: inline-flex; }
  .message-pane { padding: 20px 14px; }
  .message-stack, .message-stack.images { max-width: 88%; }
  .composer-wrap { padding: 0 10px 10px; }
  .tool-button { width: 38px; overflow: hidden; padding: 0; font-size: 0; }
  .model-select { max-width: 120px; }
  .quick-prompts { grid-template-columns: 1fr; }
  .settings-panel { right: 10px; width: calc(100% - 20px); grid-template-columns: 1fr; }
  .setting-field.full { grid-column: auto; }
  .preset-grid { grid-template-columns: 1fr; }
}
@media (max-width: 520px) {
  .composer-tools { gap: 4px; }
  .composer-tools > .icon-button { display: none; }
  .segmented, .ratio-options, .resolution-options { grid-template-columns: repeat(2, 1fr); }
}
</style>
