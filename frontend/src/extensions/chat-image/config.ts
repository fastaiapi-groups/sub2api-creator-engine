export const CHAT_GROUP_NAME = 'gpt-pro'
export const IMAGE_GROUP_NAME = 'gpt-Image-2'
export const CHAT_KEY_NAME = 'Chat'
export const IMAGE_KEY_NAME = 'Draw'
export const LEGACY_CHAT_KEY_NAMES = ['__chat_image_chat__'] as const
export const LEGACY_IMAGE_KEY_NAMES = ['__chat_image_draw__'] as const
export const DEFAULT_CHAT_MODEL = 'gpt-5.5'
export const DEFAULT_IMAGE_MODEL = 'gpt-image-2'

export const PROMPT_PRESETS = [
  {
    title: '商业宣传海报',
    category: '营销设计',
    prompt: '设计一张专业的商业宣传海报，主体突出，信息层级清晰，现代品牌视觉，高级摄影质感，留出可放置标题和行动按钮的区域。'
  },
  {
    title: '电商产品主图',
    category: '产品摄影',
    prompt: '生成一张高端电商产品主图，产品居中，干净背景，柔和棚拍光线，材质细节清晰，商业摄影，构图简洁。'
  },
  {
    title: '写实人物肖像',
    category: '人物摄影',
    prompt: '生成自然写实的人物肖像，真实皮肤纹理，柔和自然光，眼神清晰，浅景深，专业人像摄影，不要过度磨皮。'
  },
  {
    title: '社交媒体封面',
    category: '内容运营',
    prompt: '设计一张适合社交媒体发布的视觉封面，主题鲜明，高对比配色，主体具有吸引力，画面简洁并预留标题区域。'
  },
  {
    title: '3D 可爱风格',
    category: '插画风格',
    prompt: '转换为精致的 3D 可爱插画风格，圆润造型，柔和材质，明亮配色，干净背景，电影级灯光和细腻渲染。'
  },
  {
    title: '照片自然修复',
    category: '图像编辑',
    prompt: '自然修复这张照片，保留人物身份和原始构图，改善曝光、白平衡与细节，降低噪点，避免塑料感和过度锐化。'
  }
] as const
