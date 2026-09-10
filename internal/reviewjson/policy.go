package reviewjson

import (
	"encoding/json"
	"sort"
	"strings"
)

type MarketplacePolicy struct {
	Hidden          bool   `json:"hidden"`
	Label           string `json:"label,omitempty"`
	ShowSourceLinks *bool  `json:"showSourceLinks,omitempty"`
}

type MarketplacePolicies map[string]MarketplacePolicy

func ParseMarketplacePolicies(payload string) MarketplacePolicies {
	var cfg struct {
		MarketplacePolicy MarketplacePolicies `json:"marketplacePolicy"`
	}
	if err := json.Unmarshal([]byte(payload), &cfg); err != nil {
		return nil
	}
	return cfg.MarketplacePolicy.Normalized()
}

func (policies MarketplacePolicies) Normalized() MarketplacePolicies {
	if len(policies) == 0 {
		return nil
	}
	normalized := make(MarketplacePolicies, len(policies))
	for marketplace, policy := range policies {
		marketplace = strings.ToLower(strings.TrimSpace(marketplace))
		if marketplace == "" {
			continue
		}
		policy.Label = strings.TrimSpace(policy.Label)
		normalized[marketplace] = policy
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func (policies MarketplacePolicies) ExcludedMarketplaces() []string {
	normalized := policies.Normalized()
	excluded := make([]string, 0, len(normalized))
	for marketplace, policy := range normalized {
		if policy.Hidden {
			excluded = append(excluded, marketplace)
		}
	}
	sort.Strings(excluded)
	return excluded
}

func (policy MarketplacePolicy) PublicLabel() string {
	return strings.TrimSpace(policy.Label)
}

func (policy MarketplacePolicy) SourceLinksAllowed() bool {
	return policy.ShowSourceLinks == nil || *policy.ShowSourceLinks
}

func (m Mapper) policyFor(marketplace string) MarketplacePolicy {
	if len(m.MarketplacePolicy) == 0 {
		return MarketplacePolicy{}
	}
	return m.MarketplacePolicy.Normalized()[strings.ToLower(strings.TrimSpace(marketplace))]
}
