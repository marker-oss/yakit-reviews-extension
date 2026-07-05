export type WidgetContext = 'product' | 'homepage'

export type MarketplacePolicy = {
  hidden: boolean
  label: string
  showSourceLinks: boolean
}

export type WidgetConfig = {
  theme: {
    accent: string
    accentInk: string
    text: string
    muted: string
    panel: string
    border: string
    dark: boolean
  }
  typography: {
    fontFamily: string
    scale: number
    radius: number
    density: 'comfortable' | 'compact'
  }
  layout: {
    mode: 'list' | 'grid' | 'carousel'
    columns: number
    pageSize: number
    pagination: 'more' | 'pages'
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
    filters: boolean
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
  marketplacePolicy: Record<'wb' | 'ym' | 'ozon', MarketplacePolicy>
}

export const defaultWidgetConfig: WidgetConfig = {
  theme: {
    accent: '#68478D',
    accentInk: '#ffffff',
    text: '#2A2630',
    muted: '#6E6877',
    panel: '#ffffff',
    border: '#E7DFD7',
    dark: false,
  },
  typography: {
    fontFamily: 'Onest, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
    scale: 1,
    radius: 16,
    density: 'comfortable',
  },
  layout: {
    mode: 'list',
    columns: 2,
    pageSize: 3,
    pagination: 'more',
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
    filters: true,
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
}

export function mergeWidgetConfig(value: Partial<WidgetConfig>): WidgetConfig {
  return {
    theme: { ...defaultWidgetConfig.theme, ...(value.theme ?? {}) },
    typography: { ...defaultWidgetConfig.typography, ...(value.typography ?? {}) },
    layout: { ...defaultWidgetConfig.layout, ...(value.layout ?? {}) },
    header: { ...defaultWidgetConfig.header, ...(value.header ?? {}) },
    visibility: { ...defaultWidgetConfig.visibility, ...(value.visibility ?? {}) },
    defaults: { ...defaultWidgetConfig.defaults, ...(value.defaults ?? {}) },
    ranking: value.ranking?.length ? value.ranking : defaultWidgetConfig.ranking,
    marketplacePolicy: mergeMarketplacePolicy(value.marketplacePolicy),
  }
}

function mergeMarketplacePolicy(value: Partial<WidgetConfig['marketplacePolicy']> | undefined): WidgetConfig['marketplacePolicy'] {
  return {
    wb: { ...defaultWidgetConfig.marketplacePolicy.wb, ...(value?.wb ?? {}) },
    ym: { ...defaultWidgetConfig.marketplacePolicy.ym, ...(value?.ym ?? {}) },
    ozon: { ...defaultWidgetConfig.marketplacePolicy.ozon, ...(value?.ozon ?? {}) },
  }
}
