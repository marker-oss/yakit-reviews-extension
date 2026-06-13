export type WidgetContext = 'product' | 'homepage'

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
  visibility: {
    photos: boolean
    sellerAnswers: boolean
    prosCons: boolean
    marketplaceBadges: boolean
    ratingDistribution: boolean
    filters: boolean
  }
}

export const defaultWidgetConfig: WidgetConfig = {
  theme: {
    accent: '#2f7a5b',
    accentInk: '#ffffff',
    text: '#1f2520',
    muted: '#687067',
    panel: '#ffffff',
    border: '#d9ded7',
    dark: false,
  },
  typography: {
    fontFamily: 'inherit',
    scale: 1,
    radius: 8,
    density: 'comfortable',
  },
  layout: {
    mode: 'list',
    columns: 2,
    pageSize: 3,
    pagination: 'more',
  },
  visibility: {
    photos: true,
    sellerAnswers: true,
    prosCons: true,
    marketplaceBadges: true,
    ratingDistribution: true,
    filters: true,
  },
}

export function mergeWidgetConfig(value: Partial<WidgetConfig>): WidgetConfig {
  return {
    theme: { ...defaultWidgetConfig.theme, ...(value.theme ?? {}) },
    typography: { ...defaultWidgetConfig.typography, ...(value.typography ?? {}) },
    layout: { ...defaultWidgetConfig.layout, ...(value.layout ?? {}) },
    visibility: { ...defaultWidgetConfig.visibility, ...(value.visibility ?? {}) },
  }
}
