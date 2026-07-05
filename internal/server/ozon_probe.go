package server

import (
	"context"
	"time"
)

const ozonProbeTTL = 10 * time.Minute

// ozonProductsWarning returns a seller-facing warning when the Ozon Api-Key
// cannot list products (the role the review→article mapping needs). The probe
// result is cached; saving new credentials invalidates it.
func (s *Server) ozonProductsWarning(ctx context.Context) string {
	if s.cfg.OzonProductsProbe == nil {
		return ""
	}
	s.ozonProbeMu.Lock()
	defer s.ozonProbeMu.Unlock()
	if !s.ozonProbeAt.IsZero() && time.Since(s.ozonProbeAt) < ozonProbeTTL {
		return s.ozonProbeWarning
	}
	warning := ""
	if err := s.cfg.OzonProductsProbe(ctx); err != nil {
		warning = "Ключ Ozon не может получить список товаров — отзывы не привяжутся к артикулам сайта. " +
			"Перевыпустите Api-Key с ролями «Отзывы» и «Товары» и сохраните его здесь. (" + err.Error() + ")"
	}
	s.ozonProbeAt = time.Now()
	s.ozonProbeWarning = warning
	return warning
}

func (s *Server) invalidateOzonProbe() {
	s.ozonProbeMu.Lock()
	defer s.ozonProbeMu.Unlock()
	s.ozonProbeAt = time.Time{}
	s.ozonProbeWarning = ""
}
