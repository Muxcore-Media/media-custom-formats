package internal

import (
	"context"
	"fmt"
	"strings"

	formatsv1 "github.com/Muxcore-Media/media-custom-formats/proto/formatsv1"
)

func (m *Module) ListReleaseProfiles(ctx context.Context, _ *formatsv1.ListReleaseProfilesRequest) (*formatsv1.ListReleaseProfilesResponse, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	groups := m.loadReleaseGroups()
	out := make([]*formatsv1.ReleaseProfile, 0, len(groups))
	for _, g := range groups {
		out = append(out, &formatsv1.ReleaseProfile{
			Id: g.ID, Name: g.Name,
			Preferred: g.Preferred, MustContain: g.MustContain, MustNotContain: g.MustNotContain,
			PreferredScore: g.PreferredScore, Enabled: g.Enabled,
		})
	}
	return &formatsv1.ListReleaseProfilesResponse{Profiles: out}, nil
}

func (m *Module) UpsertReleaseProfile(ctx context.Context, req *formatsv1.UpsertReleaseProfileRequest) (*formatsv1.UpsertReleaseProfileResponse, error) {
	name := strings.TrimSpace(req.GetName())
	if name == "" {
		return nil, fmt.Errorf("name required")
	}
	score := req.GetPreferredScore()
	if score == 0 {
		score = 10
	}
	enabled := true
	if req.Enabled != nil {
		enabled = req.GetEnabled()
	}
	g := releaseProfileGroup{
		ID:             strings.TrimSpace(req.GetId()),
		Name:           name,
		Preferred:      req.GetPreferred(),
		MustContain:    req.GetMustContain(),
		MustNotContain: req.GetMustNotContain(),
		PreferredScore: score,
		Enabled:        enabled,
	}
	if err := m.upsertReleaseGroup(ctx, g); err != nil {
		return nil, err
	}
	// reload to return canonical id
	for _, loaded := range m.loadReleaseGroups() {
		if loaded.Name == name || (g.ID != "" && loaded.ID == g.ID) {
			return &formatsv1.UpsertReleaseProfileResponse{
				Profile: &formatsv1.ReleaseProfile{
					Id: loaded.ID, Name: loaded.Name,
					Preferred: loaded.Preferred, MustContain: loaded.MustContain, MustNotContain: loaded.MustNotContain,
					PreferredScore: loaded.PreferredScore, Enabled: loaded.Enabled,
				},
			}, nil
		}
	}
	return &formatsv1.UpsertReleaseProfileResponse{
		Profile: &formatsv1.ReleaseProfile{
			Id: g.ID, Name: g.Name, Preferred: g.Preferred, MustContain: g.MustContain,
			MustNotContain: g.MustNotContain, PreferredScore: g.PreferredScore, Enabled: g.Enabled,
		},
	}, nil
}

func (m *Module) DeleteReleaseProfile(ctx context.Context, req *formatsv1.DeleteReleaseProfileRequest) (*formatsv1.DeleteReleaseProfileResponse, error) {
	id := strings.TrimSpace(req.GetId())
	if id == "" {
		return nil, fmt.Errorf("id required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.db == nil {
		return nil, fmt.Errorf("not initialized")
	}
	res, err := m.db.ExecContext(ctx, `DELETE FROM release_profile_groups WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, fmt.Errorf("release profile not found: %s", id)
	}
	return &formatsv1.DeleteReleaseProfileResponse{}, nil
}
