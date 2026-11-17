package usecase

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/reports/domain"
	"github.com/VaivenEnterprise/VaivenApplication-Backend/internal/users"
)

// List devuelve la cola de reportes con estado status (vacío = open), los más
// antiguos primero (RF-10). limit 0 usa el valor por omisión; after es el id
// del último reporte de la página anterior.
func (s *Service) List(ctx context.Context, status, after string, limit int) (Page, error) {
	st, err := domain.ParseStatus(status)
	if err != nil {
		return Page{}, err
	}
	if limit <= 0 {
		limit = DefaultPage
	}
	if limit > MaxPage {
		limit = MaxPage
	}
	// Se pide uno de más para saber si hay otra página.
	rows, err := s.d.Repo.List(ctx, st, after, limit+1)
	if err != nil {
		return Page{}, err
	}
	var next *string
	if len(rows) > limit {
		rows = rows[:limit]
		id := rows[len(rows)-1].ID
		next = &id
	}
	views, err := s.views(ctx, rows)
	if err != nil {
		return Page{}, err
	}
	return Page{Items: views, Next: next}, nil
}

// views arma las vistas con los perfiles de quienes reportaron (en un solo
// lote) y el resumen de lo reportado.
func (s *Service) views(ctx context.Context, rows []domain.Report) ([]View, error) {
	seen := map[string]bool{}
	var ids []string
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, r := range rows {
		add(r.ReporterID)
		if r.TargetType == domain.TargetUser {
			add(r.TargetID)
		}
	}
	profiles := map[string]users.PublicUser{}
	if len(ids) > 0 {
		m, err := s.d.Users.PublicUsers(ctx, ids)
		if err != nil {
			return nil, fmt.Errorf("reports: leer perfiles: %w", err)
		}
		profiles = m
	}
	out := make([]View, 0, len(rows))
	for _, r := range rows {
		reporter, ok := profiles[r.ReporterID]
		if !ok {
			reporter = users.PublicUser{ID: r.ReporterID, DisplayName: "Cuenta eliminada"}
		}
		summary, err := s.summary(ctx, r, profiles)
		if err != nil {
			return nil, err
		}
		out = append(out, View{Report: r, Reporter: reporter, TargetSummary: summary})
	}
	return out, nil
}

// summary describe lo reportado para el revisor (título, vista previa del
// mensaje o nombre). Si ya no existe, UnavailableSummary.
func (s *Service) summary(ctx context.Context, r domain.Report, profiles map[string]users.PublicUser) (string, error) {
	switch r.TargetType {
	case domain.TargetItem:
		snap, err := s.d.Items.Snapshot(ctx, r.TargetID)
		if err != nil {
			if mapNotFound(err) == domain.ErrTargetNotFound {
				return UnavailableSummary, nil
			}
			return "", fmt.Errorf("reports: leer objeto: %w", err)
		}
		return snap.Title, nil
	case domain.TargetMessage:
		ref, found, err := s.d.Messages.Reference(ctx, r.TargetID)
		if err != nil {
			return "", fmt.Errorf("reports: leer mensaje: %w", err)
		}
		if !found {
			return UnavailableSummary, nil
		}
		return preview(ref.Body), nil
	case domain.TargetUser:
		if p, ok := profiles[r.TargetID]; ok && p.DisplayName != "" {
			return p.DisplayName, nil
		}
		return UnavailableSummary, nil
	}
	return "Decisión de moderación " + r.TargetID, nil
}

// preview recorta el texto a summaryRunes caracteres.
func preview(body string) string {
	t := strings.TrimSpace(body)
	if utf8.RuneCountInString(t) <= summaryRunes {
		return t
	}
	return string([]rune(t)[:summaryRunes-1]) + "…"
}
