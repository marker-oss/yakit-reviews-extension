export type WidgetContext = 'product' | 'homepage'

export type MarketplacePolicy = {
  hidden: boolean
  label: string
  showSourceLinks: boolean
}

export type WidgetAppearancePreset = 'default' | 'native-kit' | 'minimal' | 'editorial' | 'ugc-editorial' | 'ugc-community' | 'compact-commerce' | 'lead-summary'

export type CustomFieldDef = {
  id: string
  label: string
  type: 'select' | 'chips' | 'text'
  options: string[]
  required: boolean
  filterable: boolean
  showInReview: boolean
  showInSummary: boolean
}

export type WidgetConfig = {
  appearance: {
    preset: WidgetAppearancePreset
  }
  theme: {
    accent: string
    accentInk: string
    text: string
    muted: string
    panel: string
    border: string
    star: string
    dark: boolean
  }
  typography: {
    fontFamily: string
    inheritSite: boolean
    scale: number
    radius: number
    density: 'comfortable' | 'compact'
  }
  layout: {
    mode: 'list' | 'grid' | 'carousel' | 'video' | 'wall'
    columns: number
    pageSize: number
    pagination: 'more' | 'pages'
    video: {
      aspect: '3:4' | '9:16' | '1:1'
      tileWidth: number
      showSourceBadge: boolean
      showAuthor: boolean
      autoplayInViewer: boolean
      productPanel: boolean
    }
    wall: {
      minTileWidth: number
      gap: number
      maxTiles: number
    }
  }
  header: {
    title: string
  }
  visibility: {
    photos: boolean
    sellerAnswers: boolean
    prosCons: boolean
    marketplaceBadges: boolean
    ratingDistribution: boolean
    videoRail: boolean
    filters: boolean
    questions: boolean
  }
  defaults: {
    minRating: number
    requireText: boolean
    requirePhoto: boolean
    marketplace: 'all' | 'wb' | 'ozon' | 'ym'
    initialSort: 'relevance' | 'newest' | 'highest' | 'lowest' | 'media'
    textFirst: boolean
    photoFirst: boolean
    onlyWithAnswer: boolean
  }
  ranking: {
    field: 'pinned' | 'hasPhoto' | 'hasText' | 'rating' | 'createdAt'
    direction: 'asc' | 'desc'
  }[]
  customFields: CustomFieldDef[]
  marketplacePolicy: Record<'wb' | 'ym' | 'ozon', MarketplacePolicy>
}

export const defaultWidgetConfig: WidgetConfig = {
  appearance: {
    preset: 'default',
  },
  theme: {
    accent: '#68478D',
    accentInk: '#ffffff',
    text: '#2A2630',
    muted: '#6E6877',
    panel: '#ffffff',
    border: '#E7DFD7',
    star: '#C99A3F',
    dark: false,
  },
  typography: {
    fontFamily: 'Onest, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
    inheritSite: false,
    scale: 1,
    radius: 16,
    density: 'comfortable',
  },
  layout: {
    mode: 'list',
    columns: 2,
    pageSize: 3,
    pagination: 'more',
    video: {
      aspect: '9:16',
      tileWidth: 260,
      showSourceBadge: true,
      showAuthor: true,
      autoplayInViewer: true,
      productPanel: true,
    },
    wall: {
      minTileWidth: 200,
      gap: 12,
      maxTiles: 24,
    },
  },
  header: {
    title: 'Отзывы покупателей',
  },
  visibility: {
    photos: true,
    sellerAnswers: true,
    prosCons: true,
    marketplaceBadges: true,
    ratingDistribution: true,
    videoRail: true,
    filters: true,
    questions: true,
  },
  defaults: {
    minRating: 4,
    requireText: true,
    requirePhoto: false,
    marketplace: 'all',
    initialSort: 'relevance',
    textFirst: true,
    photoFirst: true,
    onlyWithAnswer: false,
  },
  ranking: [
    { field: 'pinned', direction: 'desc' },
    { field: 'hasPhoto', direction: 'desc' },
    { field: 'hasText', direction: 'desc' },
    { field: 'rating', direction: 'desc' },
    { field: 'createdAt', direction: 'desc' },
  ],
  marketplacePolicy: {
    wb: { hidden: false, label: '', showSourceLinks: true },
    ym: { hidden: false, label: '', showSourceLinks: true },
    ozon: { hidden: false, label: '', showSourceLinks: true },
  },
  customFields: [],
}

export function normalizeCustomFields(raw: unknown): CustomFieldDef[] {
  if (!Array.isArray(raw)) return []
  const seen = new Set<string>()
  const out: CustomFieldDef[] = []
  for (const item of raw.slice(0, 6)) {
    const field = (item ?? {}) as Partial<CustomFieldDef>
    const id = String(field.id || '').trim()
    const label = String(field.label || '').trim()
    const type = field.type === 'select' || field.type === 'chips' || field.type === 'text' ? field.type : ''
    if (!id || !label || !type || seen.has(id)) continue
    seen.add(id)
    const options = (Array.isArray(field.options) ? field.options : [])
      .map((option) => String(option || '').trim())
      .filter(Boolean)
      .slice(0, 12)
    if (type !== 'text' && options.length < 2) continue
    out.push({
      id,
      label,
      type,
      options,
      required: field.required === true,
      filterable: field.filterable === true,
      showInReview: field.showInReview !== false,
      showInSummary: field.showInSummary === true,
    })
  }
  return out
}

export function mergeWidgetConfig(value: Partial<WidgetConfig>): WidgetConfig {
  return {
    appearance: { ...defaultWidgetConfig.appearance, ...(value.appearance ?? {}) },
    theme: { ...defaultWidgetConfig.theme, ...(value.theme ?? {}) },
    typography: { ...defaultWidgetConfig.typography, ...(value.typography ?? {}) },
    layout: {
      ...defaultWidgetConfig.layout,
      ...(value.layout ?? {}),
      video: { ...defaultWidgetConfig.layout.video, ...(value.layout?.video ?? {}) },
      wall: { ...defaultWidgetConfig.layout.wall, ...(value.layout?.wall ?? {}) },
    },
    header: { ...defaultWidgetConfig.header, ...(value.header ?? {}) },
    visibility: { ...defaultWidgetConfig.visibility, ...(value.visibility ?? {}) },
    defaults: { ...defaultWidgetConfig.defaults, ...(value.defaults ?? {}) },
    customFields: normalizeCustomFields(value.customFields),
    marketplacePolicy: mergeMarketplacePolicy(value.marketplacePolicy),
    ranking: value.ranking?.length ? value.ranking : defaultWidgetConfig.ranking,
  }
}

function mergeMarketplacePolicy(value: Partial<WidgetConfig['marketplacePolicy']> | undefined): WidgetConfig['marketplacePolicy'] {
  return {
    wb: { ...defaultWidgetConfig.marketplacePolicy.wb, ...(value?.wb ?? {}) },
    ym: { ...defaultWidgetConfig.marketplacePolicy.ym, ...(value?.ym ?? {}) },
    ozon: { ...defaultWidgetConfig.marketplacePolicy.ozon, ...(value?.ozon ?? {}) },
  }
}
