package store

import "encoding/json"

func supplierArticleFromRaw(raw string) string {
	if raw == "" {
		return ""
	}
	var parsed struct {
		ProductDetails struct {
			SupplierArticle string `json:"supplierArticle"`
		} `json:"productDetails"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return ""
	}
	return parsed.ProductDetails.SupplierArticle
}
